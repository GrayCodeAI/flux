package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/catalog/concentrate"
	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
	"github.com/GrayCodeAI/eyrie/catalog/xiaomi"
)

// Per-provider fetchers (Grok, ZAI, CanopyWave, OpenCodeGo, Kimi, Xiaomi,
// MiMo, OpenRouter, Anthropic, Gemini, Ollama, DeepSeek). Split out of
// fetchers.go for clarity.
func FetchGrok(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "XAI_BASE_URL", DefaultGrokBaseURL),
		env["XAI_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	enrichFromOpenRouter(entries, "x-ai/")
	return entries, nil
}

func FetchZAI(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "ZAI_BASE_URL", DefaultZAIBaseURL),
		env["ZAI_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	enrichFromOpenRouter(entries, "z-ai/")
	return entries, nil
}

// FetchZAICoding lists models using the GLM Coding Plan dedicated endpoint.
// It expects ZAI_CODING_API_KEY (and optional ZAI_CODING_BASE_URL) in the env map.
// This ensures proper quota/billing separation from the general pay-as-you-go path.
func FetchZAICoding(env map[string]string) ([]Entry, error) {
	return fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "ZAI_CODING_BASE_URL", DefaultZAICodingBaseURL),
		env["ZAI_CODING_API_KEY"], "Bearer",
	)
}

func FetchCanopyWave(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "CANOPYWAVE_BASE_URL", DefaultCanopyWaveBaseURL),
		env["CANOPYWAVE_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	// CanopyWave returns pricing in cents per 1M tokens, not dollars.
	// Convert to dollars: 140 cents = $1.40.
	for i := range entries {
		if entries[i].InputPricePer1M > 0 {
			entries[i].InputPricePer1M /= 100
		}
		if entries[i].OutputPricePer1M > 0 {
			entries[i].OutputPricePer1M /= 100
		}
	}
	return entries, nil
}

func FetchOpenCodeGo(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "OPENCODEGO_BASE_URL", DefaultOpenCodeGoBaseURL),
		env["OPENCODEGO_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	protocolEntries := make([]struct{ ID, Protocol string }, 0, len(entries))
	for i := range entries {
		entries[i].ID = opencodego.NativeModelID(entries[i].ID)
		// Merge with static metadata from docs (pricing, protocol, context windows).
		if meta, ok := opencodego.MetadataForModel(entries[i].ID); ok {
			entries[i] = enrichFromStaticMeta(entries[i], meta)
		} else if entries[i].Protocol == "" {
			// Unknown model — derive protocol from name pattern.
			entries[i].Protocol = opencodego.ProtocolForModel(entries[i].ID)
		}
		protocolEntries = append(protocolEntries, struct{ ID, Protocol string }{ID: entries[i].ID, Protocol: entries[i].Protocol})
	}
	opencodego.UpdateProtocolMap(protocolEntries)
	return entries, nil
}

// enrichFromStaticMeta fills Entry fields from the static docs-based metadata.
// API-provided fields (like id, owned_by) are preserved; static metadata fills gaps.
func enrichFromStaticMeta(e Entry, meta opencodego.ModelMetadata) Entry {
	e.Protocol = meta.Protocol
	if e.InputPricePer1M == 0 {
		e.InputPricePer1M = meta.InputPer1M
	}
	if e.OutputPricePer1M == 0 {
		e.OutputPricePer1M = meta.OutputPer1M
	}
	if e.CachedReadPricePer1M == 0 {
		e.CachedReadPricePer1M = meta.CachedRead
	}
	if e.CachedWritePricePer1M == 0 {
		e.CachedWritePricePer1M = meta.CachedWrite
	}
	if e.ContextWindow == 0 {
		e.ContextWindow = meta.Context
	}
	if e.MaxOutput == 0 {
		e.MaxOutput = meta.MaxOutput
	}
	if meta.TierThreshold > 0 {
		e.TierThreshold = meta.TierThreshold
		e.TieredInputPricePer1M = meta.TieredInputPer1M
		e.TieredOutputPricePer1M = meta.TieredOutputPer1M
		e.TieredCachedReadPer1M = meta.TieredCachedRead
		e.TieredCachedWritePer1M = meta.TieredCachedWrite
	}
	return e
}

