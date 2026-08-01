package config

import "github.com/GrayCodeAI/eyrie/catalog/opencodego"

// APIProvider is the type for supported LLM providers.
type APIProvider = string

const (
	ProviderAnthropic           APIProvider = "anthropic"
	ProviderOpenAI              APIProvider = "openai"
	ProviderAzure               APIProvider = "azure"
	ProviderCanopyWave          APIProvider = "canopywave"
	ProviderDeepSeek            APIProvider = "deepseek"
	ProviderZAICoding           APIProvider = "zai_coding"
	ProviderZAIPayg             APIProvider = "zai_payg"
	ProviderOpenRouter          APIProvider = "openrouter"
	ProviderConcentrate         APIProvider = "concentrate"
	ProviderOpenGateway         APIProvider = "opengateway"
	ProviderAgnes               APIProvider = "agnes"
	ProviderGrok                APIProvider = "grok"
	ProviderGemini              APIProvider = "gemini"
	ProviderBedrock             APIProvider = "bedrock"
	ProviderVertex              APIProvider = "vertex"
	ProviderOllama              APIProvider = "ollama"
	ProviderOpenCodeGo          APIProvider = "opencodego"
	ProviderKimi                APIProvider = "kimi"
	ProviderXiaomiMimoPayg      APIProvider = "xiaomi_mimo_payg"
	ProviderXiaomiMimoTokenPlan APIProvider = "xiaomi_mimo_token_plan" // #nosec G101 -- provider id string, not a secret value
	ProviderMiniMaxTokenPlan    APIProvider = "minimax_token_plan"
	ProviderMiniMaxPayg         APIProvider = "minimax_payg"
	ProviderPoolside            APIProvider = "poolside"
	ProviderGroq                APIProvider = "groq"
	ProviderClinePass           APIProvider = "clinepass"
	ProviderStepFun             APIProvider = "stepfun"
)

// RuntimeProviderProfile defines how a provider is detected and configured at runtime.
type RuntimeProviderProfile struct {
	Mode           string      `json:"mode"`
	DefaultBaseURL string      `json:"default_base_url"`
	DefaultModel   string      `json:"default_model"`
	DetectionEnv   []string    `json:"detection_env"`
	ModelEnv       []string    `json:"model_env"`
	BaseURLEnv     []string    `json:"base_url_env"`
	APIKeys        []APIKeyDef `json:"api_keys"`
}

// APIKeyDef maps an env var to a key source name.
type APIKeyDef struct {
	Env    string `json:"env"`
	Source string `json:"source"`
}

