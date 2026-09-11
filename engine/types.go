package engine

import (
	"github.com/GrayCodeAI/eyrie/llm"
)

// Intent expresses a host's semantic preference without naming a provider.
type Intent = llm.Intent

// ModelClass is a provider-neutral relative model cost/capability band.
type ModelClass = llm.ModelClass

// Requirements describes capabilities that a resolved model must support.
type Requirements = llm.Requirements

// Preference contains optional selection policy supplied by the host/user.
type Preference = llm.Preference

// ContentPart is a provider-neutral multimodal message part.
type ContentPart = llm.ContentPart

// ToolCall is a normalized tool invocation.
type ToolCall = llm.ToolCall

// ToolResult is a host-executed tool result sent back to a model.
type ToolResult = llm.ToolResult

// Message is the stable conversation DTO at the host boundary.
type Message = llm.EyrieMessage

// Tool is a model-visible tool definition. Eyrie emits requests for these
// tools; the host remains responsible for permission checks and execution.
type Tool = llm.EyrieTool

// ToolChoice controls whether and how the model may request tools.
type ToolChoice = llm.ToolChoiceOption

// GenerationOptions contains model-generation controls that have equivalent
// semantics across one or more provider adapters. Provider-specific wire
// formats remain internal to Eyrie.
type GenerationOptions = llm.GenerationOptions

// Limits bounds a generation request.
type Limits = llm.Limits

// Metadata provides stable correlation identifiers. Providers receive only
// fields that their adapter explicitly supports.
type Metadata = llm.Metadata

// GenerateRequest is the provider-neutral request accepted by Engine.
type GenerateRequest = llm.GenerateRequest

// Route is the concrete model/deployment decision made by Eyrie.
type Route = llm.ResolvedRoute

// Usage is normalized token accounting.
type Usage = llm.EyrieUsage

// GenerateResponse is a normalized blocking response.
type GenerateResponse = llm.EyrieResponse

// Event type names. These are the stable stream vocabulary shared by the
// engine and its hosts; new types are additive and hosts must safely ignore
// unknown ones.
const (
	EventRouteSelected = "route_selected"
	EventRouteChanged  = "route_changed"
	EventContentDelta  = "content_delta"
	EventThinkingDelta = "thinking_delta"
	EventToolCallStart = "tool_call_start"
	EventToolCallDelta = "tool_call_delta"
	EventToolCallDone  = "tool_call_done"
	EventUsage         = "usage"
	EventRetry         = "retry"
	EventContinuation  = "continuation"
	EventWarning       = "warning"
	EventTTFT          = "ttft"
	EventDone          = "done"
)

// Event is a normalized model stream event. New optional fields and event
// types are additive; hosts must safely ignore unknown event types.
type Event = llm.EyrieStreamEvent

// Model is a host-facing catalog row.
type Model = llm.Model

// CatalogSnapshot is an immutable host-facing view of a loaded catalog. It is a
// type alias to the canonical llm.CatalogSnapshot so exactly one struct crosses
// the host boundary (matching every other type in this file).
type CatalogSnapshot = llm.CatalogSnapshot

// CredentialStatus is safe to render or log; it never contains a secret.
type CredentialStatus = llm.CredentialStatus

// CredentialProvider is safe setup metadata for a configurable provider.
// It contains identifiers and labels only, never credential material.
type CredentialProvider = llm.CredentialProviderOption

// CredentialResolution validates pasted input and returns provider choices.
// The input secret is never retained in this value.
type CredentialResolution = llm.CredentialResolution

// Gateway is one host-facing provider/deployment configuration row.
type Gateway = llm.Gateway

// StatePaths reports the Engine's host-owned state locations without reading
// or parsing either file.
type StatePaths = llm.StatePaths

// Selection is the effective provider/model state supplied to a host session.
type Selection = llm.Selection

// SelectionOptions contains optional user or command-line overrides.
type SelectionOptions = llm.SelectionOptions

// CatalogHealth is the host-facing catalog health report.
type CatalogHealth = llm.CatalogHealth

// DeploymentSummary is the host-facing deployment routing summary.
type DeploymentSummary = llm.DeploymentSummary

// PreflightOptions configures a readiness check.
type PreflightOptions = llm.PreflightOptions

// PreflightReport is the host-facing preflight result.
type PreflightReport = llm.PreflightReport

// PreflightCheck is one readiness check row.
type PreflightCheck = llm.PreflightCheck

// CheckStatus is a preflight check status.
type CheckStatus = llm.CheckStatus

// ProviderStateSecurity reports provider.json security posture (no secrets).
type ProviderStateSecurity = llm.ProviderStateSecurity

// NativeCompactionRequest is a provider-native compaction request.
type NativeCompactionRequest = llm.NativeCompactionRequest

// EventStreamer is the pull-based host stream contract.
type EventStreamer = llm.EventStreamer

// Provider is the full host port (composition of role interfaces in llm).
type Provider = llm.Provider

// Re-export preflight status constants from the contract package.
const (
	CheckOK   = llm.CheckOK
	CheckFail = llm.CheckFail
	CheckWarn = llm.CheckWarn
)
