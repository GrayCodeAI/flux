package client

import (
	"context"
	"net/http"
	"time"

	"github.com/GrayCodeAI/graycode-router/client/adapters"
	"github.com/GrayCodeAI/graycode-router/client/core"
	"github.com/GrayCodeAI/graycode-router/client/embeddings"
	"github.com/GrayCodeAI/graycode-router/llm"
)

// The provider contract and request/response data types live in
// client/core so subpackages can share them without importing this facade
// (see plans/client-package-decomposition.md). The aliases below keep the
// long-standing client.* names as the public API — they are the same types,
// not copies, so existing code and type assertions are unaffected.

type (
	// Provider is the core interface for LLM providers.
	Provider = core.Provider
	// GraycodeRouterConfig holds client configuration.
	GraycodeRouterConfig = core.GraycodeRouterConfig
	// ContentPart represents a piece of content in a multi-modal message.
	ContentPart = core.ContentPart
	// ImageURLPart represents an image content part.
	ImageURLPart = core.ImageURLPart
	// InputAudioPart represents an audio content part (base64 encoded).
	InputAudioPart = core.InputAudioPart
	// GraycodeRouterMessage represents a chat message.
	GraycodeRouterMessage = core.GraycodeRouterMessage
	// ToolResult represents the result of a tool execution.
	ToolResult = core.ToolResult
	// GraycodeRouterTool represents a tool definition.
	GraycodeRouterTool = core.GraycodeRouterTool
	// GraycodeRouterUsage tracks token usage.
	GraycodeRouterUsage = core.GraycodeRouterUsage
	// GraycodeRouterResponse is the response from a chat call.
	GraycodeRouterResponse = core.GraycodeRouterResponse
	// ToolCall represents a tool invocation.
	ToolCall = core.ToolCall
	// GraycodeRouterStreamEvent is a streaming event.
	GraycodeRouterStreamEvent = core.GraycodeRouterStreamEvent
	// StreamResult wraps a streaming response with cleanup.
	StreamResult = core.StreamResult
	// ResponseFormat specifies the desired output format for the model response.
	ResponseFormat = core.ResponseFormat
	// ChatOptions holds options for a chat request.
	ChatOptions = core.ChatOptions
	// ToolChoiceOption controls how the model uses tools (Anthropic).
	ToolChoiceOption = core.ToolChoiceOption
	// ContinuationConfig controls output continuation behavior.
	ContinuationConfig = core.ContinuationConfig
	// GraycodeRouterError is a structured error that preserves provider context,
	// HTTP metadata, and request identification for debugging.
	GraycodeRouterError = core.GraycodeRouterError
	// RetryConfig controls retry behavior for HTTP clients.
	RetryConfig = core.RetryConfig
	// SSEEvent is one server-sent event from a streaming response body.
	SSEEvent = core.SSEEvent
	// RepeatDetector detects degenerate repeating output in streamed text.
	RepeatDetector = core.RepeatDetector
	// ResponseHealth classifies whether a provider response carried usable output.
	ResponseHealth = core.ResponseHealth
	// ResponseSignals are the observations needed to classify response health.
	ResponseSignals = core.ResponseSignals
	// GuardrailType classifies a guardrail rule.
	GuardrailType = core.GuardrailType
	// GuardrailAction is what happens when a guardrail rule matches.
	GuardrailAction = core.GuardrailAction
	// GuardrailSeverity ranks how serious a violation is.
	GuardrailSeverity = core.GuardrailSeverity
	// GuardrailRule is one output-filtering rule.
	GuardrailRule = core.GuardrailRule
	// GuardrailViolation records a rule match in a response.
	GuardrailViolation = core.GuardrailViolation
	// GuardrailError is returned when a blocking rule matches.
	GuardrailError = core.GuardrailError
	// Guardrails is a compiled set of output-filtering rules.
	Guardrails = core.Guardrails
	// StreamGuardrailConfig configures incremental guardrail scanning.
	StreamGuardrailConfig = core.StreamGuardrailConfig
	// StreamGuardrailResult is the outcome of scanning one stream chunk.
	StreamGuardrailResult = core.StreamGuardrailResult
	// StreamGuardrails applies guardrail rules to a response stream chunk by chunk.
	StreamGuardrails = core.StreamGuardrails
)

// NewStreamGuardrails builds an incremental guardrail scanner over a rule set.
func NewStreamGuardrails(g *Guardrails, config StreamGuardrailConfig) *StreamGuardrails {
	return core.NewStreamGuardrails(g, config)
}

