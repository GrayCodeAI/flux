package tools

import "fmt"

// BehaviorPreset selects a back-compat behavior contract for tool execution.
// FinalizeToolConfigRequest.behavior_version carries one of these presets so a
// single locked configuration can keep older clients working without the server
// having to special-case version strings.
type BehaviorPreset string

const (
	// BehaviorPresetUnspecified is the zero value; a finalize call SHOULD reject
	// it and require an explicit choice.
	BehaviorPresetUnspecified BehaviorPreset = ""
	// BehaviorPresetCurrent is the actively-supported behavior contract.
	BehaviorPresetCurrent BehaviorPreset = "current"
	// BehaviorPresetLegacy selects the prior behavior contract for back-compat
	// with clients that haven't adopted BehaviorPresetCurrent.
	BehaviorPresetLegacy BehaviorPreset = "legacy"
)

// BehaviorPresetFrom parses a behavior_version string into a BehaviorPreset.
// Returns an error for any unrecognized value — an empty or unknown preset is
// invalid, not silently defaulted (matches the closed-enum forward-safety rule
// applied to ToolNamespace).
func BehaviorPresetFrom(s string) (BehaviorPreset, error) {
	switch BehaviorPreset(s) {
	case BehaviorPresetCurrent, BehaviorPresetLegacy, BehaviorPresetUnspecified:
		return BehaviorPreset(s), nil
	default:
		return BehaviorPresetUnspecified, fmt.Errorf("tools: unknown behavior preset %q", s)
	}
}

// FinalizeErrorCode classifies a finalize warning or violation, mirroring the
// FINALIZE_ERROR_CODE enum in proto/hawk/contracts/v1/tool.proto.
type FinalizeErrorCode int

const (
	FinalizeErrorCodeUnspecified         FinalizeErrorCode = iota // unspecified
	FinalizeErrorCodeUnknownTool                                  // unknown tool id
	FinalizeErrorCodeIncompatibleVersion                          // behavior_version incompatible with enabled_tools
	FinalizeErrorCodeDeprecatedReplaced                           // tool deprecated/replaced
	FinalizeErrorCodeInvalidParams                                // invalid tool parameters
)

var finalizeErrorCodeNames = [...]string{
	"unspecified",
	"unknown_tool",
	"incompatible_version",
	"deprecated_replaced",
	"invalid_params",
}

func (c FinalizeErrorCode) String() string {
	if int(c) >= 0 && int(c) < len(finalizeErrorCodeNames) {
		return finalizeErrorCodeNames[c]
	}
	return "unknown"
}

// VersionWarning is a non-fatal, behavior-version drift reported by finalize.
type VersionWarning struct {
	Code          FinalizeErrorCode `json:"code"`
	Message       string            `json:"message"`
	AffectedTools []string          `json:"affected_tools,omitempty"`
}

// FinalizeConfigViolation is a fatal, deterministic problem that prevents
// finalize from succeeding.
type FinalizeConfigViolation struct {
	Code    FinalizeErrorCode `json:"code"`
	ToolID  string            `json:"tool_id"`
	Message string            `json:"message"`
}

// FinalizeResult is the decoded outcome of a FinalizeToolConfig call.
type FinalizeResult struct {
	BehaviorVersion string                    `json:"behavior_version"`
	Warnings        []VersionWarning          `json:"warnings,omitempty"`
	Violations      []FinalizeConfigViolation `json:"violations,omitempty"`
	Finalized       bool                      `json:"finalized"`
}

// Ok reports whether finalize succeeded and the tool set is safe to call:
// Finalized must be true AND there must be no fatal violations. Warnings alone
// do not make Ok return false.
func (r FinalizeResult) Ok() bool {
	return r.Finalized && len(r.Violations) == 0
}

// ToolMeta is the canonical identity envelope attached to tool-call events,
// mirroring the ToolMeta message in proto/hawk/contracts/v1/tool.proto.
// version is an additive-only bump: new additive fields do NOT change it.
type ToolMeta struct {
	Version   string        `json:"version"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Namespace ToolNamespace `json:"namespace"`
	Label     string        `json:"label,omitempty"`
	ReadOnly  bool          `json:"read_only"`
}

// ToolNamespace is a CLOSED enum identifying the harness that owns a tool,
// mirroring the ToolNamespace enum in proto/hawk/contracts/v1/tool.proto. A new
// unknown namespace is a wire-breaking change that intentionally fails
// ToolNamespaceFrom (forward-safety): a deploy that rolls the contract forward
// before the consumer code cannot silently mis-route a tool it doesn't
// understand. ToolNamespaceAcp is reserved for the forthcoming hawk-acp repo.
type ToolNamespace string

const (
	ToolNamespaceUnspecified      ToolNamespace = ""           // unspecified
	ToolNamespaceHawkBuild        ToolNamespace = "hawk_build" // hawk
	ToolNamespaceHawkBuildConcise ToolNamespace = "hawk_build_concise"
	ToolNamespaceCodex            ToolNamespace = "codex"    // codex harness
	ToolNamespaceOpencode         ToolNamespace = "opencode" // opencode harness
	ToolNamespaceMcp              ToolNamespace = "mcp"      // MCP servers (falcon)
	ToolNamespaceAcp              ToolNamespace = "acp"      // reserved: hawk-acp
)

// ToolNamespaceFrom parses a namespace string into a ToolNamespace. The set is
// closed: an unrecognized value returns an error rather than silently mapping
// to ToolNamespaceUnspecified, so a new namespace on the wire fails strict
// typed deserialization (see the closed-enum forward-safety rule above).
func ToolNamespaceFrom(s string) (ToolNamespace, error) {
	switch ToolNamespace(s) {
	case ToolNamespaceUnspecified, ToolNamespaceHawkBuild,
		ToolNamespaceHawkBuildConcise, ToolNamespaceCodex,
		ToolNamespaceOpencode, ToolNamespaceMcp, ToolNamespaceAcp:
		return ToolNamespace(s), nil
	default:
		return ToolNamespaceUnspecified, fmt.Errorf("tools: unknown tool namespace %q", s)
	}
}

// String returns the namespace string as-is, mapping the unspecified zero
// value to "unspecified" so log output and JSON stay unambiguous.
func (n ToolNamespace) String() string {
	if n == ToolNamespaceUnspecified {
		return "unspecified"
	}
	return string(n)
}
