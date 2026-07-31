package adapters

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/types"
)

func TestNewOpenAIClient(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("sk-test", "https://custom.proxy.com/v1", &OpenAICompat)
	if c == nil {
		t.Fatal("NewOpenAIClient returned nil")
	}
	if c.apiKey != "sk-test" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != "https://custom.proxy.com/v1" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.providerName != "openai" {
		t.Errorf("providerName = %q", c.providerName)
	}
}

func TestNewOpenAIClient_EmptyBaseURL(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("sk-test", "", nil)
	if c.baseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.compat == nil {
		t.Error("expected default compat config")
	}
}

func TestOpenAIClient_Name(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	if c.Name() != "openai" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestOpenAIClient_Chat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-123",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "Hello from OpenAI!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 10, "total_tokens": 15},
		}), nil
	})
	c := NewOpenAIClient("sk-test", "https://api.openai.com/v1", nil)
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from OpenAI!" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finishReason = %q", resp.FinishReason)
	}
}

func TestOpenAIClient_Chat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestOpenAIClient_Chat_APIError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{"message": "rate limited"},
		}), nil
	})
	c := NewOpenAIClient("sk-test", "https://api.openai.com/v1", nil)
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	_, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAIClient_StreamChat_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"index\":0}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" stream\"},\"index\":0}]}\n\ndata: [DONE]\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	c := NewOpenAIClient("sk-test", "https://api.openai.com/v1", nil)
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.httpClient = &http.Client{Transport: transport}
	result, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer result.Close()
	var content string
	for evt := range result.Events {
		if evt.Type == "content" {
			content += evt.Content
		}
	}
	if content != "Hello stream" {
		t.Errorf("content = %q", content)
	}
}

func TestOpenAIClient_StreamChat_EmptyModel(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	_, err := c.StreamChat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: ""})
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestOpenAIClient_Ping_Success(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"data": []map[string]any{}}), nil
	})
	c := NewOpenAIClient("sk-test", "https://api.openai.com/v1", nil)
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOpenAIClient_Ping_AuthError(t *testing.T) {
	t.Parallel()
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
	})
	c := NewOpenAIClient("bad-key", "https://api.openai.com/v1", nil)
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestOpenAIClient_SetHTTPClientAndRetry(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("key", "", nil)
	c2 := NewOpenAIClient("key2", "", nil)
	c.SetHTTPClient(c2.httpClient)
	if c.httpClient != c2.httpClient {
		t.Error("SetHTTPClient did not replace client")
	}
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 3}})
	if c.retry.MaxRetries != 3 {
		t.Errorf("MaxRetries=%d", c.retry.MaxRetries)
	}
}

func TestBuildRequestBase(t *testing.T) {
	t.Parallel()
	temp := 0.7
	topP := 0.9
	req := BuildRequestBase([]core.EyrieMessage{
		{Role: "user", Content: "hello"},
	}, core.ChatOptions{
		Model: "gpt-4o", MaxTokens: 256, Temperature: &temp,
		TopP: &topP, StopSequences: []string{"\n"},
	}, false, &OpenAICompat)
	if req.Model != "gpt-4o" {
		t.Errorf("Model = %q", req.Model)
	}
	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("Temperature = %v", req.Temperature)
	}
	if req.MaxCompletionTokens == nil || *req.MaxCompletionTokens != 256 {
		t.Errorf("MaxCompletionTokens = %v", req.MaxCompletionTokens)
	}
}

func TestBuildRequestBase_MaxCompletionTokens(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompatConfig{MaxTokensField: "max_completion_tokens"}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "o1", MaxTokens: 512}, false, compat)
	if req.MaxCompletionTokens == nil || *req.MaxCompletionTokens != 512 {
		t.Errorf("MaxCompletionTokens = %v", req.MaxCompletionTokens)
	}
	if req.MaxTokens != nil {
		t.Errorf("MaxTokens should be nil")
	}
}