func FetchKimi(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "MOONSHOT_BASE_URL", DefaultKimiBaseURL),
		env["MOONSHOT_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	// Enrich with pricing from OpenRouter (Kimi API doesn't return pricing).
	enrichFromOpenRouter(entries, "moonshotai/")
	return entries, nil
}

func FetchXiaomiPayg(env map[string]string) ([]Entry, error) {
	return fetchMimoOpenAIModels(env, "XIAOMI_MIMO_PAYG_API_KEY", "XIAOMI_MIMO_PAYG_BASE_URL", DefaultXiaomiBaseURL)
}

func FetchXiaomiTokenPlan(env map[string]string) ([]Entry, error) {
	base := resolveTokenPlanOpenAIBase(env)
	if base != "" {
		env2 := make(map[string]string, len(env)+1)
		for k, v := range env {
			env2[k] = v
		}
		env2["XIAOMI_MIMO_TOKEN_PLAN_BASE_URL"] = base
		env = env2
	}
	return fetchMimoOpenAIModels(env, "XIAOMI_MIMO_TOKEN_PLAN_API_KEY", "XIAOMI_MIMO_TOKEN_PLAN_BASE_URL", "")
}

func resolveTokenPlanOpenAIBase(env map[string]string) string {
	region, err := xiaomi.NormalizeRegion(env["XIAOMI_MIMO_TOKEN_PLAN_REGION"])
	if err != nil {
		region = ""
	}
	override := strings.TrimSpace(env["XIAOMI_MIMO_TOKEN_PLAN_BASE_URL"])
	base, err := xiaomi.ResolveOpenAIBasePreferRegion(xiaomi.BillingTokenPlan, region, override)
	if err != nil {
		return ""
	}
	return base
}

func fetchMimoOpenAIModels(env map[string]string, keyEnv, baseEnv, defaultBase string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env[keyEnv])
	if apiKey == "" {
		return nil, nil
	}
	base := strings.TrimSpace(env[baseEnv])
	if base == "" {
		base = strings.TrimSpace(env["XIAOMI_BASE_URL"])
	}
	if base == "" {
		base = defaultBase
	}
	if base == "" {
		return nil, fmt.Errorf("live: missing MiMo base URL (set %s or token plan region)", baseEnv)
	}
	return fetchMimoModels(context.Background(), base, apiKey, env)
}

func fetchMimoModels(ctx context.Context, baseURL, apiKey string, env map[string]string) ([]Entry, error) {
	raw, err := xiaomi.FetchOpenAIModelsJSON(ctx, baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	platform, _ := xiaomi.FetchPlatformModelsIndex(ctx, xiaomi.PlatformModelsURLFromEnv(env))
	var entries []Entry
	for _, r := range raw {
		entry, ok := entryFromOpenAICompatJSON(r)
		if ok {
			entries = append(entries, enrichMimoEntry(entry, platform))
		}
	}
	return entries, nil
}

func FetchOpenRouter(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["OPENROUTER_API_KEY"])
	if apiKey == "" {
		return nil, nil
	}
	baseURL := strings.TrimRight(envOr(env, "OPENROUTER_BASE_URL", DefaultOpenRouterBaseURL), "/")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Data {
		var m openRouterModel
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		ctx := 0
		if m.ContextLength != nil {
			ctx = *m.ContextLength
		} else if m.TopProvider != nil && m.TopProvider.ContextLength != nil {
			ctx = *m.TopProvider.ContextLength
		}
		maxOut := 0
		if m.TopProvider != nil && m.TopProvider.MaxCompletionTokens != nil {
			maxOut = *m.TopProvider.MaxCompletionTokens
		}
		var inPrice, outPrice float64
		if m.Pricing != nil {
			inPrice = asFloat(m.Pricing.Prompt) * 1_000_000
			outPrice = asFloat(m.Pricing.Completion) * 1_000_000
		}
		entries = append(entries, Entry{
			ID: id, InputPricePer1M: inPrice, OutputPricePer1M: outPrice,
			ContextWindow: ctx, MaxOutput: maxOut, DisplayName: id,
			RawJSON: append(json.RawMessage(nil), raw...),
		})
	}
	return entries, nil
}

