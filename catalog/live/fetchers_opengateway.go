package live

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/opengateway"
)

// opengatewayModel is the subset of the public GET /v1/models OpenGateway object
// we consume. The gateway returns pricing inline (effective_pricing is the rate
// actually billed to you; pricing is the underlying provider rate), so no
// authenticated per-model fetch is required.
type opengatewayModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Aliases       []string `json:"aliases"`
	ContextWindow int      `json:"context_window"`
	Pricing       struct {
		Prompt         string `json:"prompt"`
		Completion     string `json:"completion"`
		InputCacheRead string `json:"input_cache_read"`
	} `json:"pricing"`
	EffectivePricing struct {
		Prompt         string `json:"prompt"`
		Completion     string `json:"completion"`
		InputCacheRead string `json:"input_cache_read"`
	} `json:"effective_pricing"`
}

// FetchOpenGateway lists models from the public OpenGateway model catalog.
// No API key is required to list models or read pricing (pricing is public on
// GET /v1/models); OPENGATEWAY_API_KEY is only needed for inference requests.
func FetchOpenGateway(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["OPENGATEWAY_API_KEY"])
	baseURL := strings.TrimRight(envOr(env, "OPENGATEWAY_BASE_URL", opengateway.DefaultBaseURL), "/")

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("live: opengateway: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("live: opengateway: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("live: opengateway model fetch failed (%d)", resp.StatusCode)
	}

	var payload struct {
		Data []opengatewayModel `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, fmt.Errorf("live: opengateway: decode: %w", err)
	}
	return opengatewayEntries(payload.Data), nil
}

// opengatewayEntries maps the OpenGateway model objects to eyrie Entries.
func opengatewayEntries(models []opengatewayModel) []Entry {
	var entries []Entry
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		e := Entry{
			ID:            id,
			DisplayName:   strings.TrimSpace(m.Name),
			Description:   strings.TrimSpace(m.Description),
			OwnedBy:       "gitlawb",
			ContextWindow: m.ContextWindow,
		}

		// Prefer the gateway-billed rate (effective_pricing); fall back to provider pricing.
		// OpenGateway prices are given per token (e.g. 0.000000522 USD/token); convert to per-1M.
		e.InputPricePer1M = ratePerToken(m.EffectivePricing.Prompt, m.Pricing.Prompt) * 1_000_000
		e.OutputPricePer1M = ratePerToken(m.EffectivePricing.Completion, m.Pricing.Completion) * 1_000_000
		e.CachedReadPricePer1M = ratePerToken(m.EffectivePricing.InputCacheRead, m.Pricing.InputCacheRead) * 1_000_000

		// Every model is OpenAI chat-completions compatible (the gateway normalizes
		// tool calling and reasoning params across providers).
		e.Features = append(e.Features, "function_calling")
		if strings.Contains(id, "kimi") || strings.Contains(id, "glm") || strings.Contains(id, "nemotron") || strings.Contains(id, "qwen") {
			e.ThinkingEnabled = true
			e.Features = append(e.Features, "thinking:enabled")
		}
		_ = m.Aliases
		entries = append(entries, e)
	}
	return entries
}

// ratePerToken parses a price string like "0.000000522" (USD per token).
func ratePerToken(effective, fallback string) float64 {
	s := strings.TrimSpace(effective)
	if s == "" {
		s = strings.TrimSpace(fallback)
	}
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