// Provider runtime profiles.
var (
	AnthropicRuntimeProfile = RuntimeProviderProfile{
		Mode: "anthropic", DefaultBaseURL: DefaultAnthropicOpenAIBaseURL, DefaultModel: "claude-3-5-sonnet-latest",
		DetectionEnv: []string{"ANTHROPIC_API_KEY"},
		ModelEnv:     []string{"ANTHROPIC_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"ANTHROPIC_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "ANTHROPIC_API_KEY", Source: "anthropic"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	OpenAIRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultOpenAIBaseURL, DefaultModel: "gpt-4o",
		DetectionEnv: []string{"OPENAI_API_KEY"},
		ModelEnv:     []string{"OPENAI_MODEL"},
		BaseURLEnv:   []string{"OPENAI_BASE_URL", "OPENAI_API_BASE"},
		APIKeys:      []APIKeyDef{{Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	GrokRuntimeProfile = RuntimeProviderProfile{
		Mode: "grok", DefaultBaseURL: DefaultGrokOpenAIBaseURL, DefaultModel: "grok-2",
		DetectionEnv: []string{"XAI_API_KEY"},
		ModelEnv:     []string{"XAI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"XAI_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "XAI_API_KEY", Source: "grok"}},
	}
	GeminiRuntimeProfile = RuntimeProviderProfile{
		Mode: "gemini", DefaultBaseURL: DefaultGeminiOpenAIBaseURL, DefaultModel: "gemini-2.0-flash",
		DetectionEnv: []string{"GEMINI_API_KEY"},
		ModelEnv:     []string{"GEMINI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"GEMINI_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "GEMINI_API_KEY", Source: "gemini"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	VertexRuntimeProfile = RuntimeProviderProfile{
		Mode: "gemini-vertex", DefaultBaseURL: "", DefaultModel: "gemini-2.0-flash",
		DetectionEnv: []string{"VERTEX_ACCESS_TOKEN", "GOOGLE_OAUTH_ACCESS_TOKEN"},
		ModelEnv:     []string{"VERTEX_MODEL", "GEMINI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"VERTEX_PROJECT_ID", "VERTEX_REGION"},
		APIKeys:      []APIKeyDef{{Env: "VERTEX_ACCESS_TOKEN", Source: "vertex"}, {Env: "GOOGLE_OAUTH_ACCESS_TOKEN", Source: "google"}},
	}
	AzureRuntimeProfile = RuntimeProviderProfile{
		Mode: "azure", DefaultBaseURL: "", DefaultModel: "",
		DetectionEnv: []string{"AZURE_OPENAI_API_KEY"},
		ModelEnv:     []string{"AZURE_OPENAI_DEPLOYMENT", "AZURE_OPENAI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"AZURE_OPENAI_ENDPOINT"},
		APIKeys:      []APIKeyDef{{Env: "AZURE_OPENAI_API_KEY", Source: "azure"}},
	}
	BedrockRuntimeProfile = RuntimeProviderProfile{
		Mode: "bedrock", DefaultBaseURL: "", DefaultModel: "",
		DetectionEnv: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"},
		ModelEnv:     []string{"BEDROCK_MODEL", "ANTHROPIC_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"AWS_REGION", "AWS_DEFAULT_REGION"},
		APIKeys:      []APIKeyDef{{Env: "AWS_SECRET_ACCESS_KEY", Source: "bedrock"}, {Env: "AWS_ACCESS_KEY_ID", Source: "bedrock"}},
	}
	OpenRouterRuntimeProfile = RuntimeProviderProfile{
		Mode: "openrouter", DefaultBaseURL: DefaultOpenRouterOpenAIBaseURL,
		DetectionEnv: []string{"OPENROUTER_API_KEY"},
		ModelEnv:     []string{"OPENROUTER_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"OPENROUTER_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "OPENROUTER_API_KEY", Source: "openrouter"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	ConcentrateRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultConcentrateOpenAIBaseURL,
		DetectionEnv: []string{"CONCENTRATE_API_KEY"},
		ModelEnv:     []string{"CONCENTRATE_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"CONCENTRATE_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "CONCENTRATE_API_KEY", Source: "concentrate"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	OpenGatewayRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultOpenGatewayOpenAIBaseURL,
		DetectionEnv: []string{"OPENGATEWAY_API_KEY"},
		ModelEnv:     []string{"OPENGATEWAY_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"OPENGATEWAY_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "OPENGATEWAY_API_KEY", Source: "opengateway"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	ZAIPaygRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultZAIOpenAIBaseURL,
		DetectionEnv: []string{"ZAI_API_KEY"},
		ModelEnv:     []string{"ZAI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"ZAI_BASE_URL", "ZAI_API_BASE"},
		APIKeys:      []APIKeyDef{{Env: "ZAI_API_KEY", Source: "zai_payg"}},
	}
	ZAICodingRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultZAICodingOpenAIBaseURL,
		DetectionEnv: []string{"ZAI_CODING_API_KEY"},
		ModelEnv:     []string{"ZAI_CODING_MODEL", "ZAI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"ZAI_CODING_BASE_URL", "ZAI_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "ZAI_CODING_API_KEY", Source: "zai_coding"}},
	}
	CanopyWaveRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultCanopyWaveOpenAIBaseURL,
		DetectionEnv: []string{"CANOPYWAVE_API_KEY"},
		ModelEnv:     []string{"CANOPYWAVE_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"CANOPYWAVE_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "CANOPYWAVE_API_KEY", Source: "canopywave"}, {Env: "OPENAI_API_KEY", Source: "openai"}},
	}
	DeepSeekRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: "https://api.deepseek.com",
		DetectionEnv: []string{"DEEPSEEK_API_KEY"},
		ModelEnv:     []string{"DEEPSEEK_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"DEEPSEEK_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "DEEPSEEK_API_KEY", Source: "deepseek"}},
	}
	OpenCodeGoRuntimeProfile = RuntimeProviderProfile{
		Mode: "opencodego", DefaultBaseURL: DefaultOpenCodeGoBaseURL,
		DetectionEnv: []string{"OPENCODEGO_API_KEY"},
		ModelEnv:     []string{"OPENCODEGO_MODEL"},
		BaseURLEnv:   []string{"OPENCODEGO_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "OPENCODEGO_API_KEY", Source: "opencodego"}},
	}
	KimiRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultKimiOpenAIBaseURL,
		DetectionEnv: []string{"MOONSHOT_API_KEY"},
		ModelEnv:     []string{"MOONSHOT_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"MOONSHOT_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "MOONSHOT_API_KEY", Source: "kimi"}},
	}
	XiaomiPaygRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultXiaomiOpenAIBaseURL,
		DetectionEnv: []string{EnvXiaomiPaygAPIKey},
		ModelEnv:     []string{"XIAOMI_MIMO_PAYG_MODEL", "XIAOMI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{EnvXiaomiPaygBaseURL, "XIAOMI_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: EnvXiaomiPaygAPIKey, Source: "xiaomi_mimo_payg"}},
	}
	XiaomiTokenPlanRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: "",
		DetectionEnv: []string{EnvXiaomiTokenPlanAPIKey},
		ModelEnv:     []string{"XIAOMI_MIMO_TOKEN_PLAN_MODEL", "XIAOMI_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{EnvXiaomiTokenPlanBaseURL},
		APIKeys:      []APIKeyDef{{Env: EnvXiaomiTokenPlanAPIKey, Source: "xiaomi_mimo_token_plan"}},
	}
	MiniMaxTokenPlanRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultMiniMaxOpenAIBaseURL,
		DetectionEnv: []string{"MINIMAX_TOKEN_PLAN_API_KEY"},
		ModelEnv:     []string{"MINIMAX_TOKEN_PLAN_MODEL", "MINIMAX_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"MINIMAX_TOKEN_PLAN_BASE_URL", "MINIMAX_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "MINIMAX_TOKEN_PLAN_API_KEY", Source: "minimax_token_plan"}},
	}
	MiniMaxPaygRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultMiniMaxOpenAIBaseURL,
		DetectionEnv: []string{"MINIMAX_PAYG_API_KEY"},
		ModelEnv:     []string{"MINIMAX_PAYG_MODEL", "MINIMAX_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"MINIMAX_PAYG_BASE_URL", "MINIMAX_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "MINIMAX_PAYG_API_KEY", Source: "minimax_payg"}},
	}
	OllamaRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: OllamaDefaultBaseURL,
		DetectionEnv: []string{"OLLAMA_BASE_URL"},
		ModelEnv:     []string{"OLLAMA_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"OLLAMA_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "OLLAMA_API_KEY", Source: "ollama"}},
	}
	PoolsideRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultPoolsideOpenAIBaseURL,
		DetectionEnv: []string{"POOLSIDE_API_KEY"},
		ModelEnv:     []string{"POOLSIDE_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"POOLSIDE_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "POOLSIDE_API_KEY", Source: "poolside"}},
	}
	GroqRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: "https://api.groq.com/openai/v1",
		DetectionEnv: []string{"GROQ_API_KEY"},
		ModelEnv:     []string{"GROQ_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"GROQ_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "GROQ_API_KEY", Source: "groq"}},
	}
	ClinePassRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultClinePassOpenAIBaseURL,
		DetectionEnv: []string{"CLINE_API_KEY"},
		ModelEnv:     []string{"CLINE_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"CLINE_API_BASE"},
		APIKeys:      []APIKeyDef{{Env: "CLINE_API_KEY", Source: "clinepass"}},
	}
	StepFunRuntimeProfile = RuntimeProviderProfile{
		Mode: "openai", DefaultBaseURL: DefaultStepFunOpenAIBaseURL,
		DetectionEnv: []string{"STEP_API_KEY"},
		ModelEnv:     []string{"STEP_MODEL", "OPENAI_MODEL"},
		BaseURLEnv:   []string{"STEP_BASE_URL"},
		APIKeys:      []APIKeyDef{{Env: "STEP_API_KEY", Source: "stepfun"}},
	}
)

