package client

import (
	"fmt"
	"strings"
	"sync"
)

// CostEstimator estimates the cost of an API call BEFORE sending it.
// Helps developers set budgets and avoid surprise charges.
type CostEstimator struct{}

// CostEstimate is the pre-call cost prediction.
type CostEstimate struct {
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"estimated_output_tokens"`
	InputCostUSD  float64 `json:"input_cost_usd"`
	OutputCostUSD float64 `json:"estimated_output_cost_usd"`
	TotalCostUSD  float64 `json:"estimated_total_cost_usd"`
	Model         string  `json:"model"`
	CacheDiscount float64 `json:"cache_discount_usd"` // potential savings if cached
}

// NewCostEstimator creates an estimator.
func NewCostEstimator() *CostEstimator {
	return &CostEstimator{}
}

// Estimate predicts cost for a set of messages + expected output.
func (ce *CostEstimator) Estimate(messages []GraycodeRouterMessage, model string, maxOutputTokens int) CostEstimate {
	inputTokens := ce.countInputTokens(messages)
	if maxOutputTokens <= 0 {
		maxOutputTokens = 4096 // default estimate
	}

	inPrice := pricePerToken(model, true)
	outPrice := pricePerToken(model, false)

	est := CostEstimate{
		InputTokens:   inputTokens,
		OutputTokens:  maxOutputTokens,
		InputCostUSD:  float64(inputTokens) * inPrice,
		OutputCostUSD: float64(maxOutputTokens) * outPrice,
		Model:         model,
	}
	est.TotalCostUSD = est.InputCostUSD + est.OutputCostUSD
	est.CacheDiscount = est.InputCostUSD * 0.9 // 90% savings if fully cached

	return est
}

// FormatEstimate returns a human-readable cost estimate.
func (ce *CostEstimator) FormatEstimate(est CostEstimate) string {
	return fmt.Sprintf("Estimated cost: $%.4f (%d input + ~%d output tokens, %s)",
		est.TotalCostUSD, est.InputTokens, est.OutputTokens, est.Model)
}

// IsExpensive returns true if the estimated cost exceeds a threshold.
func (ce *CostEstimator) IsExpensive(est CostEstimate, threshold float64) bool {
	return est.TotalCostUSD > threshold
}

func (ce *CostEstimator) countInputTokens(messages []GraycodeRouterMessage) int {
	total := 0
	for _, m := range messages {
		total += estimateTextTokens(m.Content)
		for _, tr := range m.ToolResults {
			total += estimateTextTokens(tr.Content)
		}
		for _, tc := range m.ToolUse {
			total += 50 // tool call overhead
			for _, v := range tc.Arguments {
				total += estimateTextTokens(fmt.Sprintf("%v", v))
			}
		}
	}
	return total
}

// StreamingTokenCounter counts tokens as they stream in real-time.
// Provides running cost estimate during generation.
type StreamingTokenCounter struct {
	mu           sync.Mutex
	model        string
	inputTokens  int
	outputTokens int
	cachedTokens int
}

// NewStreamingTokenCounter creates a counter for a specific model.
func NewStreamingTokenCounter(model string, inputTokens int) *StreamingTokenCounter {
	return &StreamingTokenCounter{
		model:       model,
		inputTokens: inputTokens,
	}
}

// AddOutput records streamed output tokens.
func (stc *StreamingTokenCounter) AddOutput(text string) {
	stc.mu.Lock()
	stc.outputTokens += estimateTextTokens(text)
	stc.mu.Unlock()
}

// AddCached records cached input tokens.
func (stc *StreamingTokenCounter) AddCached(tokens int) {
	stc.mu.Lock()
	stc.cachedTokens = tokens
	stc.mu.Unlock()
}

// currentCostLocked returns the running cost. Caller must hold stc.mu.
func (stc *StreamingTokenCounter) currentCostLocked() float64 {
	inPrice := pricePerToken(stc.model, true)
	outPrice := pricePerToken(stc.model, false)
	regularIn := stc.inputTokens - stc.cachedTokens
	if regularIn < 0 {
		regularIn = 0
	}
	return float64(regularIn)*inPrice + float64(stc.cachedTokens)*inPrice*0.1 + float64(stc.outputTokens)*outPrice
}

// CurrentCost returns the running cost so far.
func (stc *StreamingTokenCounter) CurrentCost() float64 {
	stc.mu.Lock()
	defer stc.mu.Unlock()
	return stc.currentCostLocked()
}

// Summary returns current token counts and cost.
func (stc *StreamingTokenCounter) Summary() string {
	stc.mu.Lock()
	defer stc.mu.Unlock()
	return fmt.Sprintf("Tokens: %d in + %d out ($%.4f so far)",
		stc.inputTokens, stc.outputTokens, stc.currentCostLocked())
}

// PromptOptimizer compresses conversation history to reduce input tokens.
// Keeps the most recent messages intact but summarizes older ones.
type PromptOptimizer struct {
	maxInputTokens int
}

// NewPromptOptimizer creates an optimizer with a token budget.
func NewPromptOptimizer(maxInputTokens int) *PromptOptimizer {
	if maxInputTokens <= 0 {
		maxInputTokens = 100000
	}
	return &PromptOptimizer{maxInputTokens: maxInputTokens}
}

// Optimize compresses messages to fit within the token budget.
// Preserves: system message, first user message, last N messages.
// Summarizes: middle messages.
func (po *PromptOptimizer) Optimize(messages []GraycodeRouterMessage) []GraycodeRouterMessage {
	totalTokens := 0
	for _, m := range messages {
		totalTokens += estimateTextTokens(m.Content) + 10 // +10 for overhead
	}

	if totalTokens <= po.maxInputTokens {
		return messages // no optimization needed
	}

	// Keep first message (system/context) and last 6 messages
	keepEnd := 6
	if keepEnd > len(messages) {
		keepEnd = len(messages)
	}

	if len(messages) <= keepEnd+1 {
		return messages
	}

	// Compress middle messages into a summary
	middle := messages[1 : len(messages)-keepEnd]
	summary := compressMessages(middle)

	result := make([]GraycodeRouterMessage, 0, keepEnd+2)
	result = append(result, messages[0], GraycodeRouterMessage{
		Role:    "user",
		Content: "[Earlier conversation summary: " + summary + "]",
	})
	result = append(result, messages[len(messages)-keepEnd:]...)
	return result
}

func compressMessages(messages []GraycodeRouterMessage) string {
	var parts []string
	for _, m := range messages {
		if m.Content != "" {
			parts = append(parts, m.Role+": "+m.Content)
		}
	}
	raw := strings.Join(parts, "\n")

	compressed := compressForSummary(raw)
	if len(compressed) > 0 && len(compressed) < len(raw) {
		return compressed
	}

	// Fallback: naive truncation
	if len(raw) > 500 {
		raw = raw[:500]
	}
	return raw
}