func TestBuildRequestBase_OmitMaxTokens(t *testing.T) {
	t.Parallel()
	// Agnes AI pre-authorizes the maximum token cost; sending the default 4096
	// can exceed the account balance and trigger insufficient_user_quota. With
	// OmitMaxTokens set, neither max_tokens nor max_completion_tokens is sent.
	compat := &OpenAICompatConfig{MaxTokensField: "max_tokens", OmitMaxTokens: true}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "agnes-2.5-pro-alpha", MaxTokens: 0}, false, compat)
	if req.MaxTokens != nil {
		t.Errorf("MaxTokens should be nil when OmitMaxTokens is set, got %v", *req.MaxTokens)
	}
	if req.MaxCompletionTokens != nil {
		t.Errorf("MaxCompletionTokens should be nil when OmitMaxTokens is set, got %v", *req.MaxCompletionTokens)
	}
}

func TestBuildRequestBase_StreamOptions(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256}, true, nil)
	if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Error("expected StreamOptions with IncludeUsage for streaming")
	}
}

func TestBuildRequestBase_NoStreamOptionsForIncompatible(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompatConfig{SupportsUsageInStreaming: false}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{Model: "deepseek", MaxTokens: 256}, true, compat)
	if req.StreamOptions != nil {
		t.Error("expected no StreamOptions for incompatible compat")
	}
}

func TestBuildRequestBase_ToolChoice(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "gpt-4o", MaxTokens: 256,
		ToolChoice: &core.ToolChoiceOption{Type: "tool", Name: "get_weather"},
		Tools:      []core.EyrieTool{{Name: "get_weather", Description: "Get weather", Parameters: map[string]interface{}{"type": "object"}}},
	}, false, &OpenAICompat)
	if req.ToolChoice == nil {
		t.Fatal("expected ToolChoice")
	}
	tc, ok := req.ToolChoice.(map[string]interface{})
	if !ok {
		t.Fatalf("ToolChoice type = %T", req.ToolChoice)
	}
	if tc["type"] != "function" {
		t.Errorf("ToolChoice type = %v", tc["type"])
	}
}

func TestBuildRequestBase_ResponseFormat(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "gpt-4o", MaxTokens: 256,
		ResponseFormat: &core.ResponseFormat{Type: "json_object"},
	}, false, &OpenAICompat)
	if req.ResponseFormat == nil {
		t.Fatal("expected ResponseFormat")
	}
}

func TestBuildRequestBase_ReasoningEffort(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompatConfig{SupportsReasoningEffort: true}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "o3", MaxTokens: 1024, ReasoningEffort: "high",
	}, false, compat)
	if req.ReasoningEffort != "high" {
		t.Errorf("ReasoningEffort = %q", req.ReasoningEffort)
	}
}

func TestOpenAIToolChoice(t *testing.T) {
	t.Parallel()
	tc := OpenAIToolChoice(nil)
	if tc != nil {
		t.Errorf("expected nil, got %v", tc)
	}
	if got := OpenAIToolChoice(&core.ToolChoiceOption{Type: "auto"}); got != "auto" {
		t.Errorf("auto = %v", got)
	}
	if got := OpenAIToolChoice(&core.ToolChoiceOption{Type: "none"}); got != "none" {
		t.Errorf("none = %v", got)
	}
	if got := OpenAIToolChoice(&core.ToolChoiceOption{Type: "any"}); got != "required" {
		t.Errorf("any = %v", got)
	}
	if got := OpenAIToolChoice(&core.ToolChoiceOption{Type: "tool"}); got != "required" {
		t.Errorf("tool no name = %v", got)
	}
	if got := OpenAIToolChoice(&core.ToolChoiceOption{Type: "custom"}); got != "custom" {
		t.Errorf("custom = %v", got)
	}
	withName := OpenAIToolChoice(&core.ToolChoiceOption{Type: "tool", Name: "get_weather"})
	m, ok := withName.(map[string]interface{})
	if !ok {
		t.Fatalf("tool with name type = %T", withName)
	}
	if m["type"] != "function" {
		t.Errorf("type = %v", m["type"])
	}
}

