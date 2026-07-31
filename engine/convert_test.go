package engine

import (
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
	llm "github.com/GrayCodeAI/hawk-core-contracts/llm"
)

func TestToClientMessages_ReturnsMessagesUnchanged(t *testing.T) {
	in := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	out := toClientMessages(in)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Role != "user" || out[0].Content != "hello" {
		t.Errorf("first message mismatch: %+v", out[0])
	}
}

func TestToClientOptions_MapsBasicFields(t *testing.T) {
	temp := 0.7
	req := llm.GenerateRequest{
		SystemPrompt: "be helpful",
		Temperature:  &temp,
		OutputSchema: "{}",
		Limits:       llm.Limits{MaxOutputTokens: 1000},
		Metadata:     llm.Metadata{SessionID: "sess-1", TurnID: "turn-2", UserID: "user-3", ProjectID: "proj-4"},
		Tools:        []llm.EyrieTool{{Name: "read", Description: "read a file"}},
	}
	route := Route{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}

	opts := toClientOptions(req, route, true)

	if opts.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", opts.Provider, "anthropic")
	}
	if opts.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", opts.Model, "claude-sonnet-4-20250514")
	}
	if !opts.Stream {
		t.Error("Stream = false, want true")
	}
	if opts.System != "be helpful" {
		t.Errorf("System = %q, want %q", opts.System, "be helpful")
	}
	if opts.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %d, want 1000", opts.MaxTokens)
	}
	if opts.Temperature == nil || *opts.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", opts.Temperature)
	}
	if opts.OutputSchema != "{}" {
		t.Errorf("OutputSchema = %q, want %q", opts.OutputSchema, "{}")
	}
	if len(opts.Tools) != 1 || opts.Tools[0].Name != "read" {
		t.Errorf("Tools = %+v, want one read tool", opts.Tools)
	}
}

func TestToClientOptions_MapsMetadata(t *testing.T) {
	req := llm.GenerateRequest{
		Metadata: llm.Metadata{SessionID: "sess-1", TurnID: "turn-2", UserID: "user-3", ProjectID: "proj-4"},
	}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if opts.Metadata["session.id"] != "sess-1" {
		t.Errorf("session.id = %q, want %q", opts.Metadata["session.id"], "sess-1")
	}
	if opts.Metadata["turn.id"] != "turn-2" {
		t.Errorf("turn.id = %q, want %q", opts.Metadata["turn.id"], "turn-2")
	}
	if opts.Metadata["project.id"] != "proj-4" {
		t.Errorf("project.id = %q, want %q", opts.Metadata["project.id"], "proj-4")
	}
	// UserID is intentionally not mapped to metadata in toClientOptions.
}

func TestToClientOptions_EmptyMetadataOmitted(t *testing.T) {
	req := llm.GenerateRequest{} // no metadata
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if opts.Metadata != nil {
		t.Errorf("Metadata = %+v, want nil when all fields empty", opts.Metadata)
	}
}