// APIProviderDetectionOrder is the priority order for provider detection.
var APIProviderDetectionOrder = []APIProvider{
	ProviderAnthropic, ProviderConcentrate, ProviderOpenRouter, ProviderGrok, ProviderGemini,
	ProviderVertex, ProviderBedrock, ProviderZAICoding, ProviderZAIPayg, ProviderCanopyWave, ProviderDeepSeek, ProviderPoolside, ProviderGroq, ProviderClinePass, ProviderAzure, ProviderOpenAI, ProviderOpenCodeGo,
	ProviderKimi, ProviderXiaomiMimoPayg, ProviderXiaomiMimoTokenPlan, ProviderMiniMaxTokenPlan, ProviderMiniMaxPayg, ProviderOllama, ProviderStepFun, ProviderOpenGateway,
}

// ProviderModelEnvKeys maps each provider to its model env var keys.
var ProviderModelEnvKeys = map[APIProvider][]string{
	ProviderAnthropic:           AnthropicRuntimeProfile.ModelEnv,
	ProviderOpenAI:              OpenAIRuntimeProfile.ModelEnv,
	ProviderAzure:               AzureRuntimeProfile.ModelEnv,
	ProviderCanopyWave:          CanopyWaveRuntimeProfile.ModelEnv,
	ProviderDeepSeek:            DeepSeekRuntimeProfile.ModelEnv,
	ProviderPoolside:            PoolsideRuntimeProfile.ModelEnv,
	ProviderGroq:                GroqRuntimeProfile.ModelEnv,
	ProviderClinePass:           ClinePassRuntimeProfile.ModelEnv,
	ProviderZAIPayg:             ZAIPaygRuntimeProfile.ModelEnv,
	ProviderZAICoding:           ZAICodingRuntimeProfile.ModelEnv,
	ProviderOpenRouter:          OpenRouterRuntimeProfile.ModelEnv,
	ProviderConcentrate:         ConcentrateRuntimeProfile.ModelEnv,
	ProviderGrok:                GrokRuntimeProfile.ModelEnv,
	ProviderGemini:              GeminiRuntimeProfile.ModelEnv,
	ProviderBedrock:             BedrockRuntimeProfile.ModelEnv,
	ProviderVertex:              VertexRuntimeProfile.ModelEnv,
	ProviderOllama:              {"OLLAMA_MODEL", "OPENAI_MODEL"},
	ProviderOpenCodeGo:          OpenCodeGoRuntimeProfile.ModelEnv,
	ProviderKimi:                KimiRuntimeProfile.ModelEnv,
	ProviderXiaomiMimoPayg:      XiaomiPaygRuntimeProfile.ModelEnv,
	ProviderXiaomiMimoTokenPlan: XiaomiTokenPlanRuntimeProfile.ModelEnv,
	ProviderMiniMaxTokenPlan:    {"MINIMAX_TOKEN_PLAN_MODEL", "MINIMAX_MODEL", "OPENAI_MODEL"},
	ProviderMiniMaxPayg:         {"MINIMAX_PAYG_MODEL", "MINIMAX_MODEL", "OPENAI_MODEL"},
	ProviderStepFun:             StepFunRuntimeProfile.ModelEnv,
	ProviderOpenGateway:         OpenGatewayRuntimeProfile.ModelEnv,
}