func TestOpenAIClient_MimoAuthRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if req.Header.Get("api-key") == "" {
				t.Error("expected api-key header on first call")
			}
			return jsonResponse(http.StatusUnauthorized, map[string]any{"error": "invalid"}), nil
		}
		if req.Header.Get("Authorization") != "Bearer tp-key" {
			t.Errorf("expected Bearer on retry, got %q", req.Header.Get("Authorization"))
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"id": "chatcmpl-456",
			"choices": []map[string]any{
				{"message": map[string]any{"content": "retried"}, "finish_reason": "stop"},
			},
		}), nil
	})
	c := NewOpenAIClient("tp-key", "", nil)
	c.SetRetry(core.RetryConfig{RetryConfig: types.RetryConfig{MaxRetries: 0}})
	c.SetMimoAuth()
	c.httpClient = &http.Client{Transport: transport}
	resp, err := c.Chat(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "retried" {
		t.Errorf("content = %q", resp.Content)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestBuildRequestBase_ToolResults(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{
		{Role: "user", ToolResults: []core.ToolResult{{ToolUseID: "call_1", Content: "42"}}},
	}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256}, false, nil)
	if len(req.Messages) != 1 {
		t.Fatalf("messages = %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "tool" {
		t.Errorf("role = %v", req.Messages[0]["role"])
	}
	if req.Messages[0]["tool_call_id"] != "call_1" {
		t.Errorf("tool_call_id = %v", req.Messages[0]["tool_call_id"])
	}
}

func TestBuildRequestBase_ToolUse(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{
		{Role: "assistant", Content: "Let me check", ToolUse: []core.ToolCall{{ID: "call_1", Name: "get_weather", Arguments: map[string]interface{}{"city": "NYC"}}}},
	}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256}, false, nil)
	if len(req.Messages) != 1 {
		t.Fatalf("messages = %d", len(req.Messages))
	}
	toolCalls := req.Messages[0]["tool_calls"].([]map[string]interface{})
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls = %d", len(toolCalls))
	}
	if toolCalls[0]["id"] != "call_1" {
		t.Errorf("tool_call id = %v", toolCalls[0]["id"])
	}
}

func TestBuildRequestBase_ContentParts(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{
		{Role: "user", ContentParts: []core.ContentPart{
			{Type: "text", Text: "desc"},
			{Type: "image_url", ImageURL: &core.ImageURLPart{URL: "https://example.com/img.png", Detail: "high"}},
		}},
	}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content blocks = %d", len(content))
	}
	if content[1]["type"] != "image_url" {
		t.Errorf("block type = %v", content[1]["type"])
	}
}

func TestBuildRequestBase_LegacyImages(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{
		{Role: "user", Content: "Check", Images: []string{"https://example.com/img.png"}},
	}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256}, false, nil)
	content := req.Messages[0]["content"].([]map[string]interface{})
	if len(content) != 2 {
		t.Fatalf("content blocks = %d", len(content))
	}
}

func TestBuildRequestBase_CacheRole(t *testing.T) {
	t.Parallel()
	compat := &OpenAICompatConfig{SupportsCacheRole: true}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "moonshot-v1", MaxTokens: 256,
		KimiContextCacheID: "cache_abc", KimiCacheResetTTL: true,
	}, false, compat)
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "cache" {
		t.Errorf("first msg role = %v", req.Messages[0]["role"])
	}
	if req.Messages[0]["reset_ttl"] != true {
		t.Errorf("expected reset_ttl")
	}
}

func TestBuildRequestBase_ZAIThinking(t *testing.T) {
	t.Parallel()
	enabled := true
	compat := &OpenAICompatConfig{ThinkingFormat: "zai"}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "glm-4", MaxTokens: 256,
		GLMThinkingEnabled: &enabled,
	}, false, compat)
	if req.Thinking == nil || req.Thinking["type"] != "enabled" {
		t.Errorf("Thinking = %v", req.Thinking)
	}
}

func TestBuildRequestBase_LongCatDefaultsThinkingDisabled(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "LongCat-2.0", MaxTokens: 256,
	}, false, &LongCatCompat)
	if req.Thinking == nil || req.Thinking["type"] != "disabled" {
		t.Fatalf("LongCat default Thinking = %v, want type=disabled", req.Thinking)
	}
	enabled := true
	reqOn := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "LongCat-2.0", MaxTokens: 256,
		GLMThinkingEnabled: &enabled,
	}, false, &LongCatCompat)
	if reqOn.Thinking == nil || reqOn.Thinking["type"] != "enabled" {
		t.Fatalf("LongCat opt-in Thinking = %v, want type=enabled", reqOn.Thinking)
	}
}

