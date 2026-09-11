package xiaomi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultPlatformModelsURL is the public MiMo platform catalog (context, pricing, names).
// Inference GET {base}/models only returns id/object/owned_by.
const DefaultPlatformModelsURL = "https://platform.xiaomimimo.com/api/v1/models"

// maxPlatformResponseBytes caps how much of the platform catalog response is
// read, so a malicious or misconfigured endpoint (including the env-overridable
// XIAOMI_MIMO_PLATFORM_MODELS_URL) cannot exhaust memory via an unbounded body.
const maxPlatformResponseBytes = 10 * 1024 * 1024 // 10 MiB

var platformHTTPClient = &http.Client{Timeout: 30 * time.Second}

// PlatformModel is one row from the platform catalog API.
type PlatformModel struct {
	ID               string
	Name             string
	Description      string
	ContextLength    int
	MaxOutputLength  int
	InputPricePer1M  float64
	OutputPricePer1M float64
	Raw              json.RawMessage
}

// PlatformModelsURLFromEnv returns override URL for tests, else DefaultPlatformModelsURL.
func PlatformModelsURLFromEnv(env map[string]string) string {
	if env != nil {
		if v := strings.TrimSpace(env["XIAOMI_MIMO_PLATFORM_MODELS_URL"]); v != "" {
			return v
		}
	}
	return DefaultPlatformModelsURL
}

// NativeModelID strips vendor prefix (xiaomi/mimo-v2.5 → mimo-v2.5).
func NativeModelID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// FetchPlatformModelsIndex GETs the platform catalog and indexes by native model id.
func FetchPlatformModelsIndex(ctx context.Context, catalogURL string) (map[string]PlatformModel, error) {
	catalogURL = strings.TrimSpace(catalogURL)
	if catalogURL == "" {
		catalogURL = DefaultPlatformModelsURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "eyrie-model-catalog/1.0")

	resp, err := platformHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xiaomi platform catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPlatformResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("xiaomi platform catalog: HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make(map[string]PlatformModel, len(payload.Data))
	for _, raw := range payload.Data {
		pm, ok := platformModelFromJSON(raw)
		if !ok {
			continue
		}
		key := NativeModelID(pm.ID)
		if key == "" {
			continue
		}
		out[key] = pm
	}
	return out, nil
}

func platformModelFromJSON(raw json.RawMessage) (PlatformModel, bool) {
	var row struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		ContextLength   int    `json:"context_length"`
		MaxOutputLength int    `json:"max_output_length"`
		TopProvider     *struct {
			ContextLength int `json:"context_length"`
		} `json:"top_provider"`
		Pricing json.RawMessage `json:"pricing"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return PlatformModel{}, false
	}
	id := strings.TrimSpace(row.ID)
	if id == "" {
		return PlatformModel{}, false
	}
	ctx := row.ContextLength
	if ctx <= 0 && row.TopProvider != nil && row.TopProvider.ContextLength > 0 {
		ctx = row.TopProvider.ContextLength
	}
	in, out := pricingPer1MFromRaw(row.Pricing)
	return PlatformModel{
		ID:               id,
		Name:             strings.TrimSpace(row.Name),
		Description:      strings.TrimSpace(row.Description),
		ContextLength:    ctx,
		MaxOutputLength:  row.MaxOutputLength,
		InputPricePer1M:  in,
		OutputPricePer1M: out,
		Raw:              append(json.RawMessage(nil), raw...),
	}, true
}

func pricingPer1MFromRaw(raw json.RawMessage) (in, out float64) {
	if len(raw) == 0 {
		return 0, 0
	}
	var single struct {
		Prompt     interface{} `json:"prompt"`
		Completion interface{} `json:"completion"`
	}
	if json.Unmarshal(raw, &single) == nil && (single.Prompt != nil || single.Completion != nil) {
		return asFloat(single.Prompt) * 1_000_000, asFloat(single.Completion) * 1_000_000
	}
	var tiers []struct {
		Prompt     interface{} `json:"prompt"`
		Completion interface{} `json:"completion"`
		MinContext *int        `json:"min_context"`
	}
	if json.Unmarshal(raw, &tiers) != nil || len(tiers) == 0 {
		return 0, 0
	}
	best := tiers[0]
	bestMin := -1
	if best.MinContext != nil {
		bestMin = *best.MinContext
	}
	for _, t := range tiers[1:] {
		minCtx := -1
		if t.MinContext != nil {
			minCtx = *t.MinContext
		}
		if bestMin < 0 || (minCtx >= 0 && minCtx < bestMin) {
			best = t
			bestMin = minCtx
		}
	}
	return asFloat(best.Prompt) * 1_000_000, asFloat(best.Completion) * 1_000_000
}

func asFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		var f float64
		_, _ = fmt.Sscanf(n, "%f", &f)
		return f
	}
	return 0
}

// ApplyPlatformMetadata fills empty live fields from platform catalog; stores platform JSON as metadata when matched.
func ApplyPlatformMetadata(
	id, displayName, description string,
	ctx, maxOut int,
	inPrice, outPrice float64,
	inferenceRaw json.RawMessage,
	platform map[string]PlatformModel,
) (string, string, int, int, float64, float64, json.RawMessage) {
	if len(platform) == 0 {
		return displayName, description, ctx, maxOut, inPrice, outPrice, inferenceRaw
	}
	pm, ok := platform[NativeModelID(id)]
	if !ok {
		return displayName, description, ctx, maxOut, inPrice, outPrice, inferenceRaw
	}
	if strings.TrimSpace(displayName) == "" || displayName == id {
		if pm.Name != "" {
			displayName = pm.Name
		}
	}
	if description == "" && pm.Description != "" {
		description = pm.Description
	}
	if ctx <= 0 && pm.ContextLength > 0 {
		ctx = pm.ContextLength
	}
	if maxOut <= 0 && pm.MaxOutputLength > 0 {
		maxOut = pm.MaxOutputLength
	}
	if inPrice <= 0 && outPrice <= 0 {
		inPrice, outPrice = pm.InputPricePer1M, pm.OutputPricePer1M
	}
	meta := inferenceRaw
	if len(pm.Raw) > 0 {
		meta = pm.Raw
	}
	return displayName, description, ctx, maxOut, inPrice, outPrice, meta
}
