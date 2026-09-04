package live

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/catalog/opencodego"
)

// maxLiveResponseBytes caps how much of an external provider's HTTP response
// the live catalog fetchers will read, so a malicious or buggy provider cannot
// exhaust memory by returning an unbounded body.
const maxLiveResponseBytes = 10 * 1024 * 1024 // 10 MiB

// decodeJSONLimited decodes JSON from r into v, reading at most
// maxLiveResponseBytes. Use this instead of json.NewDecoder(resp.Body) for
// responses from untrusted/remote endpoints.
func decodeJSONLimited(r io.Reader, v any) error {
	return json.NewDecoder(io.LimitReader(r, maxLiveResponseBytes)).Decode(v)
}

// Provider FetchFunc implementations live in fetchers_cloud.go and
// fetchers_providers.go; this file holds the registry, shared parsing/pricing
// helpers, and AWS SigV4 signing helpers.

var httpClient = &http.Client{Timeout: 30 * time.Second}

const (
	DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	DefaultCanopyWaveBaseURL = "https://inference.canopywave.io/v1"
	DefaultZAIBaseURL        = "https://api.z.ai/api/paas/v4"
	DefaultZAICodingBaseURL  = "https://api.z.ai/api/coding/paas/v4"
	DefaultOpenAIBaseURL     = "https://api.openai.com/v1"
	DefaultGrokBaseURL       = "https://api.x.ai/v1"
	DefaultOpenCodeGoBaseURL = opencodego.DefaultBaseURL
	DefaultKimiBaseURL       = "https://api.moonshot.ai/v1"
	DefaultXiaomiBaseURL     = "https://api.xiaomimimo.com/v1"
	DefaultMiniMaxBaseURL    = "https://api.minimax.io/v1"
	DefaultFireworksBaseURL  = "https://api.fireworks.ai/inference/v1"
)

// FetchFunc lists models from a live provider API.
type FetchFunc func(env map[string]string) ([]Entry, error)

const (
	DefaultDeepSeekBaseURL  = "https://api.deepseek.com"
	DefaultPoolsideBaseURL  = "https://inference.poolside.ai/v1"
	DefaultGroqBaseURL      = "https://api.groq.com/openai/v1"
	DefaultClinePassBaseURL = "https://api.cline.bot/api/v1" // #nosec G101 -- public API base URL, not a secret value
)

// Registry maps fetcher keys to implementations.
var Registry = map[string]FetchFunc{
	"anthropic":              FetchAnthropic,
	"openai":                 FetchOpenAI,
	"azure":                  FetchAzure,
	"gemini":                 FetchGemini,
	"bedrock":                FetchBedrock,
	"vertex":                 FetchVertex,
	"openrouter":             FetchOpenRouter,
	"grok":                   FetchGrok,
	"zai_payg":               FetchZAI,
	"zai_coding":             FetchZAICoding,
	"concentrate":            FetchConcentrate,
	"opengateway":            FetchOpenGateway,
	"agnes":                  FetchAgnes,
	"longcat":                FetchLongCat,
	"fireworks":              FetchFireworks,
	"canopywave":             FetchCanopyWave,
	"opencodego":             FetchOpenCodeGo,
	"kimi":                   FetchKimi,
	"xiaomi_mimo_payg":       FetchXiaomiPayg,
	"xiaomi_mimo_token_plan": FetchXiaomiTokenPlan,
	"minimax_token_plan":     FetchMiniMaxTokenPlan,
	"minimax_payg":           FetchMiniMaxPayg,
	"ollama":                 FetchOllama,
	"deepseek":               FetchDeepSeek,
	"poolside":               FetchPoolside,
	"groq":                   FetchGroq,
	"clinepass":              FetchClinePass,
	"stepfun":                FetchStepFun,
}

// Fetch runs a registered live fetcher.
func Fetch(key string, env map[string]string) ([]Entry, error) {
	fn, ok := Registry[key]
	if !ok {
		return nil, fmt.Errorf("live: unknown fetcher %q", key)
	}
	return fn(env)
}

