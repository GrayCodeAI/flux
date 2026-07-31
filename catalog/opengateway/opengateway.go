// Package opengateway holds shared constants for the OpenGateway inference gateway
// (https://gitlawb.com/opengateway), an OpenAI-compatible endpoint that routes
// requests across providers (MiMo, Gemini, MiniMax, Qwen, Kimi, GLM, etc.) and
// returns the live model catalog with inline pricing from GET /v1/models.
package opengateway

// DefaultBaseURL is the OpenGateway API root.
const DefaultBaseURL = "https://opengateway.gitlawb.com/v1"