// anthropicModelEntry represents one model from the Anthropic GET /v1/models response.
type anthropicModelEntry struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	MaxInputTokens int    `json:"max_input_tokens"`
	MaxTokens      int    `json:"max_tokens"`
	Capabilities   struct {
		Batch struct {
			Supported bool `json:"supported"`
		} `json:"batch"`
		Citations struct {
			Supported bool `json:"supported"`
		} `json:"citations"`
		CodeExecution struct {
			Supported bool `json:"supported"`
		} `json:"code_execution"`
		Effort struct {
			Supported bool `json:"supported"`
			Low       struct {
				Supported bool `json:"supported"`
			} `json:"low"`
			Medium struct {
				Supported bool `json:"supported"`
			} `json:"medium"`
			High struct {
				Supported bool `json:"supported"`
			} `json:"high"`
			XHigh struct {
				Supported bool `json:"supported"`
			} `json:"xhigh"`
			Max struct {
				Supported bool `json:"supported"`
			} `json:"max"`
		} `json:"effort"`
		ImageInput struct {
			Supported bool `json:"supported"`
		} `json:"image_input"`
		PDFInput struct {
			Supported bool `json:"supported"`
		} `json:"pdf_input"`
		StructuredOutputs struct {
			Supported bool `json:"supported"`
		} `json:"structured_outputs"`
		Thinking struct {
			Supported bool `json:"supported"`
			Types     struct {
				Enabled struct {
					Supported bool `json:"supported"`
				} `json:"enabled"`
				Adaptive struct {
					Supported bool `json:"supported"`
				} `json:"adaptive"`
			} `json:"types"`
		} `json:"thinking"`
	} `json:"capabilities"`
}

func FetchAnthropic(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["ANTHROPIC_API_KEY"])
	if apiKey == "" {
		return nil, nil
	}
	baseURL := strings.TrimRight(envOr(env, "ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"), "/")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Data {
		var m anthropicModelEntry
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = id
		}
		entry := Entry{
			ID: id, DisplayName: label,
			ContextWindow:  m.MaxInputTokens, // maps to ModelCatalogEntry.ContextWindow via LiveEntriesToCatalog
			MaxInputTokens: m.MaxInputTokens,
			MaxOutput:      m.MaxTokens,
			RawJSON:        append(json.RawMessage(nil), raw...),
		}
		// Extract capabilities
		entry.ThinkingEnabled = m.Capabilities.Thinking.Types.Enabled.Supported
		entry.ThinkingAdaptive = m.Capabilities.Thinking.Types.Adaptive.Supported
		if m.Capabilities.Effort.Supported {
			entry.EffortSupported = true
			var levels []string
			for _, lvl := range []string{"low", "medium", "high", "xhigh", "max"} {
				switch lvl {
				case "low":
					if m.Capabilities.Effort.Low.Supported {
						levels = append(levels, lvl)
					}
				case "medium":
					if m.Capabilities.Effort.Medium.Supported {
						levels = append(levels, lvl)
					}
				case "high":
					if m.Capabilities.Effort.High.Supported {
						levels = append(levels, lvl)
					}
				case "xhigh":
					if m.Capabilities.Effort.XHigh.Supported {
						levels = append(levels, lvl)
					}
				case "max":
					if m.Capabilities.Effort.Max.Supported {
						levels = append(levels, lvl)
					}
				}
			}
			entry.EffortLevels = strings.Join(levels, ",")
		}
		entry.StructuredOutput = m.Capabilities.StructuredOutputs.Supported
		entry.CodeExecution = m.Capabilities.CodeExecution.Supported
		entry.CitationsSupported = m.Capabilities.Citations.Supported
		entry.PDFInput = m.Capabilities.PDFInput.Supported
		entry.ImageInput = m.Capabilities.ImageInput.Supported
		// Populate Features list for downstream catalog pipeline
		if entry.ThinkingEnabled {
			entry.Features = append(entry.Features, "thinking:enabled")
		}
		if entry.ThinkingAdaptive {
			entry.Features = append(entry.Features, "thinking:adaptive")
		}
		if entry.EffortSupported {
			entry.Features = append(entry.Features, "effort")
			if entry.EffortLevels != "" {
				entry.Features = append(entry.Features, "effort:"+entry.EffortLevels)
			}
		}
		if entry.StructuredOutput {
			entry.Features = append(entry.Features, "structured_output")
		}
		if entry.CodeExecution {
			entry.Features = append(entry.Features, "code_execution")
		}
		if entry.CitationsSupported {
			entry.Features = append(entry.Features, "citations")
		}
		if entry.PDFInput {
			entry.Features = append(entry.Features, "pdf_input")
		}
		if entry.ImageInput {
			entry.Features = append(entry.Features, "image_input")
		}
		entries = append(entries, entry)
	}
	// Enrich with pricing from OpenRouter (Anthropic API doesn't return pricing).
	enrichFromOpenRouter(entries, "anthropic/")
	return entries, nil
}

