package client

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog"
)

// ProviderFeatures tracks which capabilities each provider supports.
// Prevents sending unsupported features (thinking, tools, images) to providers
// that don't handle them, avoiding cryptic API errors.
// Read-only after construction — no mutex needed.
type ProviderFeatures struct {
	features map[string]FeatureSet
}

// FeatureSet describes what a provider or model supports.
// When the catalog is loaded, per-model values from the live API take precedence
// over the hardcoded defaults.
type FeatureSet struct {
	Thinking         bool `json:"thinking"`
	AdaptiveThinking bool `json:"adaptive_thinking"`
	ToolUse          bool `json:"tool_use"`
	Images           bool `json:"images"`
	Streaming        bool `json:"streaming"`
	Caching          bool `json:"caching"`
	JSON             bool `json:"json_mode"`
	Embeddings       bool `json:"embeddings"`
	MaxContext       int  `json:"max_context"`
	MaxOutput        int  `json:"max_output"`
	Effort           bool `json:"effort"`
	StructuredOutput bool `json:"structured_output"`
	CodeExecution    bool `json:"code_execution"`
	Citations        bool `json:"citations"`
	PDFInput         bool `json:"pdf_input"`
}

// NewProviderFeatures creates a feature registry.
// The catalog is the single source of truth for per-model capabilities.
// The hardcoded map is empty — all values come from the live API via the catalog.
func NewProviderFeatures() *ProviderFeatures {
	return &ProviderFeatures{features: map[string]FeatureSet{}}
}

// Get returns features for a provider or model.
// The compiled catalog (populated from the live API) is the single source of truth.
// Returns zero-value FeatureSet if the catalog is not loaded — caller must ensure
// the catalog is loaded before querying features.
func (pf *ProviderFeatures) Get(provider string) FeatureSet {
	if fs := featureSetFromCatalog(provider); fs != nil {
		return *fs
	}
	return FeatureSet{}
}

// featureSetFromCatalog looks up per-model capabilities from the compiled catalog.
// Returns nil if the catalog is not loaded or the model is not found.
func featureSetFromCatalog(modelOrProvider string) *FeatureSet {
	if cachedCatalog == nil {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(modelOrProvider))
	if key == "" {
		return nil
	}
	// Try as deployment ID (e.g., "anthropic-direct")
	deploymentID := key + "-direct"
	if offerings, ok := cachedCatalog.OfferingsByDeployment[deploymentID]; ok && len(offerings) > 0 {
		fs := featureSetFromCapabilities(offerings[0].Capabilities)
		fs.Caching = true // Anthropic always supports caching
		return fs
	}
	// Try as canonical model ID (e.g., "anthropic/claude-sonnet-4-6")
	if offerings, ok := cachedCatalog.OfferingsByCanonicalModel[key]; ok && len(offerings) > 0 {
		return featureSetFromCapabilities(offerings[0].Capabilities)
	}
	// Try as native model ID (e.g., "claude-sonnet-4-6")
	for _, offerings := range cachedCatalog.OfferingsByDeployment {
		for _, offering := range offerings {
			if strings.EqualFold(offering.NativeModelID, key) || strings.EqualFold(offering.CanonicalModelID, key) {
				return featureSetFromCapabilities(offering.Capabilities)
			}
		}
	}
	return nil
}

// featureSetFromCapabilities converts a catalog CapabilitySetV1 to a client FeatureSet.
func featureSetFromCapabilities(caps catalog.CapabilitySet) *FeatureSet {
	return &FeatureSet{
		Thinking:         caps.ExplicitThinkingBudget == catalog.CapabilitySupported,
		AdaptiveThinking: caps.AdaptiveThinking == catalog.CapabilitySupported,
		ToolUse:          caps.FunctionCalling == catalog.CapabilitySupported,
		Images:           caps.ImageInput == catalog.CapabilitySupported,
		Streaming:        true,
		JSON:             caps.StructuredOutput == catalog.CapabilitySupported,
		MaxContext:       caps.MaxInputTokens,
		MaxOutput:        caps.MaxOutputTokens,
		Effort:           caps.Effort == catalog.CapabilitySupported,
		StructuredOutput: caps.StructuredOutput == catalog.CapabilitySupported,
		CodeExecution:    caps.CodeExecution == catalog.CapabilitySupported,
		Citations:        caps.Citations == catalog.CapabilitySupported,
		PDFInput:         caps.PDFInput == catalog.CapabilitySupported,
	}
}

// Supports checks if a provider supports a specific feature.
func (pf *ProviderFeatures) Supports(provider, feature string) bool {
	f := pf.Get(provider)
	switch strings.ToLower(feature) {
	case "thinking":
		return f.Thinking
	case "adaptive_thinking":
		return f.AdaptiveThinking
	case "tools", "tool_use":
		return f.ToolUse
	case "images":
		return f.Images
	case "streaming":
		return f.Streaming
	case "caching", "cache":
		return f.Caching
	case "json", "json_mode":
		return f.JSON
	case "embeddings":
		return f.Embeddings
	case "effort":
		return f.Effort
	case "structured_output":
		return f.StructuredOutput
	case "code_execution":
		return f.CodeExecution
	case "citations":
		return f.Citations
	case "pdf_input":
		return f.PDFInput
	default:
		return false
	}
}

