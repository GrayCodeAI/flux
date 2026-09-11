package engine

import "github.com/GrayCodeAI/eyrie/client"

// toClientMessages returns the messages unchanged: the engine and the client
// both speak the canonical contract message type, so no per-field conversion
// is needed.
func toClientMessages(in []Message) []client.EyrieMessage {
	return in
}

// toClientOptions maps a normalized generation request onto the client's
// wire-format chat options. Provider-specific translation continues to live in
// the adapters; this is the contract-level mapping.
func toClientOptions(req GenerateRequest, route Route, stream bool) client.ChatOptions {
	tools := append([]client.EyrieTool(nil), req.Tools...)
	opts := client.ChatOptions{
		Provider: route.Provider, Model: route.Model, Stream: stream,
		System: req.SystemPrompt, Tools: tools, Temperature: req.Temperature,
		MaxTokens: req.Limits.MaxOutputTokens, MetadataUserID: req.Metadata.UserID,
	}
	advanced := req.Options
	opts.EnableCaching = advanced.EnableCaching
	opts.ReasoningEffort = advanced.ReasoningEffort
	opts.ThinkingBudgetTokens = advanced.ThinkingBudgetTokens
	opts.ThinkingMode = advanced.ThinkingMode
	opts.ThinkingDisplay = advanced.ThinkingDisplay
	opts.GLMThinkingEnabled = advanced.GLMThinkingEnabled
	opts.VirtualKeyID = advanced.VirtualKeyID
	opts.KimiContextCacheID = advanced.KimiContextCacheID
	opts.KimiCacheResetTTL = advanced.KimiCacheResetTTL
	opts.TopP = advanced.TopP
	opts.TopK = advanced.TopK
	opts.StopSequences = append([]string(nil), advanced.StopSequences...)
	if advanced.ToolChoice != nil {
		opts.ToolChoice = &client.ToolChoiceOption{
			Type: advanced.ToolChoice.Type, Name: advanced.ToolChoice.Name,
			DisableParallelToolUse: advanced.ToolChoice.DisableParallelToolUse,
		}
	}
	opts.ServiceTier = advanced.ServiceTier
	opts.OutputEffort = advanced.OutputEffort
	opts.PresencePenalty = advanced.PresencePenalty
	opts.FrequencyPenalty = advanced.FrequencyPenalty
	opts.N = advanced.N
	opts.LogProbs = advanced.LogProbs
	opts.TopLogProbs = advanced.TopLogProbs
	opts.Seed = advanced.Seed
	opts.Store = advanced.Store
	opts.Metadata = cloneStringMap(advanced.Metadata)
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]string)
	}
	setMetadataIfPresent(opts.Metadata, "session.id", req.Metadata.SessionID)
	setMetadataIfPresent(opts.Metadata, "turn.id", req.Metadata.TurnID)
	setMetadataIfPresent(opts.Metadata, "project.id", req.Metadata.ProjectID)
	if len(opts.Metadata) == 0 {
		opts.Metadata = nil
	}
	opts.Modalities = append([]string(nil), advanced.Modalities...)
	opts.AudioConfig = advanced.AudioConfig
	opts.Prediction = advanced.Prediction
	opts.WebSearchOptions = advanced.WebSearchOptions
	if req.OutputSchema != "" {
		opts.ResponseFormat = &client.ResponseFormat{Type: "json_schema", Schema: req.OutputSchema}
		opts.OutputSchema = req.OutputSchema
	}
	return opts
}

func setMetadataIfPresent(metadata map[string]string, key, value string) {
	if value != "" {
		metadata[key] = value
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// fromClientResponse attaches the resolved route to a client response. The
// engine and the client both speak the canonical contract response type, so
// this only sets the route the engine selected.
func fromClientResponse(resp *client.EyrieResponse, route Route) *GenerateResponse {
	if resp == nil {
		return &GenerateResponse{Route: &route}
	}
	resp.Route = &route
	return resp
}

// fromClientUsage returns the usage unchanged: the engine and the client both
// speak the canonical contract usage type.
func fromClientUsage(usage *client.EyrieUsage) *Usage {
	return usage
}