func FetchGemini(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["GEMINI_API_KEY"])
	if apiKey == "" {
		return nil, nil
	}
	base := strings.TrimRight(envOr(env, "GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"), "/")
	url := base + "/v1beta/models?key=" + apiKey
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Models {
		var m struct {
			Name                       string   `json:"name"`
			DisplayName                string   `json:"displayName"`
			InputTokenLimit            int      `json:"inputTokenLimit"`
			OutputTokenLimit           int      `json:"outputTokenLimit"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}
		supportsGen := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGen = true
				break
			}
		}
		if !supportsGen {
			continue
		}
		id := strings.TrimPrefix(name, "models/")
		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = id
		}
		entries = append(entries, Entry{
			ID: id, DisplayName: label, ContextWindow: m.InputTokenLimit, MaxOutput: m.OutputTokenLimit,
			RawJSON: append(json.RawMessage(nil), raw...),
		})
	}
	// Enrich with pricing from OpenRouter (Gemini API doesn't return pricing).
	enrichFromOpenRouter(entries, "google/")
	return entries, nil
}

func FetchOllama(env map[string]string) ([]Entry, error) {
	baseURL := strings.TrimSpace(env["OLLAMA_BASE_URL"])
	if baseURL == "" {
		return nil, nil
	}
	root := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, root+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		Models []json.RawMessage `json:"models"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Models {
		var m struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		id := strings.TrimSpace(m.Name)
		if id == "" {
			continue
		}
		entries = append(entries, Entry{
			ID: id, DisplayName: id,
			RawJSON: append(json.RawMessage(nil), raw...),
		})
	}
	return entries, nil
}

// FetchDeepSeek lists models from the DeepSeek OpenAI-compatible API.
func FetchDeepSeek(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "DEEPSEEK_BASE_URL", DefaultDeepSeekBaseURL),
		env["DEEPSEEK_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	enrichFromOpenRouter(entries, "deepseek/")
	return entries, nil
}

// FetchPoolside lists models from the Poolside OpenAI-compatible API.
func FetchPoolside(env map[string]string) ([]Entry, error) {
	return fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "POOLSIDE_BASE_URL", DefaultPoolsideBaseURL),
		env["POOLSIDE_API_KEY"], "Bearer",
	)
}