// Guardrail rule types, actions, and severities (see core).
const (
	GuardrailPII             = core.GuardrailPII
	GuardrailPromptInjection = core.GuardrailPromptInjection
	GuardrailHarmfulContent  = core.GuardrailHarmfulContent
	GuardrailSecretLeak      = core.GuardrailSecretLeak
	GuardrailCustom          = core.GuardrailCustom
	GuardrailBlock           = core.GuardrailBlock
	GuardrailRedact          = core.GuardrailRedact
	GuardrailWarn            = core.GuardrailWarn
	SeverityLow              = core.SeverityLow
	SeverityMedium           = core.SeverityMedium
	SeverityHigh             = core.SeverityHigh
	SeverityCritical         = core.SeverityCritical
)

// NewGuardrails compiles a guardrail rule set (panics on invalid patterns).
func NewGuardrails(rules ...GuardrailRule) *Guardrails { return core.NewGuardrails(rules...) }

// NewGuardrailsSafe compiles a guardrail rule set, returning pattern errors.
func NewGuardrailsSafe(rules ...GuardrailRule) (*Guardrails, error) {
	return core.NewGuardrailsSafe(rules...)
}

// ApplyRedactions scrubs matched content from a response.
func ApplyRedactions(response string, violations []GuardrailViolation) string {
	return core.ApplyRedactions(response, violations)
}

// DefaultPIIRules returns the built-in PII redaction rules.
func DefaultPIIRules() []GuardrailRule { return core.DefaultPIIRules() }

// DefaultSecretLeakRules returns the built-in secret-leak blocking rules.
func DefaultSecretLeakRules() []GuardrailRule { return core.DefaultSecretLeakRules() }

// DefaultPromptInjectionRules returns the built-in prompt-injection rules.
func DefaultPromptInjectionRules() []GuardrailRule { return core.DefaultPromptInjectionRules() }

// DefaultHarmfulContentRules returns the built-in harmful-content rules.
func DefaultHarmfulContentRules() []GuardrailRule { return core.DefaultHarmfulContentRules() }

// AllDefaultRules returns every built-in guardrail rule.
func AllDefaultRules() []GuardrailRule { return core.AllDefaultRules() }

// RulesForType returns the built-in rules for one guardrail type.
func RulesForType(t GuardrailType) []GuardrailRule { return core.RulesForType(t) }

// Response health classifications (see core.DetectResponseHealth).
const (
	ResponseOK                 = core.ResponseOK
	ResponseErrorOnlyReasoning = core.ResponseErrorOnlyReasoning
	ResponseEmpty              = core.ResponseEmpty
	ResponseMalformedStream    = core.ResponseMalformedStream
)

// streamChannelBuffer bridges the buffer-size constant that moved to core.
const streamChannelBuffer = core.StreamChannelBuffer

// DetectResponseHealth classifies a response from stream/response signals.
func DetectResponseHealth(sig ResponseSignals) ResponseHealth {
	return core.DetectResponseHealth(sig)
}

// ResponseHasContent reports whether a response carries content or tool calls.
func ResponseHasContent(resp *GraycodeRouterResponse) bool {
	return core.ResponseHasContent(resp)
}

// DefaultRepeatDetector returns a RepeatDetector with production thresholds.
func DefaultRepeatDetector() *RepeatDetector {
	return core.DefaultRepeatDetector()
}

// NewPooledHTTPClient returns an *http.Client sharing the pooled transport.
func NewPooledHTTPClient(timeout time.Duration) *http.Client {
	return core.NewPooledHTTPClient(timeout)
}

// CloseIdleConnections closes idle pooled connections across all provider clients.
func CloseIdleConnections() {
	core.CloseIdleConnections()
}

// ParseInlineToolCalls extracts inline/Hermes-style tool calls from text.
func ParseInlineToolCalls(text string) (string, []ToolCall) {
	return core.ParseInlineToolCalls(text)
}

// NewRetryConfig constructs a RetryConfig from core fields and optional
// HTTP status codes to retry on.
func NewRetryConfig(maxRetries int, baseDelay, maxDelay time.Duration, retryOn ...int) RetryConfig {
	return core.NewRetryConfig(maxRetries, baseDelay, maxDelay, retryOn...)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return core.DefaultRetryConfig()
}