func TestToClientOptions_MapsAdvancedOptions(t *testing.T) {
	topP := 0.9
	topK := 40
	req := llm.GenerateRequest{
		Options: llm.GenerationOptions{
			EnableCaching:        true,
			ReasoningEffort:      "high",
			ThinkingBudgetTokens: 2048,
			ThinkingMode:         "enabled",
			TopP:                 &topP,
			TopK:                 &topK,
			StopSequences:        []string{"\n\n", "END"},
			ServiceTier:          "auto",
			OutputEffort:         "medium",
			Modalities:           []string{"text", "audio"},
			AudioConfig:          "pcm16",
			Prediction:           "cached",
			WebSearchOptions:     "auto",
			VirtualKeyID:         "vk-123",
			KimiContextCacheID:   "cache-456",
			KimiCacheResetTTL:    true,
		},
	}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if !opts.EnableCaching {
		t.Error("EnableCaching = false, want true")
	}
	if opts.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q, want %q", opts.ReasoningEffort, "high")
	}
	if opts.ThinkingBudgetTokens != 2048 {
		t.Errorf("ThinkingBudgetTokens = %d, want 2048", opts.ThinkingBudgetTokens)
	}
	if opts.ThinkingMode != "enabled" {
		t.Errorf("ThinkingMode = %q, want %q", opts.ThinkingMode, "enabled")
	}
	if opts.TopP == nil || *opts.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", opts.TopP)
	}
	if opts.TopK == nil || *opts.TopK != 40 {
		t.Errorf("TopK = %v, want 40", opts.TopK)
	}
	if len(opts.StopSequences) != 2 || opts.StopSequences[0] != "\n\n" {
		t.Errorf("StopSequences = %v, want [\\n\\n END]", opts.StopSequences)
	}
	if opts.ServiceTier != "auto" {
		t.Errorf("ServiceTier = %q, want %q", opts.ServiceTier, "auto")
	}
	if opts.OutputEffort != "medium" {
		t.Errorf("OutputEffort = %q, want %q", opts.OutputEffort, "medium")
	}
	if len(opts.Modalities) != 2 {
		t.Errorf("Modalities = %v, want [text audio]", opts.Modalities)
	}
	if opts.AudioConfig != "pcm16" {
		t.Errorf("AudioConfig = %q, want %q", opts.AudioConfig, "pcm16")
	}
	if opts.Prediction != "cached" {
		t.Errorf("Prediction = %q, want %q", opts.Prediction, "cached")
	}
	if opts.WebSearchOptions != "auto" {
		t.Errorf("WebSearchOptions = %q, want %q", opts.WebSearchOptions, "auto")
	}
	if opts.VirtualKeyID != "vk-123" {
		t.Errorf("VirtualKeyID = %q, want %q", opts.VirtualKeyID, "vk-123")
	}
	if opts.KimiContextCacheID != "cache-456" {
		t.Errorf("KimiContextCacheID = %q, want %q", opts.KimiContextCacheID, "cache-456")
	}
	if !opts.KimiCacheResetTTL {
		t.Error("KimiCacheResetTTL = false, want true")
	}
}

func TestToClientOptions_MapsToolChoice(t *testing.T) {
	req := llm.GenerateRequest{
		Options: llm.GenerationOptions{
			ToolChoice: &llm.ToolChoiceOption{
				Type:                   "tool",
				Name:                   "read",
				DisableParallelToolUse: true,
			},
		},
	}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if opts.ToolChoice == nil {
		t.Fatal("ToolChoice = nil, want non-nil")
	}
	if opts.ToolChoice.Type != "tool" {
		t.Errorf("ToolChoice.Type = %q, want %q", opts.ToolChoice.Type, "tool")
	}
	if opts.ToolChoice.Name != "read" {
		t.Errorf("ToolChoice.Name = %q, want %q", opts.ToolChoice.Name, "read")
	}
	if !opts.ToolChoice.DisableParallelToolUse {
		t.Error("DisableParallelToolUse = false, want true")
	}
}

func TestToClientOptions_OutputSchemaCreatesResponseFormat(t *testing.T) {
	req := llm.GenerateRequest{OutputSchema: `{"type":"object"}`}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if opts.ResponseFormat == nil {
		t.Fatal("ResponseFormat = nil, want non-nil")
	}
	if opts.ResponseFormat.Type != "json_schema" {
		t.Errorf("ResponseFormat.Type = %q, want %q", opts.ResponseFormat.Type, "json_schema")
	}
	if opts.ResponseFormat.Schema != `{"type":"object"}` {
		t.Errorf("ResponseFormat.Schema = %q, want %q", opts.ResponseFormat.Schema, `{"type":"object"}`)
	}
}

func TestToClientOptions_NoOutputSchemaLeavesResponseFormatNil(t *testing.T) {
	req := llm.GenerateRequest{}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	if opts.ResponseFormat != nil {
		t.Errorf("ResponseFormat = %+v, want nil", opts.ResponseFormat)
	}
}

