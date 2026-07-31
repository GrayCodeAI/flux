package registry

// ProbeKind identifies HTTP credential validation.
type ProbeKind string

const (
	ProbeAnthropic    ProbeKind = "probe_anthropic"
	ProbeOpenAIModels ProbeKind = "probe_openai_models"
	ProbeGemini       ProbeKind = "probe_gemini"
	ProbeOllama       ProbeKind = "probe_ollama"
	ProbeNone         ProbeKind = "probe_none"
)

type RetryConfig struct {
	BaseDelayMs       int     `json:"base_delay_ms,omitempty"`
	MaxDelayMs        int     `json:"max_delay_ms,omitempty"`
	MaxRetries        int     `json:"max_retries,omitempty"`
	BackoffMultiplier float64 `json:"backoff_multiplier,omitempty"`
	JitterPct         int     `json:"jitter_pct,omitempty"`
	RetryOnCodes      []int   `json:"retry_on_codes,omitempty"`
	AbortOnCodes      []int   `json:"abort_on_codes,omitempty"`
}

// ProviderSpec is the single source of truth for setup providers.
// Every registered provider discovers models via its live list API only (no remote bootstrap).
type ProviderSpec struct {
	ProviderID             string
	DisplayName            string
	DeploymentID           string
	SortOrder              int
	ChatPreference         int
	RequiresKey            bool
	CredentialEnv          string
	CredentialEnvFallbacks []string // additional env var names for the same credential
	CredentialAliases      []string // compatibility env var names accepted by host UIs
	BaseURLEnv             []string
	ProbeKind              ProbeKind
	ProbeBaseURL           string
	LiveFetcherKey         string // key in catalog/live registry
	LiveCatalogKey         string // legacy provider key in ModelCatalog.Providers map
	PublicModelCatalog     bool   // live model listing is available without provider credentials
	ProtocolID             string
	AdapterID              string
	RuntimeProfileKey      string
	DirectFallbacks        []string
	PrepareCredentialEnv   bool
	RetryConfig            *RetryConfig
	IsLocal                bool
	// TransportKind selects the runtime client implementation. Empty means
	// OpenAI-compatible; dedicated transports declare their kind explicitly.
	TransportKind string
	// RuntimeBaseURL overrides ProbeBaseURL when chat and model-list endpoints
	// differ. RuntimeCredentialEnv overrides CredentialEnv when setup metadata
	// describes a non-secret value, as with Ollama's base URL.
	RuntimeBaseURL       string
	RuntimeCredentialEnv string
}

// EnvFallback describes one deployment env_fallback row.
type EnvFallback struct {
	Field string
	Env   []string
}