// Embedding API moved to client/embeddings; aliased here for compatibility.
type (
	// Embedder is the interface for creating embeddings.
	Embedder = embeddings.Embedder
	// EmbeddingParams holds asymmetric params for indexing vs query.
	EmbeddingParams = embeddings.EmbeddingParams
	// EmbeddingRequest represents an embedding API call.
	EmbeddingRequest = embeddings.EmbeddingRequest
	// EmbeddingResponse holds embedding results.
	EmbeddingResponse = embeddings.EmbeddingResponse
	// SemanticCacheConfig configures the embedding-based semantic cache.
	SemanticCacheConfig = embeddings.SemanticCacheConfig
	// SemanticCacheStats reports semantic cache effectiveness.
	SemanticCacheStats = embeddings.SemanticCacheStats
	// EmbeddingCachedProvider caches chat responses keyed by embedding similarity.
	EmbeddingCachedProvider = embeddings.EmbeddingCachedProvider
)

// DefaultEmbeddingParams returns known-good asymmetric params for common embedding models.
func DefaultEmbeddingParams(model string) EmbeddingParams {
	return embeddings.DefaultEmbeddingParams(model)
}

// DefaultSemanticCacheConfig returns sensible semantic cache defaults.
func DefaultSemanticCacheConfig() SemanticCacheConfig {
	return embeddings.DefaultSemanticCacheConfig()
}

// NewEmbeddingCachedProvider wraps a provider with an embedding-similarity cache.
func NewEmbeddingCachedProvider(inner Provider, embedder Embedder, cfg SemanticCacheConfig) *EmbeddingCachedProvider {
	return embeddings.NewEmbeddingCachedProvider(inner, embedder, cfg)
}

var (
	copyResponse     = core.CopyResponse
	emit             = core.Emit
	parseImageString = core.ParseImageString
	applyGuardrails  = core.ApplyGuardrails
)

// NewStreamResult creates a StreamResult with a cancel function for resource
// cleanup. The request ID is optional; pass "" when it is not yet available.
// The canonical constructor lives in
// github.com/GrayCodeAI/graycode-router/llm; this is a thin facade
// wrapper that keeps the public client API stable.
func NewStreamResult(events <-chan GraycodeRouterStreamEvent, cancel context.CancelFunc) *StreamResult {
	return llm.NewStreamResult(events, "", cancel)
}

// NewStreamResultWithRequestID is NewStreamResult carrying the provider's request ID.
func NewStreamResultWithRequestID(events <-chan GraycodeRouterStreamEvent, requestID string, cancel context.CancelFunc) *StreamResult {
	return llm.NewStreamResult(events, requestID, cancel)
}

// DefaultContinuationConfig returns sensible defaults.
func DefaultContinuationConfig() ContinuationConfig {
	return core.DefaultContinuationConfig()
}

// ---------------------------------------------------------------------------
// Adapter type aliases (moved to client/adapters)
// ---------------------------------------------------------------------------

type (
	// AnthropicClient implements Provider for the Anthropic Messages API.
	AnthropicClient = adapters.AnthropicClient
	// OpenAIClient implements Provider for the OpenAI Chat Completions API.
	OpenAIClient = adapters.OpenAIClient
	// GeminiClient implements Provider for the Google Gemini API.
	GeminiClient = adapters.GeminiClient
	// GeminiOpenAIClient implements Provider for the OpenAI-compatible Gemini endpoint.
	GeminiOpenAIClient = adapters.GeminiOpenAIClient
	// AzureClient implements Provider for the Azure OpenAI API.
	AzureClient = adapters.AzureClient
	// BedrockClient implements Provider for the AWS Bedrock API.
	BedrockClient = adapters.BedrockClient
	// VertexClient implements Provider for the Google Vertex AI API.
	VertexClient = adapters.VertexClient
	// DeepSeekClient implements Provider for the DeepSeek API.
	DeepSeekClient = adapters.DeepSeekClient
	// ZAIClient implements Provider for the Z.AI API.
	ZAIClient = adapters.ZAIClient
	// MiMoClient implements Provider for the Xiaomi MiMo API.
	MiMoClient = adapters.MiMoClient
	// AgnesClient implements Provider for the Agnes AI API.
	AgnesClient = adapters.AgnesClient
	// StepFunClient implements Provider for the StepFun API.
	StepFunClient = adapters.StepFunClient
	// LongCatClient implements Provider for the LongCat API.
	LongCatClient = adapters.LongCatClient
	// GrokClient implements Provider for the xAI (Grok) API.
	GrokClient = adapters.GrokClient
	// OpenRouterClient implements Provider for the OpenRouter API.
	OpenRouterClient = adapters.OpenRouterClient
	// CanopyWaveClient implements Provider for the CanopyWave API.
	CanopyWaveClient = adapters.CanopyWaveClient
	// OpenGatewayClient implements Provider for the OpenGateway API.
	OpenGatewayClient = adapters.OpenGatewayClient
	// GroqClient implements Provider for the Groq API.
	GroqClient = adapters.GroqClient
	// ClinePassClient implements Provider for the ClinePass API.
	ClinePassClient = adapters.ClinePassClient
	// OllamaClient implements Provider for the Ollama API.
	OllamaClient = adapters.OllamaClient
	// KimiClient implements Provider for the Kimi (Moonshot) API.
	KimiClient = adapters.KimiClient
	// MiniMaxClient implements Provider for the MiniMax API.
	MiniMaxClient = adapters.MiniMaxClient
	// ConcentrateResponsesClient implements Provider for the Concentrate Responses API.
	ConcentrateResponsesClient = adapters.ConcentrateResponsesClient
	// OpenCodeGoClient implements Provider for the OpenCode Go API.
	OpenCodeGoClient = adapters.OpenCodeGoClient
	// PoolsideClient implements Poolside reasoning-only stream recovery.
	PoolsideClient = adapters.PoolsideClient
	// ProtocolRouter routes between OpenAI and Anthropic protocols.
	ProtocolRouter = adapters.ProtocolRouter
	// ProtocolStreamConfig controls streaming across two protocols.
	ProtocolStreamConfig = adapters.ProtocolStreamConfig
	// TokenCountResult holds token counting results.
	TokenCountResult = adapters.TokenCountResult
)