func TestBuildRequestBase_AgnesThinking(t *testing.T) {
	t.Parallel()
	enabled := true
	compat := &OpenAICompatConfig{ThinkingFormat: "agnes", OmitMaxTokens: true}
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "agnes-2.0-flash", MaxTokens: 0,
		GLMThinkingEnabled: &enabled,
	}, false, compat)
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs["enable_thinking"] != true {
		t.Errorf("ChatTemplateKwargs = %v", req.ChatTemplateKwargs)
	}
	if req.Thinking != nil {
		t.Errorf("Agnes OpenAI path should not set thinking object, got %v", req.Thinking)
	}
}

func TestBuildRequestBase_OpenRouterReasoning(t *testing.T) {
	t.Parallel()
	enabled := true
	disabled := false
	reqOn := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "openrouter/auto", MaxTokens: 256, ThinkingEnabled: &enabled,
	}, false, &OpenRouterCompat)
	if reqOn.Reasoning == nil || reqOn.Reasoning["enabled"] != true {
		t.Fatalf("OpenRouter on Reasoning = %v", reqOn.Reasoning)
	}
	reqOff := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "openrouter/auto", MaxTokens: 256, ThinkingEnabled: &disabled,
	}, false, &OpenRouterCompat)
	if reqOff.Reasoning == nil || reqOff.Reasoning["effort"] != "none" {
		t.Fatalf("OpenRouter off Reasoning = %v, want effort=none", reqOff.Reasoning)
	}
}

func TestBuildRequestBase_KimiDeepSeekXiaomiThinking(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		compat *OpenAICompatConfig
	}{
		{"kimi", &KimiCompat},
		{"deepseek", &DeepSeekCompat},
		{"xiaomi", &XiaomiCompat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
				Model: "m", MaxTokens: 256,
			}, false, tc.compat)
			if req.Thinking == nil || req.Thinking["type"] != "disabled" {
				t.Fatalf("default Thinking = %v, want type=disabled", req.Thinking)
			}
			enabled := true
			reqOn := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
				Model: "m", MaxTokens: 256, ThinkingEnabled: &enabled,
			}, false, tc.compat)
			if reqOn.Thinking == nil || reqOn.Thinking["type"] != "enabled" {
				t.Fatalf("opt-in Thinking = %v, want type=enabled", reqOn.Thinking)
			}
		})
	}
}

func TestBuildRequestBase_MiniMaxThinkingAdaptive(t *testing.T) {
	t.Parallel()
	req := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "MiniMax-M3", MaxTokens: 256,
	}, false, &MiniMaxCompat)
	if req.Thinking == nil || req.Thinking["type"] != "disabled" {
		t.Fatalf("default Thinking = %v, want type=disabled", req.Thinking)
	}
	enabled := true
	reqOn := BuildRequestBase([]core.EyrieMessage{{Role: "user", Content: "hi"}}, core.ChatOptions{
		Model: "MiniMax-M3", MaxTokens: 256, ThinkingEnabled: &enabled,
	}, false, &MiniMaxCompat)
	if reqOn.Thinking == nil || reqOn.Thinking["type"] != "adaptive" {
		t.Fatalf("opt-in Thinking = %v, want type=adaptive", reqOn.Thinking)
	}
}

func TestOpenAIClient_Ping_MimoAuthRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusUnauthorized, map[string]any{}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"data": []map[string]any{}}), nil
	})
	c := NewOpenAIClient("tp-key", "", nil)
	c.SetMimoAuth()
	c.httpClient = &http.Client{Transport: transport}
	err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestOpenAIClient_BuildOpenAIRequest(t *testing.T) {
	t.Parallel()
	c := NewOpenAIClient("sk-test", "https://api.openai.com/v1", nil)
	req, body, err := c.BuildOpenAIRequest(context.Background(), []core.EyrieMessage{{Role: "user", Content: "Hi"}}, core.ChatOptions{Model: "gpt-4o", MaxTokens: 256, Temperature: float64Ptr(0.5)}, false)
	if err != nil {
		t.Fatalf("BuildOpenAIRequest: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.Method != "POST" {
		t.Errorf("method = %q", req.Method)
	}
	if req.Header.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if body == nil {
		t.Error("expected non-nil body")
	}
}
