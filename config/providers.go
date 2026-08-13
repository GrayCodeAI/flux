package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog/opencodego"
)

// VertexGeminiBaseURL returns the Google Vertex AI publisher endpoint for Gemini.
func VertexGeminiBaseURL(projectID, region string) string {
	projectID = strings.TrimSpace(projectID)
	region = strings.TrimSpace(region)
	if projectID == "" || region == "" {
		return ""
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google", region, projectID, region)
}

// Default base URLs for each provider.
const (
	DefaultOpenAIBaseURL            = "https://api.openai.com/v1"
	DefaultOpenRouterOpenAIBaseURL  = "https://openrouter.ai/api/v1"
	DefaultConcentrateOpenAIBaseURL = "https://api.concentrate.ai/v1"
	DefaultOpenGatewayOpenAIBaseURL = "https://opengateway.gitlawb.com/v1"
	DefaultCanopyWaveOpenAIBaseURL  = "https://inference.canopywave.io/v1"
	DefaultZAIOpenAIBaseURL         = "https://api.z.ai/api/paas/v4"
	DefaultZAICodingOpenAIBaseURL   = "https://api.z.ai/api/coding/paas/v4"
	DefaultGeminiOpenAIBaseURL      = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultAnthropicOpenAIBaseURL   = "https://api.anthropic.com/v1"
	DefaultGrokOpenAIBaseURL        = "https://api.x.ai/v1"
	DefaultOpenCodeGoBaseURL        = opencodego.DefaultBaseURL
	DefaultKimiOpenAIBaseURL        = "https://api.moonshot.ai/v1"
	DefaultXiaomiOpenAIBaseURL      = "https://api.xiaomimimo.com/v1"
	DefaultMiniMaxOpenAIBaseURL     = "https://api.minimax.io/v1"
	DefaultGroqOpenAIBaseURL        = "https://api.groq.com/openai/v1"
	DefaultPoolsideOpenAIBaseURL    = "https://inference.poolside.ai/v1"
	DefaultClinePassOpenAIBaseURL   = "https://api.cline.bot/api/v1" // #nosec G101 -- public API base URL, not a secret value
	DefaultStepFunOpenAIBaseURL     = "https://api.stepfun.ai/v1"
	DefaultAgnesOpenAIBaseURL       = "https://apihub.agnes-ai.com/v1"
	DefaultLongCatOpenAIBaseURL     = "https://api.longcat.chat/openai/v1"
	DefaultLongCatAnthropicBaseURL  = "https://api.longcat.chat/anthropic"
	DefaultFireworksOpenAIBaseURL   = "https://api.fireworks.ai/inference/v1"
)

// ProviderTransport is the transport type for provider requests.
type ProviderTransport string

const TransportChatCompletions ProviderTransport = "chat_completions"

// ReasoningEffort levels.
type ReasoningEffort string

const (
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
)

// ResolvedProviderRequest holds the resolved provider request details.
type ResolvedProviderRequest struct {
	Transport      ProviderTransport `json:"transport"`
	RequestedModel string            `json:"requested_model"`
	ResolvedModel  string            `json:"resolved_model"`
	BaseURL        string            `json:"base_url"`
	Reasoning      *struct {
		Effort ReasoningEffort `json:"effort"`
	} `json:"reasoning,omitempty"`
}

// IsLocalProviderURL checks if a base URL points to localhost.
func IsLocalProviderURL(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// ResolveProviderRequest resolves model/baseUrl/transport from options and env.
func ResolveProviderRequest(model, baseURL, fallbackModel string) ResolvedProviderRequest {
	requested := strings.TrimSpace(model)
	if requested == "" {
		requested = strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	}
	if requested == "" {
		requested = strings.TrimSpace(fallbackModel)
	}
	if requested == "" {
		requested = "gpt-4o"
	}

	resolvedModel := requested
	var reasoning *struct {
		Effort ReasoningEffort `json:"effort"`
	}
	if idx := strings.Index(requested, "?"); idx != -1 {
		resolvedModel = strings.TrimSpace(requested[:idx])
		params, _ := url.ParseQuery(requested[idx+1:])
		if r := strings.TrimSpace(strings.ToLower(params.Get("reasoning"))); r == "low" || r == "medium" || r == "high" {
			reasoning = &struct {
				Effort ReasoningEffort `json:"effort"`
			}{Effort: ReasoningEffort(r)}
		}
	}

	rawBase := baseURL
	if rawBase == "" {
		rawBase = os.Getenv("OPENAI_BASE_URL")
	}
	if rawBase == "" {
		rawBase = os.Getenv("OPENAI_API_BASE")
	}
	if rawBase == "" {
		rawBase = DefaultOpenAIBaseURL
	}
	rawBase = strings.TrimRight(rawBase, "/")

	return ResolvedProviderRequest{
		Transport:      TransportChatCompletions,
		RequestedModel: requested,
		ResolvedModel:  resolvedModel,
		BaseURL:        rawBase,
		Reasoning:      reasoning,
	}
}
