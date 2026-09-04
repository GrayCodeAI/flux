package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OpenAI/OpenRouter enrichment and cloud-provider (Azure, Bedrock, Vertex,
// MiniMax) fetchers. Split out of fetchers.go for clarity.
func FetchOpenAI(env map[string]string) ([]Entry, error) {
	entries, err := fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "OPENAI_BASE_URL", DefaultOpenAIBaseURL),
		env["OPENAI_API_KEY"], "Bearer",
	)
	if err != nil {
		return nil, err
	}
	// Enrich with capabilities from OpenRouter (context window, pricing, supported parameters)
	enrichOpenAIWithOpenRouter(entries)
	return entries, nil
}

// enrichOpenAIWithOpenRouter fetches OpenRouter's model list and enriches OpenAI entries
// with context window, pricing, and capability data that OpenAI's own API doesn't return.
func enrichOpenAIWithOpenRouter(entries []Entry) {
	if len(entries) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DefaultOpenRouterBaseURL+"/models", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var payload struct {
		Data []openRouterModelEntry `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return
	}
	// Build lookup map: "gpt-4o" → openRouterModelEntry
	lookup := map[string]openRouterModelEntry{}
	for _, m := range payload.Data {
		// OpenRouter IDs are "openai/gpt-4o" — strip prefix
		nativeID := strings.TrimPrefix(m.ID, "openai/")
		if nativeID != m.ID {
			lookup[nativeID] = m
		}
	}
	// Enrich entries
	for i := range entries {
		or, ok := lookup[entries[i].ID]
		if !ok {
			continue
		}
		if or.ContextLength > 0 {
			entries[i].ContextWindow = or.ContextLength
			entries[i].MaxInputTokens = or.ContextLength
		}
		if or.TopProvider.MaxCompletionTokens > 0 {
			entries[i].MaxOutput = or.TopProvider.MaxCompletionTokens
		}
		if p, err := strconv.ParseFloat(or.Pricing.Prompt, 64); err == nil && p > 0 {
			entries[i].InputPricePer1M = p * 1_000_000
		}
		if p, err := strconv.ParseFloat(or.Pricing.Completion, 64); err == nil && p > 0 {
			entries[i].OutputPricePer1M = p * 1_000_000
		}
		// Map supported parameters to features
		features := map[string]bool{}
		for _, sp := range or.SupportedParameters {
			features[sp] = true
		}
		if features["tools"] || features["functions"] {
			entries[i].Features = append(entries[i].Features, "tools")
		}
		if features["reasoning_effort"] {
			entries[i].Features = append(entries[i].Features, "thinking:enabled")
			entries[i].ThinkingEnabled = true
		}
		if features["response_format"] {
			entries[i].Features = append(entries[i].Features, "structured_output")
			entries[i].StructuredOutput = true
		}
		if features["temperature"] {
			entries[i].Features = appendUnique(entries[i].Features, "temperature")
		}
		if features["presence_penalty"] || features["frequency_penalty"] {
			entries[i].Features = appendUnique(entries[i].Features, "penalties")
		}
		if or.ContextLength > 0 {
			entries[i].Features = appendUnique(entries[i].Features, fmt.Sprintf("context:%d", or.ContextLength))
		}
	}
}

// enrichFromOpenRouter fetches OpenRouter's model list and enriches entries
// with pricing and context data. prefix is the OpenRouter provider prefix (e.g., "moonshotai/").
func enrichFromOpenRouter(entries []Entry, prefix string) {
	if len(entries) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DefaultOpenRouterBaseURL+"/models", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var payload struct {
		Data []openRouterModelEntry `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return
	}
	// Build lookup map by stripping prefix
	lookup := map[string]openRouterModelEntry{}
	for _, m := range payload.Data {
		nativeID := strings.TrimPrefix(m.ID, prefix)
		if nativeID != m.ID {
			lookup[nativeID] = m
		}
	}
	// Enrich entries
	for i := range entries {
		or, ok := lookup[entries[i].ID]
		if !ok {
			continue
		}
		if or.ContextLength > 0 && entries[i].ContextWindow == 0 {
			entries[i].ContextWindow = or.ContextLength
		}
		if or.TopProvider.MaxCompletionTokens > 0 && entries[i].MaxOutput == 0 {
			entries[i].MaxOutput = or.TopProvider.MaxCompletionTokens
		}
		if p, err := strconv.ParseFloat(or.Pricing.Prompt, 64); err == nil && p > 0 && entries[i].InputPricePer1M == 0 {
			entries[i].InputPricePer1M = p * 1_000_000
		}
		if p, err := strconv.ParseFloat(or.Pricing.Completion, 64); err == nil && p > 0 && entries[i].OutputPricePer1M == 0 {
			entries[i].OutputPricePer1M = p * 1_000_000
		}
	}
}

