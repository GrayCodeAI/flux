// OpenTelemetry GenAI semantic convention attribute keys for AI agent spans.
//
// These exported constants are the canonical, ecosystem-wide attribute keys for
// describing LLM / AI agent operations. They follow the OpenTelemetry GenAI
// semantic conventions (gen_ai.*) and are shared as the reference set that the
// other hawk-eco repos (hawk, harrier, shrike, swift) should mirror when emitting
// spans, so dashboards and exporters can correlate cost/usage/identity across
// the whole ecosystem.
//
// The legacy llm.* attribute keys in observability.go remain for backwards
// compatibility; new instrumentation should prefer the gen_ai.* keys below.
// This file is stdlib-only and purely additive (constant declarations only).
//
// Spec: https://opentelemetry.io/docs/specs/semconv/gen-ai/
package graycoderouter

// GenAI semantic convention attribute keys. These mirror the OpenTelemetry
// gen_ai.* namespace plus the small set of ecosystem extensions
// (cost.usd, session.id, agent.id) documented in docs/OTEL-CONVENTIONS.md.
const (
	// AttrGenAISystem identifies the GenAI provider/system handling the request
	// (e.g. "openai", "anthropic", "gemini"). OTel: gen_ai.system.
	AttrGenAISystem = "gen_ai.system"

	// AttrGenAIRequestModel is the model name requested by the caller
	// (e.g. "gpt-4o", "claude-sonnet-4-5"). OTel: gen_ai.request.model.
	AttrGenAIRequestModel = "gen_ai.request.model"

	// AttrGenAIResponseModel is the model that actually served the response, when
	// the provider reports it. OTel: gen_ai.response.model.
	AttrGenAIResponseModel = "gen_ai.response.model"

	// AttrGenAIUsageInputTokens is the number of prompt/input tokens consumed.
	// OTel: gen_ai.usage.input_tokens.
	AttrGenAIUsageInputTokens = "gen_ai.usage.input_tokens" // #nosec G101 -- OTel semconv attribute key string, not a secret value

	// AttrGenAIUsageOutputTokens is the number of completion/output tokens
	// generated. OTel: gen_ai.usage.output_tokens.
	AttrGenAIUsageOutputTokens = "gen_ai.usage.output_tokens" // #nosec G101 -- OTel semconv attribute key string, not a secret value

	// AttrGenAIOperationName names the GenAI operation (e.g. "chat", "embeddings",
	// "tool_use"). OTel: gen_ai.operation.name.
	AttrGenAIOperationName = "gen_ai.operation.name"

	// AttrCostUSD is the computed monetary cost of the operation in US dollars.
	// Ecosystem extension to the OTel gen_ai.* set; key: cost.usd.
	AttrCostUSD = "cost.usd"

	// AttrToolName is the name of the tool/function invoked during an agent step.
	// OTel: gen_ai.tool.name (exposed here as the stable key "tool.name").
	AttrToolName = "tool.name"

	// AttrSessionID correlates spans belonging to the same logical session or
	// conversation. Ecosystem key: session.id.
	AttrSessionID = "session.id"

	// AttrAgentID identifies the agent instance producing the span. Ecosystem
	// key: agent.id (aligns with OTel gen_ai.agent.id).
	AttrAgentID = "agent.id"
)