const (
	OllamaDefaultBaseURL     = "http://localhost:11434/v1"
	OpenCodeGoDefaultBaseURL = opencodego.DefaultBaseURL
)

// OpenAICompatibleRuntimeProfileOrder is the detection order for runtime profiles.
var OpenAICompatibleRuntimeProfileOrder = []string{
	"concentrate", "agnes", "longcat", "openrouter", "grok", "gemini", "anthropic", "zai_coding", "zai_payg", "canopywave", "deepseek", "poolside", "groq", "clinepass", "openai", "opencodego", "kimi", "xiaomi_mimo_payg", "xiaomi_mimo_token_plan", "minimax_token_plan", "minimax_payg", "stepfun", "opengateway",
}

// OpenAICompatibleRuntimeProfiles maps profile key to its runtime profile.
var OpenAICompatibleRuntimeProfiles = map[string]RuntimeProviderProfile{
	"concentrate":            ConcentrateRuntimeProfile,
	"opengateway":            OpenGatewayRuntimeProfile,
	"agnes":                  OpenAIRuntimeProfile,
	"longcat":                OpenAIRuntimeProfile,
	"anthropic":              AnthropicRuntimeProfile,
	"grok":                   GrokRuntimeProfile,
	"gemini":                 GeminiRuntimeProfile,
	"zai_payg":               ZAIPaygRuntimeProfile,
	"zai_coding":             ZAICodingRuntimeProfile,
	"canopywave":             CanopyWaveRuntimeProfile,
	"deepseek":               DeepSeekRuntimeProfile,
	"poolside":               PoolsideRuntimeProfile,
	"groq":                   GroqRuntimeProfile,
	"clinepass":              ClinePassRuntimeProfile,
	"openai":                 OpenAIRuntimeProfile,
	"openrouter":             OpenRouterRuntimeProfile,
	"opencodego":             OpenCodeGoRuntimeProfile,
	"kimi":                   KimiRuntimeProfile,
	"xiaomi_mimo_payg":       XiaomiPaygRuntimeProfile,
	"xiaomi_mimo_token_plan": XiaomiTokenPlanRuntimeProfile,
	"minimax_token_plan":     MiniMaxTokenPlanRuntimeProfile,
	"minimax_payg":           MiniMaxPaygRuntimeProfile,
	"stepfun":                StepFunRuntimeProfile,
}