type openRouterModelEntry struct {
	ID                  string   `json:"id"`
	ContextLength       int      `json:"context_length"`
	SupportedParameters []string `json:"supported_parameters"`
	TopProvider         struct {
		MaxCompletionTokens int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func FetchMiniMaxTokenPlan(env map[string]string) ([]Entry, error) {
	return fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "MINIMAX_TOKEN_PLAN_BASE_URL", DefaultMiniMaxBaseURL),
		env["MINIMAX_TOKEN_PLAN_API_KEY"], "Bearer",
	)
}

func FetchMiniMaxPayg(env map[string]string) ([]Entry, error) {
	return fetchOpenAICompatModels(
		context.Background(),
		envOr(env, "MINIMAX_PAYG_BASE_URL", DefaultMiniMaxBaseURL),
		env["MINIMAX_PAYG_API_KEY"], "Bearer",
	)
}

func FetchAzure(env map[string]string) ([]Entry, error) {
	if id := firstEnv(env, "AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_MODEL", "OPENAI_MODEL"); id != "" {
		return []Entry{{ID: id, DisplayName: id}}, nil
	}
	token := firstEnv(env, "AZURE_OPENAI_MANAGEMENT_TOKEN", "AZURE_ACCESS_TOKEN")
	subscriptionID := strings.TrimSpace(env["AZURE_SUBSCRIPTION_ID"])
	resourceGroup := strings.TrimSpace(env["AZURE_RESOURCE_GROUP"])
	accountName := firstEnv(env, "AZURE_OPENAI_ACCOUNT_NAME", "AZURE_OPENAI_ACCOUNT")
	if token == "" || subscriptionID == "" || resourceGroup == "" || accountName == "" {
		return nil, nil
	}
	apiVersion := envOr(env, "AZURE_OPENAI_MANAGEMENT_API_VERSION", "2024-10-01")
	path := fmt.Sprintf("https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.CognitiveServices/accounts/%s/deployments",
		url.PathEscape(subscriptionID), url.PathEscape(resourceGroup), url.PathEscape(accountName))
	reqURL := path + "?api-version=" + url.QueryEscape(apiVersion)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("live: create azure request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure deployment fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		Value []json.RawMessage `json:"value"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Value {
		entry, ok := entryFromAzureDeploymentJSON(raw)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func entryFromAzureDeploymentJSON(raw json.RawMessage) (Entry, bool) {
	var dep struct {
		Name       string `json:"name"`
		Properties struct {
			Model struct {
				Name    string `json:"name"`
				Format  string `json:"format"`
				Version string `json:"version"`
			} `json:"model"`
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &dep); err != nil {
		return Entry{}, false
	}
	id := strings.TrimSpace(dep.Name)
	if id == "" || !strings.EqualFold(strings.TrimSpace(dep.Properties.ProvisioningState), "Succeeded") {
		return Entry{}, false
	}
	label := id
	if model := strings.TrimSpace(dep.Properties.Model.Name); model != "" {
		label = id + " (" + model + ")"
	}
	return Entry{ID: id, DisplayName: label, OwnedBy: "azure", RawJSON: append(json.RawMessage(nil), raw...)}, true
}

func FetchBedrock(env map[string]string) ([]Entry, error) {
	accessKeyID := strings.TrimSpace(env["AWS_ACCESS_KEY_ID"])
	secretAccessKey := strings.TrimSpace(env["AWS_SECRET_ACCESS_KEY"])
	region := firstEnv(env, "AWS_REGION", "AWS_DEFAULT_REGION")
	if accessKeyID == "" || secretAccessKey == "" || region == "" {
		return nil, nil
	}
	reqURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/foundation-models?byProvider=Anthropic", region)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("live: create bedrock request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")
	signAWSV4(req, accessKeyID, secretAccessKey, strings.TrimSpace(env["AWS_SESSION_TOKEN"]), region, "bedrock", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bedrock model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		ModelSummaries []json.RawMessage `json:"modelSummaries"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.ModelSummaries {
		entry, ok := entryFromBedrockModelJSON(raw)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func entryFromBedrockModelJSON(raw json.RawMessage) (Entry, bool) {
	var m struct {
		ModelID                    string   `json:"modelId"`
		ModelName                  string   `json:"modelName"`
		ProviderName               string   `json:"providerName"`
		ResponseStreamingSupported bool     `json:"responseStreamingSupported"`
		InputModalities            []string `json:"inputModalities"`
		OutputModalities           []string `json:"outputModalities"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return Entry{}, false
	}
	id := strings.TrimSpace(m.ModelID)
	if id == "" {
		return Entry{}, false
	}
	if provider := strings.TrimSpace(m.ProviderName); provider != "" && !strings.EqualFold(provider, "Anthropic") {
		return Entry{}, false
	}
	label := strings.TrimSpace(m.ModelName)
	if label == "" {
		label = id
	}
	features := append([]string(nil), m.InputModalities...)
	features = append(features, m.OutputModalities...)
	if m.ResponseStreamingSupported {
		features = append(features, "streaming")
	}
	return Entry{ID: id, DisplayName: label, OwnedBy: "anthropic", Features: features, RawJSON: append(json.RawMessage(nil), raw...)}, true
}

func FetchVertex(env map[string]string) ([]Entry, error) {
	projectID := strings.TrimSpace(env["VERTEX_PROJECT_ID"])
	region := strings.TrimSpace(env["VERTEX_REGION"])
	token := firstEnv(env, "VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN")
	if projectID == "" || region == "" || token == "" {
		return nil, nil
	}
	// Fetch Anthropic models from Vertex AI (not Google's own models)
	reqURL := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models",
		region, url.PathEscape(projectID), url.PathEscape(region))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("live: create vertex request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vertex model fetch failed (%d)", resp.StatusCode)
	}
	var payload struct {
		PublisherModels []json.RawMessage `json:"publisherModels"`
		Models          []json.RawMessage `json:"models"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	rawModels := payload.PublisherModels
	if len(rawModels) == 0 {
		rawModels = payload.Models
	}
	var entries []Entry
	for _, raw := range rawModels {
		entry, ok := entryFromVertexModelJSON(raw)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func entryFromVertexModelJSON(raw json.RawMessage) (Entry, bool) {
	var m struct {
		Name             string   `json:"name"`
		DisplayName      string   `json:"displayName"`
		Description      string   `json:"description"`
		VersionID        string   `json:"versionId"`
		Frameworks       []string `json:"frameworks"`
		SupportedActions []string `json:"supportedActions"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return Entry{}, false
	}
	id := strings.TrimSpace(m.Name)
	if id == "" {
		return Entry{}, false
	}
	if i := strings.LastIndex(id, "/models/"); i >= 0 {
		id = id[i+len("/models/"):]
	}
	label := strings.TrimSpace(m.DisplayName)
	if label == "" {
		label = id
	}
	features := append([]string(nil), m.Frameworks...)
	// Tag supported actions as features
	for _, action := range m.SupportedActions {
		features = appendUnique(features, "action:"+action)
	}
	return Entry{
		ID:          id,
		DisplayName: label,
		Description: strings.TrimSpace(m.Description),
		OwnedBy:     "anthropic",
		Features:    features,
		RawJSON:     append(json.RawMessage(nil), raw...),
	}, true
}
