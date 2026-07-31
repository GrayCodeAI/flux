package client

import "github.com/GrayCodeAI/eyrie/client/adapters"

// OpenAICompatConfig holds provider-specific compatibility flags.
type OpenAICompatConfig = adapters.OpenAICompatConfig

// Per-provider compat configs.
var (
	OpenAICompat     = adapters.OpenAICompat
	GrokCompat       = adapters.GrokCompat
	OpenRouterCompat = adapters.OpenRouterCompat
	GeminiCompat     = adapters.GeminiCompat
	ZAICompat        = adapters.ZAICompat
	CanopyWaveCompat = adapters.CanopyWaveCompat
	OllamaCompat     = adapters.OllamaCompat
	OpenCodeGoCompat = adapters.OpenCodeGoCompat
	PoolsideCompat   = adapters.PoolsideCompat
	GroqCompat       = adapters.GroqCompat
	ClinePassCompat  = adapters.ClinePassCompat
	KimiCompat       = adapters.KimiCompat
	XiaomiCompat     = adapters.XiaomiCompat
	AzureCompat      = adapters.AzureCompat
	BedrockCompat    = adapters.BedrockCompat
	VertexCompat     = adapters.VertexCompat
	DeepSeekCompat   = adapters.DeepSeekCompat
)