// FetchConcentrate lists models from the Concentrate AI OpenAI-compatible API.
// Concentrate returns a rich format with max_input_tokens, max_tokens, and capabilities.
// Set CONCENTRATE_FETCH_PRICING=true to fetch pricing from individual model endpoints
// (slower but more accurate). Pricing is cached to disk for 24 hours.
//
// Note: /v1/models does not require authentication per Concentrate docs.
// CONCENTRATE_API_KEY is only needed when CONCENTRATE_FETCH_PRICING=true.
func FetchConcentrate(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["CONCENTRATE_API_KEY"])
	isCustomBaseURL := env["CONCENTRATE_BASE_URL"] != ""
	fetchPricing := strings.EqualFold(env["CONCENTRATE_FETCH_PRICING"], "true")

	// Pricing fetch requires auth; model listing does not
	if fetchPricing && apiKey == "" {
		return nil, fmt.Errorf("concentrate: CONCENTRATE_API_KEY required when CONCENTRATE_FETCH_PRICING=true")
	}

	baseURL := strings.TrimRight(envOr(env, "CONCENTRATE_BASE_URL", "https://api.concentrate.ai/v1"), "/")
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("concentrate model fetch failed (%d)", resp.StatusCode)
	}

	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}

	var entries []Entry
	for _, raw := range payload.Data {
		var m struct {
			ID             string `json:"id"`
			DisplayName    string `json:"display_name"`
			MaxInputTokens int    `json:"max_input_tokens"`
			MaxTokens      int    `json:"max_tokens"`
			OwnedBy        string `json:"owned_by"`
			Capabilities   struct {
				Effort struct {
					Supported bool                     `json:"supported"`
					Low       struct{ Supported bool } `json:"low"`
					Medium    struct{ Supported bool } `json:"medium"`
					High      struct{ Supported bool } `json:"high"`
					XHigh     struct{ Supported bool } `json:"xhigh"`
					Max       struct{ Supported bool } `json:"max"`
				} `json:"effort"`
				ImageInput        struct{ Supported bool } `json:"image_input"`
				PDFInput          struct{ Supported bool } `json:"pdf_input"`
				StructuredOutputs struct{ Supported bool } `json:"structured_outputs"`
				Thinking          struct {
					Supported bool `json:"supported"`
					Types     struct {
						Enabled  struct{ Supported bool } `json:"enabled"`
						Adaptive struct{ Supported bool } `json:"adaptive"`
					} `json:"types"`
				} `json:"thinking"`
				CodeExecution     struct{ Supported bool } `json:"code_execution"`
				Citations         struct{ Supported bool } `json:"citations"`
				ContextManagement struct {
					Supported             bool                     `json:"supported"`
					ClearThinking20251015 struct{ Supported bool } `json:"clear_thinking_20251015"`
					ClearToolUses20250919 struct{ Supported bool } `json:"clear_tool_uses_20250919"`
					Compact20260112       struct{ Supported bool } `json:"compact_20260112"`
				} `json:"context_management"`
			} `json:"capabilities"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = id
		}
		entry := Entry{
			ID: id, DisplayName: label, ContextWindow: m.MaxInputTokens, MaxOutput: m.MaxTokens,
			OwnedBy: strings.TrimSpace(m.OwnedBy),
			RawJSON: append(json.RawMessage(nil), raw...),
		}
		// Set protocol based on owned_by (anthropic → Messages API, others → Chat Completions)
		if strings.EqualFold(m.OwnedBy, "anthropic") {
			entry.Protocol = "anthropic"
		} else {
			entry.Protocol = "openai"
		}
		// Extract capabilities
		entry.ThinkingEnabled = m.Capabilities.Thinking.Types.Enabled.Supported
		entry.ThinkingAdaptive = m.Capabilities.Thinking.Types.Adaptive.Supported
		if m.Capabilities.Effort.Supported {
			entry.EffortSupported = true
			var levels []string
			for _, lvl := range []string{"low", "medium", "high", "xhigh", "max"} {
				switch lvl {
				case "low":
					if m.Capabilities.Effort.Low.Supported {
						levels = append(levels, lvl)
					}
				case "medium":
					if m.Capabilities.Effort.Medium.Supported {
						levels = append(levels, lvl)
					}
				case "high":
					if m.Capabilities.Effort.High.Supported {
						levels = append(levels, lvl)
					}
				case "xhigh":
					if m.Capabilities.Effort.XHigh.Supported {
						levels = append(levels, lvl)
					}
				case "max":
					if m.Capabilities.Effort.Max.Supported {
						levels = append(levels, lvl)
					}
				}
			}
			entry.EffortLevels = strings.Join(levels, ",")
		}
		entry.StructuredOutput = m.Capabilities.StructuredOutputs.Supported
		entry.CodeExecution = m.Capabilities.CodeExecution.Supported
		entry.CitationsSupported = m.Capabilities.Citations.Supported
		entry.PDFInput = m.Capabilities.PDFInput.Supported
		entry.ImageInput = m.Capabilities.ImageInput.Supported
		// Context management (Concentrate-specific capability)
		if m.Capabilities.ContextManagement.Supported {
			entry.Features = append(entry.Features, "context_management")
			if m.Capabilities.ContextManagement.ClearThinking20251015.Supported {
				entry.Features = append(entry.Features, "context_management:clear_thinking")
			}
			if m.Capabilities.ContextManagement.ClearToolUses20250919.Supported {
				entry.Features = append(entry.Features, "context_management:clear_tool_uses")
			}
			if m.Capabilities.ContextManagement.Compact20260112.Supported {
				entry.Features = append(entry.Features, "context_management:compact")
			}
		}
		// Populate Features list for downstream catalog pipeline
		if entry.ThinkingEnabled {
			entry.Features = append(entry.Features, "thinking:enabled")
		}
		if entry.ThinkingAdaptive {
			entry.Features = append(entry.Features, "thinking:adaptive")
		}
		if entry.EffortSupported {
			entry.Features = append(entry.Features, "effort")
			if entry.EffortLevels != "" {
				entry.Features = append(entry.Features, "effort:"+entry.EffortLevels)
			}
		}
		if entry.StructuredOutput {
			entry.Features = append(entry.Features, "structured_output")
		}
		if entry.CodeExecution {
			entry.Features = append(entry.Features, "code_execution")
		}
		if entry.CitationsSupported {
			entry.Features = append(entry.Features, "citations")
		}
		if entry.PDFInput {
			entry.Features = append(entry.Features, "pdf_input")
		}
		if entry.ImageInput {
			entry.Features = append(entry.Features, "image_input")
		}
		entries = append(entries, entry)
	}
	// Update protocol map for dual-protocol routing (anthropic → Messages API, others → Chat Completions).
	protocolEntries := make([]struct{ ID, Protocol string }, 0, len(entries))
	for i := range entries {
		protocolEntries = append(protocolEntries, struct{ ID, Protocol string }{ID: entries[i].ID, Protocol: entries[i].Protocol})
	}
	concentrate.UpdateProtocolMap(protocolEntries)
	// Fetch precise pricing from individual model endpoints if requested (slower but accurate).
	// Pricing is cached to disk for 24 hours to avoid repeated API calls.
	if fetchPricing && !isCustomBaseURL {
		fetchConcentratePricing(context.Background(), baseURL, apiKey, entries)
	}
	return entries, nil
}

// fetchConcentratePricing fetches pricing from individual model endpoints with rate limiting.
// Pricing is cached to disk for 24 hours to avoid repeated API calls.
// Captures input, output, reasoning, and tiered pricing per the Concentrate API format.
func fetchConcentratePricing(ctx context.Context, baseURL, apiKey string, entries []Entry) {
	for i := range entries {
		// Check cache first
		if entry, ok := concentrate.GetPricing(entries[i].ID); ok {
			entries[i].InputPricePer1M = entry.InputPrice
			entries[i].OutputPricePer1M = entry.OutputPrice
			entries[i].TierThreshold = entry.TierThreshold
			entries[i].TieredInputPricePer1M = entry.TieredInputPrice
			entries[i].TieredOutputPricePer1M = entry.TieredOutputPrice
			continue
		}

		// Build request for individual model pricing
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models/"+entries[i].ID, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}
		var modelResp struct {
			Providers map[string]struct {
				Pricing struct {
					Tokens struct {
						Input struct {
							Price struct{ USD float64 }
							Units float64
						}
						Output struct {
							Price struct{ USD float64 }
							Units float64
						}
						Reasoning struct {
							Price struct{ USD float64 }
							Units float64
						}
						Tiers []struct {
							Above int `json:"above"`
							Input struct {
								Price struct{ USD float64 }
								Units float64
							}
							Output struct {
								Price struct{ USD float64 }
								Units float64
							}
						}
					}
				}
			}
		}
		_ = json.NewDecoder(resp.Body).Decode(&modelResp)
		resp.Body.Close()

		// pricePer1M converts a price with units to per-1M-token price.
		// If units is 1 (per-token), multiply by 1M.
		// If units is 1000 (per-1K-tokens), multiply by 1000.
		// If units is 0 or missing, assume per-token.
		pricePer1M := func(price, units float64) float64 {
			if units <= 0 {
				units = 1
			}
			return price * (1_000_000 / units)
		}

		// Use first available provider's pricing (usually "openai" or the primary provider)
		var cacheEntry concentrate.PricingCacheEntry
		if len(modelResp.Providers) > 0 {
			for _, p := range modelResp.Providers {
				if p.Pricing.Tokens.Input.Price.USD > 0 {
					cacheEntry.InputPrice = pricePer1M(p.Pricing.Tokens.Input.Price.USD, p.Pricing.Tokens.Input.Units)
					entries[i].InputPricePer1M = cacheEntry.InputPrice
				}
				if p.Pricing.Tokens.Output.Price.USD > 0 {
					cacheEntry.OutputPrice = pricePer1M(p.Pricing.Tokens.Output.Price.USD, p.Pricing.Tokens.Output.Units)
					entries[i].OutputPricePer1M = cacheEntry.OutputPrice
				}
				if p.Pricing.Tokens.Reasoning.Price.USD > 0 {
					cacheEntry.ReasoningPrice = pricePer1M(p.Pricing.Tokens.Reasoning.Price.USD, p.Pricing.Tokens.Reasoning.Units)
				}
				// Capture tiered pricing if present
				for _, tier := range p.Pricing.Tokens.Tiers {
					if tier.Above > 0 && tier.Input.Price.USD > 0 {
						cacheEntry.TierThreshold = tier.Above
						cacheEntry.TieredInputPrice = pricePer1M(tier.Input.Price.USD, tier.Input.Units)
						cacheEntry.TieredOutputPrice = pricePer1M(tier.Output.Price.USD, tier.Output.Units)
						entries[i].TierThreshold = tier.Above
						entries[i].TieredInputPricePer1M = cacheEntry.TieredInputPrice
						entries[i].TieredOutputPricePer1M = cacheEntry.TieredOutputPrice
						break
					}
				}
				break
			}
		}

		// Cache the pricing for future runs
		concentrate.SetPricing(entries[i].ID, cacheEntry)

		// Small delay to avoid rate limiting
		time.Sleep(50 * time.Millisecond)
	}
}

// FetchGroq lists models from the Groq OpenAI-compatible API.
func FetchGroq(env map[string]string) ([]Entry, error) {
	return fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "GROQ_BASE_URL", DefaultGroqBaseURL),
		env["GROQ_API_KEY"], "Bearer",
	)
}

// FetchClinePass returns a curated list of known ClinePass models.
// ClinePass does not expose a GET /models endpoint, so we return a
// static list based on official documentation at docs.cline.bot.
func FetchClinePass(env map[string]string) ([]Entry, error) {
	apiKey := strings.TrimSpace(env["CLINE_API_KEY"])
	if apiKey == "" {
		return nil, nil
	}
	now := time.Now().Unix()
	models := []struct {
		id, name, owner   string
		ctx, maxOut       int
		inPrice, outPrice float64 // per 1M tokens (ClinePass reference pricing), 0 = free
	}{
		{"cline-pass/deepseek-v4-pro", "DeepSeek V4 Pro", "deepseek", 131072, 8192, 1.74, 3.48},
		{"cline-pass/deepseek-v4-flash", "DeepSeek V4 Flash", "deepseek", 131072, 8192, 0.14, 0.28},
		{"cline-pass/glm-5.2", "GLM 5.2", "zhipu", 131072, 8192, 1.40, 4.40},
		{"cline-pass/kimi-k2.7-code", "Kimi K2.7 Code", "moonshot", 131072, 8192, 0.95, 4.00},
		{"cline-pass/kimi-k2.6", "Kimi K2.6", "moonshot", 131072, 8192, 0.95, 4.00},
		{"cline-pass/minimax-m3", "MiniMax M3", "minimax", 131072, 8192, 0.30, 1.20},
		{"cline-pass/mimo-v2.5-pro", "MiMo V2.5 Pro", "xiaomi", 131072, 8192, 1.74, 3.48},
		{"cline-pass/mimo-v2.5", "MiMo V2.5", "xiaomi", 131072, 8192, 0.14, 0.28},
		{"cline-pass/qwen3.7-max", "Qwen 3.7 Max", "qwen", 131072, 8192, 2.50, 7.50},
		{"cline-pass/qwen3.7-plus", "Qwen 3.7 Plus", "qwen", 131072, 8192, 0.40, 1.28},
		{"cline-pass/poolside-laguna-m.1-free", "Poolside Laguna M.1 (Free)", "poolside", 262144, 32768, 0, 0},
	}
	var entries []Entry
	for _, m := range models {
		entry, _ := entryFromOpenAICompatJSON(json.RawMessage(fmt.Sprintf(
			`{"id":%q,"name":%q,"owned_by":%q,"context_length":%d,"max_completion_tokens":%d,"created":%d}`,
			m.id, m.name, m.owner, m.ctx, m.maxOut, now,
		)))
		if entry.ID != "" {
			entry.InputPricePer1M = m.inPrice
			entry.OutputPricePer1M = m.outPrice
			entries = append(entries, entry)
		}
	}
	return entries, nil
}