// Adapter protocol constants.
const (
	ChatProtocolCompletions = adapters.ChatProtocolCompletions
	ChatProtocolMessages    = adapters.ChatProtocolMessages
)

// Adapter constructors.
func NewAnthropicClient(apiKey, baseURL string, opts ...ClientOption) *AnthropicClient {
	return adapters.NewAnthropicClient(apiKey, baseURL, opts...)
}

func NewOpenAIClient(apiKey, baseURL string, compat *OpenAICompatConfig, opts ...ClientOption) *OpenAIClient {
	return adapters.NewOpenAIClient(apiKey, baseURL, compat, opts...)
}

func NewPoolsideClient(apiKey, baseURL string, opts ...ClientOption) *PoolsideClient {
	return adapters.NewPoolsideClient(apiKey, baseURL, opts...)
}

func NewGeminiClient(apiKey, baseURL string) *GeminiClient {
	return adapters.NewGeminiClient(apiKey, baseURL)
}

func NewGeminiOpenAIClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *GeminiOpenAIClient {
	return adapters.NewGeminiOpenAIClient(apiKey, openAIBase, compat, opts...)
}

func NewAzureClient(apiKey, endpoint, apiVersion string) *AzureClient {
	return adapters.NewAzureClient(apiKey, endpoint, apiVersion)
}

func NewBedrockClient(accessKeyID, secretAccessKey, sessionToken, region string) *BedrockClient {
	return adapters.NewBedrockClient(accessKeyID, secretAccessKey, sessionToken, region)
}

func NewVertexClient(projectID, region, token string) *VertexClient {
	return adapters.NewVertexClient(projectID, region, token)
}

func NewDeepSeekClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *DeepSeekClient {
	return adapters.NewDeepSeekClient(apiKey, openAIBase, compat, opts...)
}

func NewZAIClient(apiKey, openAIBase, anthropicBase string, compat *OpenAICompatConfig, providerID string, opts ...ClientOption) *ZAIClient {
	return adapters.NewZAIClient(apiKey, openAIBase, anthropicBase, compat, providerID, opts...)
}

func NewMiMoClient(apiKey, openAIBase string, compat *OpenAICompatConfig, providerID string, opts ...ClientOption) *MiMoClient {
	return adapters.NewMiMoClient(apiKey, openAIBase, compat, providerID, opts...)
}

func NewAgnesClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *AgnesClient {
	return adapters.NewAgnesClient(apiKey, openAIBase, compat, opts...)
}

func NewStepFunClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *StepFunClient {
	return adapters.NewStepFunClient(apiKey, openAIBase, compat, opts...)
}

func NewLongCatClient(apiKey, openAIBase, anthropicBase string, compat *OpenAICompatConfig, opts ...ClientOption) *LongCatClient {
	return adapters.NewLongCatClient(apiKey, openAIBase, anthropicBase, compat, opts...)
}

func NewGrokClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *GrokClient {
	return adapters.NewGrokClient(apiKey, openAIBase, compat, opts...)
}

func NewOpenRouterClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *OpenRouterClient {
	return adapters.NewOpenRouterClient(apiKey, openAIBase, compat, opts...)
}

func NewCanopyWaveClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *CanopyWaveClient {
	return adapters.NewCanopyWaveClient(apiKey, openAIBase, compat, opts...)
}

func NewOpenGatewayClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *OpenGatewayClient {
	return adapters.NewOpenGatewayClient(apiKey, openAIBase, compat, opts...)
}

func NewGroqClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *GroqClient {
	return adapters.NewGroqClient(apiKey, openAIBase, compat, opts...)
}

func NewClinePassClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *ClinePassClient {
	return adapters.NewClinePassClient(apiKey, openAIBase, compat, opts...)
}

func NewOllamaClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *OllamaClient {
	return adapters.NewOllamaClient(apiKey, openAIBase, compat, opts...)
}

func NewKimiClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *KimiClient {
	return adapters.NewKimiClient(apiKey, openAIBase, compat, opts...)
}

func NewMiniMaxClient(apiKey, openAIBase string, compat *OpenAICompatConfig, opts ...ClientOption) *MiniMaxClient {
	return adapters.NewMiniMaxClient(apiKey, openAIBase, compat, opts...)
}

func NewConcentrateResponsesClient(apiKey, baseURL string, opts ...ClientOption) *ConcentrateResponsesClient {
	return adapters.NewConcentrateResponsesClient(apiKey, baseURL, opts...)
}

func NewOpenCodeGoClient(apiKey, baseURL string, opts ...ClientOption) *OpenCodeGoClient {
	return adapters.NewOpenCodeGoClient(apiKey, baseURL, opts...)
}

func AnthropicBaseFromOpenAIV1(openAIBase string) string {
	return adapters.AnthropicBaseFromOpenAIV1(openAIBase)
}

// Provider registry constants.
const (
	ProviderTypeAnthropic        = adapters.ProviderTypeAnthropic
	ProviderTypeOpenAI           = adapters.ProviderTypeOpenAI
	ProviderTypeOpenAICompatible = adapters.ProviderTypeOpenAICompatible
	ProviderTypeAzure            = adapters.ProviderTypeAzure
	ProviderTypeBedrock          = adapters.ProviderTypeBedrock
	ProviderTypeVertex           = adapters.ProviderTypeVertex
)

// Package-local aliases keep existing in-package tests and helpers readable
// without expanding the public client facade.
type (
	anthropicRequest        = adapters.AnthropicRequest
	anthropicResponse       = adapters.AnthropicResponse
	anthropicTool           = adapters.AnthropicTool
	anthropicToolChoice     = adapters.AnthropicToolChoice
	anthropicThinking       = adapters.AnthropicThinking
	anthropicMetadata       = adapters.AnthropicMetadata
	anthropicOutputConfig   = adapters.AnthropicOutputConfig
	openaiEmbeddingData     = adapters.OpenAIEmbeddingData
	openaiEmbeddingResponse = adapters.OpenAIEmbeddingResponse
	openaiEmbeddingUsage    = adapters.OpenAIEmbeddingUsage
)

var (
	audioFormatToMediaType      = adapters.AudioFormatToMediaType
	resolveThinking             = adapters.ResolveThinking
	resolveToolChoice           = adapters.ResolveToolChoice
	resolveOutputConfig         = adapters.ResolveOutputConfig
	buildAnthropicMessages      = adapters.BuildAnthropicMessages
	buildAnthropicCachedRequest = adapters.BuildAnthropicCachedRequest
	parseAnthropicResponse      = adapters.ParseAnthropicResponse
	convertToAnthropicTools     = adapters.ConvertToAnthropicTools
	buildRequestBase            = adapters.BuildRequestBase
	openaiBaseFallbackURL       = adapters.OpenAIBaseFallbackURL
	dynamicProviderEnvVar       = adapters.DynamicProviderEnvVar
	geminiSharedParserEnvVar    = adapters.GeminiSharedParserEnvVar
	processGeminiStream         = adapters.ProcessGeminiStream
	oaCompatUnsupportedError    = adapters.OACompatUnsupportedError
	CoreProviders               = adapters.CoreProviders
	OpenAICompatibleProviders   = adapters.OpenAICompatibleProviders
	sha256Hex                   = adapters.Sha256Hex
	awsSigningKey               = adapters.AWSSigningKey
	canonicalAWSHeaders         = adapters.CanonicalAWSHeaders
	awsCanonicalURI             = adapters.AWSCanonicalURI
	dynamicProviderEnabled      = adapters.DynamicProviderEnabled
	thinkingForBudget           = adapters.ThinkingForBudget
)
