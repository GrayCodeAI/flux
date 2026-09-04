package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/credentials"
)

const (
	anthropicCompactionURL     = "https://api.anthropic.com/v1/messages"
	anthropicCompactionBeta    = "compact-2026-01-12"
	anthropicCompactionEdit    = "compact_20260112"
	anthropicMinCompactTrigger = 50_000
)

// NativeCompactionOpts describes a provider-native conversation compaction request.
type NativeCompactionOpts struct {
	Provider        string
	Model           string
	Messages        []client.GraycodeRouterMessage
	ContextWindow   int
	ThresholdPct    int
	MaxOutputTokens int
}

// NativeCompactionResult is provider-neutral output from native compaction.
type NativeCompactionResult struct {
	Summary string
}

// SupportsNativeCompaction reports whether GraycodeRouter can compact this selection
// with a configured provider credential.
func SupportsNativeCompaction(ctx context.Context, provider, model string) bool {
	return SupportsNativeCompactionWithStore(ctx, provider, model, credentials.DefaultStore())
}

// SupportsNativeCompactionWithStore is the host-neutral form using an explicit
// credential store.
func SupportsNativeCompactionWithStore(ctx context.Context, provider, model string, store credentials.Store) bool {
	if !supportsAnthropicCompactionSelection(provider, model) {
		return false
	}
	if store == nil {
		store = credentials.DefaultStore()
	}
	secret, err := store.Get(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"))
	return err == nil && strings.TrimSpace(secret) != ""
}

// CompactNativeConversation invokes the provider-native compaction protocol.
func CompactNativeConversation(ctx context.Context, opts NativeCompactionOpts) (*NativeCompactionResult, error) {
	return CompactNativeConversationWithStore(ctx, opts, credentials.DefaultStore())
}

// CompactNativeConversationWithStore invokes provider-native compaction using
// an explicit host credential store.
func CompactNativeConversationWithStore(ctx context.Context, opts NativeCompactionOpts, store credentials.Store) (*NativeCompactionResult, error) {
	if !supportsAnthropicCompactionSelection(opts.Provider, opts.Model) {
		return nil, fmt.Errorf("runtime: provider native compaction unavailable for %s/%s", opts.Provider, opts.Model)
	}
	if store == nil {
		store = credentials.DefaultStore()
	}
	apiKey, err := store.Get(ctx, credentials.AccountForEnv("ANTHROPIC_API_KEY"))
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("runtime: Anthropic credential required for native compaction")
	}
	trigger := opts.ContextWindow * opts.ThresholdPct / 100
	if trigger < anthropicMinCompactTrigger {
		trigger = anthropicMinCompactTrigger
	}
	maxOutput := opts.MaxOutputTokens
	if maxOutput <= 0 {
		maxOutput = 8192
	}
	messages, system := anthropicCompactionMessages(opts.Messages)
	body := map[string]any{
		"model": opts.Model, "max_tokens": maxOutput, "messages": messages,
		"context_management": map[string]any{"edits": []map[string]any{{
			"type":                   anthropicCompactionEdit,
			"trigger":                map[string]any{"type": "input_tokens", "value": trigger},
			"pause_after_compaction": true,
		}}},
	}
	if system != "" {
		body["system"] = system
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicCompactionURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", anthropicCompactionBeta)

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runtime: Anthropic compaction HTTP %d: %s", resp.StatusCode, truncateNativeCompactionError(respBody))
	}
	var parsed anthropicNativeCompactionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("runtime: parse Anthropic compaction response: %w", err)
	}
	summary := parsed.summary()
	if summary == "" {
		return nil, fmt.Errorf("runtime: Anthropic compaction produced no summary (stop=%s)", parsed.StopReason)
	}
	return &NativeCompactionResult{Summary: summary}, nil
}

func supportsAnthropicCompactionSelection(provider, model string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	if provider != "anthropic" && !strings.Contains(provider, "anthropic") && !strings.HasPrefix(model, "claude-") {
		return false
	}
	for _, prefix := range []string{"claude-opus-4", "claude-sonnet-4", "claude-mythos"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func anthropicCompactionMessages(messages []client.GraycodeRouterMessage) ([]map[string]any, string) {
	var system string
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			system = message.Content
			continue
		}
		if message.Role == "assistant" && len(message.ToolUse) > 0 {
			content := make([]map[string]any, 0, len(message.ToolUse)+1)
			if message.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": message.Content})
			}
			for _, call := range message.ToolUse {
				input := call.Arguments
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": input})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
			continue
		}
		if message.Role == "user" && len(message.ToolResults) > 0 {
			content := make([]map[string]any, 0, len(message.ToolResults)+1)
			if message.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": message.Content})
			}
			for _, result := range message.ToolResults {
				block := map[string]any{"type": "tool_result", "tool_use_id": result.ToolUseID, "content": result.Content}
				if result.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
			out = append(out, map[string]any{"role": "user", "content": content})
			continue
		}
		out = append(out, map[string]any{"role": message.Role, "content": message.Content})
	}
	return out, system
}

type anthropicNativeCompactionResponse struct {
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type    string `json:"type"`
		Text    string `json:"text,omitempty"`
		Summary string `json:"summary,omitempty"`
	} `json:"content"`
}

func (response *anthropicNativeCompactionResponse) summary() string {
	for _, block := range response.Content {
		if block.Type != "compaction" && block.Type != "text" {
			continue
		}
		if summary := strings.TrimSpace(block.Summary); summary != "" {
			return summary
		}
		if text := strings.TrimSpace(block.Text); text != "" {
			return text
		}
	}
	return ""
}

func truncateNativeCompactionError(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}