type listModelJSON struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Title                string   `json:"title"`
	DisplayName          string   `json:"display_name"`
	Description          string   `json:"description"`
	ContextLength        *int     `json:"context_length"`
	ContextSize          *int     `json:"context_size"`
	ContextWindow        *int     `json:"context_window"` // LongCat and similar OpenAI-compat APIs
	MaxCompletionTokens  *int     `json:"max_completion_tokens"`
	MaxOutputTokens      *int     `json:"max_output_tokens"`
	MaxInputTokens       *int     `json:"max_input_tokens"`
	MaxTokens            *int     `json:"max_tokens"`
	InputTokenPricePerM  *float64 `json:"input_token_price_per_m"`
	OutputTokenPricePerM *float64 `json:"output_token_price_per_m"`
	Features             []string `json:"features"`
	SupportedFeatures    []string `json:"supported_features"`
	Tags                 []string `json:"tags"`
	OwnedBy              string   `json:"owned_by"`
	Status               *int     `json:"status"`
	// APIType captures protocol hints from the provider (e.g., "anthropic", "openai").
	// Not all providers return this; empty means unknown.
	APIType string `json:"api_type,omitempty"`
	Pricing *struct {
		Prompt      interface{} `json:"prompt"`
		Completion  interface{} `json:"completion"`
		CachedRead  interface{} `json:"cached_read,omitempty"`
		CachedWrite interface{} `json:"cached_write,omitempty"`
		Request     interface{} `json:"request,omitempty"`
		Image       interface{} `json:"image,omitempty"`
	} `json:"pricing"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	ContextLength *int   `json:"context_length"`
	TopProvider   *struct {
		ContextLength       *int `json:"context_length"`
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	} `json:"top_provider"`
	Pricing *struct {
		Prompt     interface{} `json:"prompt"`
		Completion interface{} `json:"completion"`
	} `json:"pricing"`
}

func asFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

func ownerFromModelID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return ""
}

func fetchOpenAICompatModels(ctx context.Context, baseURL, apiKey, authHeader string) ([]Entry, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, nil
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("live: missing base URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("live: create request: %w", err)
	}
	if authHeader == "x-api-key" {
		req.Header.Set("x-api-key", apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "graycode-router-model-catalog/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("live model fetch failed (%d)", resp.StatusCode)
	}

	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, &payload); err != nil {
		return nil, err
	}
	var entries []Entry
	for _, raw := range payload.Data {
		entry, ok := entryFromOpenAICompatJSON(raw)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func intOrFirst(def int, vals ...*int) int {
	for _, p := range vals {
		if p != nil && *p > 0 {
			return *p
		}
	}
	return def
}

func entryFromOpenAICompatJSON(raw json.RawMessage) (Entry, bool) {
	var m listModelJSON
	if err := json.Unmarshal(raw, &m); err != nil {
		return Entry{}, false
	}
	id := strings.TrimSpace(m.ID)
	if id == "" {
		return Entry{}, false
	}
	if m.Status != nil && *m.Status <= 0 {
		return Entry{}, false
	}
	inPrice, outPrice := pricingFromListModel(m)
	cachedRead, cachedWrite := cachedPricingFromListModel(m)
	label := strings.TrimSpace(m.DisplayName)
	if label == "" {
		label = strings.TrimSpace(m.Title)
	}
	if label == "" {
		label = strings.TrimSpace(m.Name)
	}
	if label == "" {
		label = id
	}
	features := append([]string(nil), m.Features...)
	for _, feature := range m.SupportedFeatures {
		features = appendUnique(features, feature)
	}
	owner := strings.TrimSpace(m.OwnedBy)
	if owner == "" {
		owner = ownerFromModelID(id)
	}
	// Derive protocol from api_type field or features/tags hints.
	protocol := protocolFromMetadata(m.APIType, features, m.Tags)
	return Entry{
		ID: id, DisplayName: label, Description: strings.TrimSpace(m.Description), OwnedBy: owner,
		InputPricePer1M: inPrice, OutputPricePer1M: outPrice,
		CachedReadPricePer1M:  cachedRead,
		CachedWritePricePer1M: cachedWrite,
		ContextWindow:         intOrFirst(0, m.ContextLength, m.ContextSize, m.ContextWindow, m.MaxInputTokens),
		MaxOutput:             intOrFirst(0, m.MaxCompletionTokens, m.MaxOutputTokens, m.MaxTokens),
		Features:              features,
		Protocol:              protocol,
		RawJSON:               append(json.RawMessage(nil), raw...),
	}, true
}

// protocolFromMetadata derives the API protocol from available metadata.
// Returns "anthropic", "openai", or "" if unknown.
func protocolFromMetadata(apiType string, features, tags []string) string {
	apiType = strings.ToLower(strings.TrimSpace(apiType))
	if apiType == "anthropic" || apiType == "openai" {
		return apiType
	}
	// Check features and tags for protocol hints.
	for _, f := range append(features, tags...) {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "anthropic" || f == "anthropic-compat" || f == "anthropic-messages" {
			return "anthropic"
		}
		if f == "openai" || f == "openai-compat" || f == "openai-chat-completions" {
			return "openai"
		}
	}
	return ""
}

func pricingFromListModel(m listModelJSON) (inPrice, outPrice float64) {
	if m.InputTokenPricePerM != nil {
		inPrice = *m.InputTokenPricePerM
	}
	if m.OutputTokenPricePerM != nil {
		outPrice = *m.OutputTokenPricePerM
	}
	if inPrice > 0 || outPrice > 0 {
		return inPrice, outPrice
	}
	if m.Pricing != nil {
		inPrice = asFloat(m.Pricing.Prompt) * 1_000_000
		outPrice = asFloat(m.Pricing.Completion) * 1_000_000
	}
	return inPrice, outPrice
}

// cachedPricingFromListModel extracts cached read/write pricing from the model JSON.
// Returns (cachedRead, cachedWrite) per 1M tokens.
func cachedPricingFromListModel(m listModelJSON) (cachedRead, cachedWrite float64) {
	if m.Pricing == nil {
		return 0, 0
	}
	cachedRead = asFloat(m.Pricing.CachedRead) * 1_000_000
	cachedWrite = asFloat(m.Pricing.CachedWrite) * 1_000_000
	return cachedRead, cachedWrite
}

func envOr(env map[string]string, key, def string) string {
	if v := strings.TrimSpace(env[key]); v != "" {
		return v
	}
	return def
}

func firstEnv(env map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(env[key]); v != "" {
			return v
		}
	}
	return ""
}

// AWS SigV4 signing helpers shared by cloud-provider fetchers.
func signAWSV4(req *http.Request, accessKeyID, secretAccessKey, sessionToken, region, service string, body []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	payloadHash := sha256Hex(body)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}
	req.Header.Set("Host", req.URL.Host)
	canonicalHeaders, signedHeaders := canonicalAWSHeaders(req.Header)
	canonicalQuery := req.URL.Query().Encode()
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(awsSigningKey(secretAccessKey, dateStamp, region, service), []byte(stringToSign)))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKeyID, scope, signedHeaders, signature))
}

func canonicalAWSHeaders(headers http.Header) (string, string) {
	var names []string
	for name := range headers {
		names = append(names, strings.ToLower(name))
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		values := headers.Values(name)
		for i, v := range values {
			values[i] = strings.Join(strings.Fields(v), " ")
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(strings.Join(values, ","))
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

func awsSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