// RuntimeProviderProfiles maps provider/profile keys to runtime detection profiles.
var RuntimeProviderProfiles = map[string]RuntimeProviderProfile{
	"anthropic":              AnthropicRuntimeProfile,
	"openai":                 OpenAIRuntimeProfile,
	"agnes":                  OpenAIRuntimeProfile,
	"grok":                   GrokRuntimeProfile,
	"gemini":                 GeminiRuntimeProfile,
	"vertex":                 VertexRuntimeProfile,
	"azure":                  AzureRuntimeProfile,
	"bedrock":                BedrockRuntimeProfile,
	"openrouter":             OpenRouterRuntimeProfile,
	"concentrate":            ConcentrateRuntimeProfile,
	"opengateway":            OpenGatewayRuntimeProfile,
	"zai_payg":               ZAIPaygRuntimeProfile,
	"zai_coding":             ZAICodingRuntimeProfile,
	"canopywave":             CanopyWaveRuntimeProfile,
	"deepseek":               DeepSeekRuntimeProfile,
	"poolside":               PoolsideRuntimeProfile,
	"groq":                   GroqRuntimeProfile,
	"clinepass":              ClinePassRuntimeProfile,
	"opencodego":             OpenCodeGoRuntimeProfile,
	"kimi":                   KimiRuntimeProfile,
	"xiaomi_mimo_payg":       XiaomiPaygRuntimeProfile,
	"xiaomi_mimo_token_plan": XiaomiTokenPlanRuntimeProfile,
	"minimax_token_plan":     MiniMaxTokenPlanRuntimeProfile,
	"minimax_payg":           MiniMaxPaygRuntimeProfile,
	"stepfun":                StepFunRuntimeProfile,
	"ollama":                 OllamaRuntimeProfile,
}

// RuntimeProfileByKey returns the provider runtime profile registered for key.
func RuntimeProfileByKey(key string) (RuntimeProviderProfile, bool) {
	profile, ok := RuntimeProviderProfiles[key]
	return profile, ok
}