// DeprecationChecker warns when using models approaching retirement.
// Read-only after construction — no mutex needed.
type DeprecationChecker struct {
	deprecations map[string]DeprecationInfo
}

// DeprecationInfo describes a model's deprecation status.
type DeprecationInfo struct {
	Model       string    `json:"model"`
	Deprecated  bool      `json:"deprecated"`
	RetireDate  time.Time `json:"retire_date"`
	Replacement string    `json:"replacement"`
	Message     string    `json:"message"`
}

// NewDeprecationChecker creates a checker with known deprecations.
func NewDeprecationChecker() *DeprecationChecker {
	dc := &DeprecationChecker{
		deprecations: map[string]DeprecationInfo{
			"claude-3-opus-20240229": {
				Model: "claude-3-opus-20240229", Deprecated: true,
				Replacement: "claude-opus-4-6", Message: "Claude 3 Opus is deprecated. Use Claude Opus 4.6.",
			},
			"claude-3-sonnet-20240229": {
				Model: "claude-3-sonnet-20240229", Deprecated: true,
				Replacement: "claude-sonnet-4-6", Message: "Claude 3 Sonnet is deprecated. Use Claude Sonnet 4.6.",
			},
			"claude-3-haiku-20240307": {
				Model: "claude-3-haiku-20240307", Deprecated: true,
				Replacement: "claude-haiku-4-5-20251001", Message: "Claude 3 Haiku is deprecated. Use Claude Haiku 4.5.",
			},
			"gpt-4-turbo": {
				Model: "gpt-4-turbo", Deprecated: true,
				Replacement: "gpt-4.1", Message: "GPT-4 Turbo is deprecated. Use GPT-4.1.",
			},
			"gpt-3.5-turbo": {
				Model: "gpt-3.5-turbo", Deprecated: true,
				Replacement: "gpt-4.1-mini", Message: "GPT-3.5 Turbo is deprecated. Use GPT-4.1 Mini.",
			},
			"deepseek-chat": {
				Model: "deepseek-chat", Deprecated: true,
				Replacement: "deepseek-v4-flash", Message: "deepseek-chat retires 2026-07-24. Use deepseek-v4-flash.",
			},
			"deepseek-reasoner": {
				Model: "deepseek-reasoner", Deprecated: true,
				Replacement: "deepseek-v4-pro", Message: "deepseek-reasoner retires 2026-07-24. Use deepseek-v4-pro.",
			},
		},
	}
	return dc
}

// Check returns deprecation info if the model is deprecated.
func (dc *DeprecationChecker) Check(model string) *DeprecationInfo {
	if info, ok := dc.deprecations[model]; ok && info.Deprecated {
		return &info
	}
	return nil
}

// Warn logs a deprecation warning if applicable.
func (dc *DeprecationChecker) Warn(model string) {
	if info := dc.Check(model); info != nil {
		slog.Warn("deprecated model", "model", model, "replacement", info.Replacement, "message", info.Message)
	}
}

// RequestLogger logs all API requests/responses for debugging.
type RequestLogger struct {
	mu      sync.Mutex
	enabled bool
	entries []RequestLogEntry
	maxSize int
}

// RequestLogEntry is a single logged API call.
type RequestLogEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	LatencyMs    int64     `json:"latency_ms"`
	Status       string    `json:"status"` // "success" or "error"
	Error        string    `json:"error,omitempty"`
	CacheHit     bool      `json:"cache_hit"`
}

// NewRequestLogger creates a logger.
func NewRequestLogger(enabled bool) *RequestLogger {
	return &RequestLogger{
		enabled: enabled,
		entries: make([]RequestLogEntry, 0, 100),
		maxSize: 500,
	}
}

// Log records a request.
func (rl *RequestLogger) Log(entry RequestLogEntry) {
	if !rl.enabled {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	entry.Timestamp = time.Now()
	rl.entries = append(rl.entries, entry)
	if len(rl.entries) > rl.maxSize {
		rl.entries = rl.entries[len(rl.entries)-rl.maxSize:]
	}
}

// Recent returns the last N log entries.
func (rl *RequestLogger) Recent(n int) []RequestLogEntry {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if n > len(rl.entries) {
		n = len(rl.entries)
	}
	result := make([]RequestLogEntry, n)
	copy(result, rl.entries[len(rl.entries)-n:])
	return result
}

// Summary returns aggregate stats from the log.
func (rl *RequestLogger) Summary() string {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if len(rl.entries) == 0 {
		return "No API calls logged."
	}

	total := len(rl.entries)
	var errors, cacheHits int
	var totalLatency int64
	var totalIn, totalOut int

	for _, e := range rl.entries {
		if e.Status == "error" {
			errors++
		}
		if e.CacheHit {
			cacheHits++
		}
		totalLatency += e.LatencyMs
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
	}

	avgLatency := totalLatency / int64(total)
	return fmt.Sprintf("API calls: %d (errors: %d, cache hits: %d, avg latency: %dms, tokens: %d in / %d out)",
		total, errors, cacheHits, avgLatency, totalIn, totalOut)
}
