package registry

import "github.com/GrayCodeAI/eyrie/catalog/opencodego"

func init() {
	for _, spec := range providerSpecs() {
		DefaultRegistry.Register(spec)
	}
}

// All returns every registered provider spec (sorted by SortOrder at use sites).
func All() []ProviderSpec {
	return DefaultRegistry.All()
}

func providerSpecs() []ProviderSpec {
	return []ProviderSpec{
		// ── Direct API providers ──────────────────────────────────────────
		{
			ProviderID: "anthropic", DisplayName: "Anthropic", DeploymentID: "anthropic-direct", SortOrder: 1, ChatPreference: 2,
			TransportKind: "anthropic",
			RequiresKey:   true, CredentialEnv: "ANTHROPIC_API_KEY",
			CredentialAliases: []string{"CLAUDE_API_KEY"},
			BaseURLEnv:        []string{"ANTHROPIC_BASE_URL"},
			ProbeKind:         ProbeAnthropic,
			LiveFetcherKey:    "anthropic", LiveCatalogKey: "anthropic",
			ProtocolID: "anthropic-messages", AdapterID: "anthropic", RuntimeProfileKey: "anthropic",
			DirectFallbacks: []string{"openai"},
		},
		{
			ProviderID: "openai", DisplayName: "OpenAI", DeploymentID: "openai-direct", SortOrder: 2, ChatPreference: 1,
			TransportKind: "openai",
			RequiresKey:   true, CredentialEnv: "OPENAI_API_KEY",
			BaseURLEnv: []string{"OPENAI_BASE_URL", "OPENAI_API_BASE"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.openai.com/v1",
			LiveFetcherKey: "openai", LiveCatalogKey: "openai",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "openai",
			DirectFallbacks: []string{"anthropic"},
		},
		{
			ProviderID: "gemini", DisplayName: "Gemini API", DeploymentID: "gemini-direct", SortOrder: 3, ChatPreference: 5,
			RuntimeBaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
			RequiresKey:    true, CredentialEnv: "GEMINI_API_KEY",
			CredentialAliases: []string{"GOOGLE_API_KEY"},
			BaseURLEnv:        []string{"GEMINI_BASE_URL"},
			ProbeKind:         ProbeGemini,
			LiveFetcherKey:    "gemini", LiveCatalogKey: "gemini",
			ProtocolID: "gemini-generate-content", AdapterID: "gemini", RuntimeProfileKey: "gemini",
		},
		{
			ProviderID: "deepseek", DisplayName: "DeepSeek", DeploymentID: "deepseek-direct", SortOrder: 4, ChatPreference: 11,
			RequiresKey: true, CredentialEnv: "DEEPSEEK_API_KEY",
			BaseURLEnv: []string{"DEEPSEEK_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.deepseek.com/v1",
			LiveFetcherKey: "deepseek", LiveCatalogKey: "deepseek",
			ProtocolID: "openai-chat-completions", AdapterID: "deepseek", RuntimeProfileKey: "deepseek",
		},
		{
			ProviderID: "grok", DisplayName: "xAI", DeploymentID: "grok-direct", SortOrder: 5, ChatPreference: 4,
			RequiresKey: true, CredentialEnv: "XAI_API_KEY",
			BaseURLEnv: []string{"XAI_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.x.ai/v1",
			LiveFetcherKey: "grok", LiveCatalogKey: "grok",
			ProtocolID: "openai-chat-completions", AdapterID: "grok", RuntimeProfileKey: "grok",
		},
		{
			ProviderID: "kimi", DisplayName: "Kimi", DeploymentID: "kimi-direct", SortOrder: 6, ChatPreference: 14,
			RequiresKey: true, CredentialEnv: "MOONSHOT_API_KEY",
			BaseURLEnv: []string{"MOONSHOT_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.moonshot.ai/v1",
			LiveFetcherKey: "kimi", LiveCatalogKey: "kimi",
			ProtocolID: "openai-chat-completions", AdapterID: "kimi", RuntimeProfileKey: "kimi",
		},
		{
			ProviderID: "zai_coding", DisplayName: "Z.AI — Coding Plan", DeploymentID: "zai_coding-direct", SortOrder: 7, ChatPreference: 8,
			RequiresKey: true, CredentialEnv: "ZAI_CODING_API_KEY",
			BaseURLEnv: []string{"ZAI_CODING_BASE_URL", "ZAI_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.z.ai/api/coding/paas/v4",
			LiveFetcherKey: "zai_coding", LiveCatalogKey: "zai_coding",
			ProtocolID: "openai-chat-completions", AdapterID: "zai_coding", RuntimeProfileKey: "zai_coding",
			PrepareCredentialEnv: true,
		},
		{
			ProviderID: "zai_payg", DisplayName: "Z.AI — Pay-as-you-go", DeploymentID: "zai_payg-direct", SortOrder: 8, ChatPreference: 9,
			RequiresKey: true, CredentialEnv: "ZAI_API_KEY",
			BaseURLEnv: []string{"ZAI_BASE_URL", "ZAI_API_BASE"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.z.ai/api/paas/v4",
			LiveFetcherKey: "zai_payg", LiveCatalogKey: "zai_payg",
			ProtocolID: "openai-chat-completions", AdapterID: "zai_payg", RuntimeProfileKey: "zai_payg",
			PrepareCredentialEnv: true,
		},
		{
			ProviderID: "xiaomi_mimo_token_plan", DisplayName: "Xiaomi MiMo — Token Plan", DeploymentID: "xiaomi_mimo_token_plan-direct", SortOrder: 9, ChatPreference: 16,
			RequiresKey: true, CredentialEnv: "XIAOMI_MIMO_TOKEN_PLAN_API_KEY",
			BaseURLEnv: []string{"XIAOMI_MIMO_TOKEN_PLAN_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "",
			LiveFetcherKey: "xiaomi_mimo_token_plan", LiveCatalogKey: "xiaomi_mimo_token_plan",
			ProtocolID: "openai-chat-completions", AdapterID: "xiaomi_mimo", RuntimeProfileKey: "xiaomi_mimo_token_plan",
			PrepareCredentialEnv: true,
		},
		{
			ProviderID: "xiaomi_mimo_payg", DisplayName: "Xiaomi MiMo — Pay-as-you-go", DeploymentID: "xiaomi_mimo_payg-direct", SortOrder: 10, ChatPreference: 15,
			RequiresKey: true, CredentialEnv: "XIAOMI_MIMO_PAYG_API_KEY",
			CredentialAliases: []string{"XIAOMI_MIMO_API_KEY"},
			BaseURLEnv:        []string{"XIAOMI_MIMO_PAYG_BASE_URL", "XIAOMI_BASE_URL"},
			ProbeKind:         ProbeOpenAIModels, ProbeBaseURL: "https://api.xiaomimimo.com/v1",
			LiveFetcherKey: "xiaomi_mimo_payg", LiveCatalogKey: "xiaomi_mimo_payg",
			ProtocolID: "openai-chat-completions", AdapterID: "xiaomi_mimo", RuntimeProfileKey: "xiaomi_mimo_payg",
		},
		{
			ProviderID: "minimax_token_plan", DisplayName: "MiniMax — Token Plan", DeploymentID: "minimax_token_plan-direct", SortOrder: 11, ChatPreference: 17,
			RequiresKey: true, CredentialEnv: "MINIMAX_TOKEN_PLAN_API_KEY",
			BaseURLEnv: []string{"MINIMAX_TOKEN_PLAN_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.minimax.io/v1",
			LiveFetcherKey: "minimax_token_plan", LiveCatalogKey: "minimax_token_plan",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "minimax_token_plan",
		},
		{
			ProviderID: "minimax_payg", DisplayName: "MiniMax — Pay-as-you-go", DeploymentID: "minimax_payg-direct", SortOrder: 12, ChatPreference: 18,
			RequiresKey: true, CredentialEnv: "MINIMAX_PAYG_API_KEY",
			BaseURLEnv: []string{"MINIMAX_PAYG_BASE_URL", "MINIMAX_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.minimax.io/v1",
			LiveFetcherKey: "minimax_payg", LiveCatalogKey: "minimax_payg",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "minimax_payg",
		},

		// ── Cloud platform providers ──────────────────────────────────────
		{
			ProviderID: "azure", DisplayName: "Azure OpenAI", DeploymentID: "openai-azure", SortOrder: 13, ChatPreference: 12,
			TransportKind: "azure",
			RequiresKey:   true, CredentialEnv: "AZURE_OPENAI_API_KEY",
			BaseURLEnv:     []string{"AZURE_OPENAI_ENDPOINT"},
			ProbeKind:      ProbeNone,
			LiveFetcherKey: "azure", LiveCatalogKey: "azure",
			ProtocolID: "openai-chat-completions", AdapterID: "openai-azure", RuntimeProfileKey: "azure",
		},
		{
			ProviderID: "bedrock", DisplayName: "Amazon Bedrock", DeploymentID: "anthropic-bedrock", SortOrder: 14, ChatPreference: 7,
			TransportKind: "bedrock",
			RequiresKey:   true, CredentialEnv: "AWS_SECRET_ACCESS_KEY",
			CredentialEnvFallbacks: []string{"AWS_ACCESS_KEY_ID", "AWS_SESSION_TOKEN"},
			BaseURLEnv:             []string{"AWS_REGION", "AWS_DEFAULT_REGION"},
			ProbeKind:              ProbeNone,
			LiveFetcherKey:         "bedrock", LiveCatalogKey: "bedrock",
			ProtocolID: "anthropic-messages", AdapterID: "anthropic-bedrock", RuntimeProfileKey: "bedrock",
		},
		{
			ProviderID: "vertex", DisplayName: "Vertex AI", DeploymentID: "gemini-vertex", SortOrder: 15, ChatPreference: 6,
			TransportKind: "vertex",
			RequiresKey:   true, CredentialEnv: "VERTEX_ACCESS_TOKEN",
			CredentialEnvFallbacks: []string{"GOOGLE_OAUTH_ACCESS_TOKEN"},
			BaseURLEnv:             []string{"VERTEX_PROJECT_ID", "VERTEX_REGION"},
			ProbeKind:              ProbeNone,
			LiveFetcherKey:         "vertex", LiveCatalogKey: "vertex",
			ProtocolID: "gemini-generate-content", AdapterID: "gemini-vertex", RuntimeProfileKey: "vertex",
		},

		// ── Aggregators ───────────────────────────────────────────────────
		{
			ProviderID: "openrouter", DisplayName: "OpenRouter", DeploymentID: "openrouter", SortOrder: 16, ChatPreference: 3,
			RequiresKey: true, CredentialEnv: "OPENROUTER_API_KEY",
			BaseURLEnv: []string{"OPENROUTER_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://openrouter.ai/api/v1",
			LiveFetcherKey: "openrouter", LiveCatalogKey: "openrouter",
			ProtocolID: "openai-chat-completions", AdapterID: "openrouter", RuntimeProfileKey: "openrouter",
		},
		{
			ProviderID: "concentrate", DisplayName: "Concentrate AI (Pay-as-you-go)", DeploymentID: "concentrate-payg", SortOrder: 7, ChatPreference: 4,
			RequiresKey: true, CredentialEnv: "CONCENTRATE_API_KEY",
			BaseURLEnv: []string{"CONCENTRATE_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.concentrate.ai/v1",
			LiveFetcherKey: "concentrate", LiveCatalogKey: "concentrate",
			PublicModelCatalog: true,
			ProtocolID:         "openai-responses", AdapterID: "concentrate-responses", RuntimeProfileKey: "concentrate",
		},
		{
			ProviderID: "stepfun", DisplayName: "StepFun", DeploymentID: "stepfun-direct", SortOrder: 2, ChatPreference: 27,
			RequiresKey: true, CredentialEnv: "STEP_API_KEY",
			BaseURLEnv: []string{"STEP_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.stepfun.ai/v1",
			LiveFetcherKey: "stepfun", LiveCatalogKey: "stepfun",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "stepfun",
		},
		{
			ProviderID: "agnes", DisplayName: "Agnes AI", DeploymentID: "agnes-direct", SortOrder: 1, ChatPreference: 25,
			RequiresKey: true, CredentialEnv: "AGNES_API_KEY",
			BaseURLEnv: []string{"AGNES_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://apihub.agnes-ai.com/v1",
			LiveFetcherKey: "agnes", LiveCatalogKey: "agnes",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "agnes",
		},
		{
			ProviderID: "longcat", DisplayName: "LongCat", DeploymentID: "longcat-direct", SortOrder: 12, ChatPreference: 26,
			RequiresKey: true, CredentialEnv: "LONGCAT_API_KEY",
			BaseURLEnv: []string{"LONGCAT_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.longcat.chat/openai/v1",
			LiveFetcherKey: "longcat", LiveCatalogKey: "longcat",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "longcat",
		},

		// ── Niche ─────────────────────────────────────────────────────────
		{
			ProviderID: "canopywave", DisplayName: "CanopyWave", DeploymentID: "canopywave", SortOrder: 17, ChatPreference: 10,
			RequiresKey: true, CredentialEnv: "CANOPYWAVE_API_KEY",
			BaseURLEnv: []string{"CANOPYWAVE_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://inference.canopywave.io/v1",
			LiveFetcherKey: "canopywave", LiveCatalogKey: "canopywave",
			ProtocolID: "openai-chat-completions", AdapterID: "canopywave", RuntimeProfileKey: "canopywave",
		},
		{
			ProviderID: "poolside", DisplayName: "Poolside", DeploymentID: "poolside", SortOrder: 18, ChatPreference: 20,
			RequiresKey: true, CredentialEnv: "POOLSIDE_API_KEY",
			BaseURLEnv: []string{"POOLSIDE_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://inference.poolside.ai/v1",
			LiveFetcherKey: "poolside", LiveCatalogKey: "poolside",
			ProtocolID: "openai-chat-completions", AdapterID: "poolside", RuntimeProfileKey: "poolside",
		},
		{
			ProviderID: "groq", DisplayName: "Groq", DeploymentID: "groq-direct", SortOrder: 19, ChatPreference: 21,
			RequiresKey: true, CredentialEnv: "GROQ_API_KEY",
			BaseURLEnv: []string{"GROQ_BASE_URL"},
			ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.groq.com/openai/v1",
			LiveFetcherKey: "groq", LiveCatalogKey: "groq",
			ProtocolID: "openai-chat-completions", AdapterID: "groq", RuntimeProfileKey: "groq",
		},
		{
			ProviderID: "clinepass", DisplayName: "ClinePass", DeploymentID: "clinepass", SortOrder: 20, ChatPreference: 22,
			RuntimeBaseURL: "https://api.cline.bot/api/v1",
			RequiresKey:    true, CredentialEnv: "CLINE_API_KEY",
			BaseURLEnv:     []string{"CLINE_API_BASE"},
			ProbeKind:      ProbeNone,
			LiveFetcherKey: "clinepass", LiveCatalogKey: "clinepass",
			ProtocolID: "openai-chat-completions", AdapterID: "clinepass", RuntimeProfileKey: "clinepass",
		},
		{
			ProviderID: "opencodego", DisplayName: "OpenCode Go", DeploymentID: "opencodego", SortOrder: 21, ChatPreference: 13,
			RequiresKey: true, CredentialEnv: "OPENCODEGO_API_KEY",
			BaseURLEnv:     []string{"OPENCODEGO_BASE_URL"},
			ProbeKind:      ProbeOpenAIModels,
			ProbeBaseURL:   opencodego.DefaultBaseURL,
			LiveFetcherKey: "opencodego", LiveCatalogKey: "opencodego",
			ProtocolID: "openai-chat-completions", AdapterID: "opencodego", RuntimeProfileKey: "opencodego",
		},

		// ── Local ─────────────────────────────────────────────────────────
		{
			ProviderID: "ollama", DisplayName: "Ollama", DeploymentID: "ollama-local", SortOrder: 22, ChatPreference: 19,
			RuntimeBaseURL: "http://localhost:11434/v1", RuntimeCredentialEnv: "OLLAMA_API_KEY",
			RequiresKey: false, CredentialEnv: "OLLAMA_BASE_URL",
			BaseURLEnv:     []string{"OLLAMA_BASE_URL"},
			ProbeKind:      ProbeOllama,
			LiveFetcherKey: "ollama", LiveCatalogKey: "ollama",
			ProtocolID: "openai-chat-completions", AdapterID: "openai", RuntimeProfileKey: "ollama",
			IsLocal: true,
			RetryConfig: &RetryConfig{
				BaseDelayMs: 2000, MaxDelayMs: 10000, MaxRetries: 3,
				BackoffMultiplier: 1.5, JitterPct: 10,
				RetryOnCodes: []int{500, 503},
				AbortOnCodes: []int{400},
			},
		},
	}
}