func TestToClientOptions_ClonesSlicesAndMaps(t *testing.T) {
	// Verify that mutating the request after conversion does not affect the options.
	req := llm.GenerateRequest{
		Tools:          []llm.EyrieTool{{Name: "a"}, {Name: "b"}},
		Options:        llm.GenerationOptions{StopSequences: []string{"x", "y"}},
		OutputSchema:   "orig",
	}
	route := Route{Provider: "test", Model: "test/model"}
	opts := toClientOptions(req, route, false)

	// Mutate the original request.
	req.Tools[0].Name = "mutated"
	req.Options.StopSequences[0] = "mutated"
	req.OutputSchema = "mutated"

	if opts.Tools[0].Name != "a" {
		t.Errorf("Tools not cloned: got %q, want %q", opts.Tools[0].Name, "a")
	}
	if opts.StopSequences[0] != "x" {
		t.Errorf("StopSequences not cloned: got %q, want %q", opts.StopSequences[0], "x")
	}
	if opts.OutputSchema != "orig" {
		t.Errorf("OutputSchema = %q, want %q", opts.OutputSchema, "orig")
	}
}

func TestFromClientResponse_AttachesRoute(t *testing.T) {
	resp := &client.EyrieResponse{Content: "hello"}
	route := Route{Provider: "anthropic", Model: "claude-sonnet-4-20250514"}
	out := fromClientResponse(resp, route)

	if out == nil {
		t.Fatal("fromClientResponse returned nil")
	}
	if out.Route == nil {
		t.Fatal("Route = nil, want non-nil")
	}
	if out.Route.Provider != "anthropic" {
		t.Errorf("Route.Provider = %q, want %q", out.Route.Provider, "anthropic")
	}
	if out.Content != "hello" {
		t.Errorf("Content = %q, want %q", out.Content, "hello")
	}
}

func TestFromClientResponse_NilResponse(t *testing.T) {
	route := Route{Provider: "test", Model: "test/model"}
	out := fromClientResponse(nil, route)

	if out == nil {
		t.Fatal("fromClientResponse(nil) returned nil")
	}
	if out.Route == nil || out.Route.Provider != "test" {
		t.Errorf("Route = %+v, want provider test", out.Route)
	}
}

func TestFromClientUsage_ReturnsUnchanged(t *testing.T) {
	usage := &client.EyrieUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	out := fromClientUsage(usage)
	if out != usage {
		t.Error("fromClientUsage should return the same pointer")
	}
}

func TestSetMetadataIfPresent_SetsOnlyNonEmpty(t *testing.T) {
	metadata := map[string]string{}
	setMetadataIfPresent(metadata, "key1", "value1")
	setMetadataIfPresent(metadata, "key2", "")
	setMetadataIfPresent(metadata, "key3", "value3")

	if metadata["key1"] != "value1" {
		t.Errorf("key1 = %q, want %q", metadata["key1"], "value1")
	}
	if _, ok := metadata["key2"]; ok {
		t.Error("key2 should not be set for empty value")
	}
	if metadata["key3"] != "value3" {
		t.Errorf("key3 = %q, want %q", metadata["key3"], "value3")
	}
}

func TestCloneStringMap_CopiesAndIsolates(t *testing.T) {
	original := map[string]string{"a": "1", "b": "2"}
	cloned := cloneStringMap(original)

	if len(cloned) != 2 || cloned["a"] != "1" || cloned["b"] != "2" {
		t.Fatalf("clone = %+v, want %+v", cloned, original)
	}
	// Mutating the clone must not affect the original.
	cloned["a"] = "mutated"
	if original["a"] != "1" {
		t.Errorf("original mutated: got %q, want %q", original["a"], "1")
	}
}

func TestCloneStringMap_NilReturnsNil(t *testing.T) {
	if cloneStringMap(nil) != nil {
		t.Error("cloneStringMap(nil) should return nil")
	}
}
