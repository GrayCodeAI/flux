package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GrayCodeAI/graycode-router/client"
	"github.com/GrayCodeAI/graycode-router/conversation"
	"github.com/google/uuid"
)

// This file implements an OpenAI-compatible proxy endpoint
// (POST /v1/chat/completions, LiteLLM-style) on top of graycode-router's conversation
// engine. It translates the OpenAI request/response shapes to and from the
// engine's prompt path so existing tooling that speaks the OpenAI API can talk
// to graycode-router unchanged.

// openAIChatMessage is a single message in an OpenAI chat request/response.
type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatRequest is the subset of the OpenAI /v1/chat/completions request
// body that graycode-router understands.
type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Stream      bool                `json:"stream,omitempty"`
	Temperature *float64            `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Tools       []openAITool        `json:"tools,omitempty"`
	// Accepted for compatibility but currently ignored.
	N               int             `json:"n,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stop            json.RawMessage `json:"stop,omitempty"`
	User            string          `json:"user,omitempty"`
	PresencePenalty *float64        `json:"presence_penalty,omitempty"`
	FreqPenalty     *float64        `json:"frequency_penalty,omitempty"`
}

// openAITool is the OpenAI tool/function declaration shape.
type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// openAIUsage mirrors the OpenAI usage block.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIChoice is a single completion choice (non-streaming).
type openAIChoice struct {
	Index        int               `json:"index"`
	Message      openAIChatMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// openAIChatResponse is the non-streaming response body.
type openAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

// openAIChunkDelta is the incremental message in a streaming chunk.
type openAIChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// openAIChunkChoice is a single choice within a streaming chunk.
type openAIChunkChoice struct {
	Index        int              `json:"index"`
	Delta        openAIChunkDelta `json:"delta"`
	FinishReason *string          `json:"finish_reason"`
}

// openAIChatChunk is one "chat.completion.chunk" SSE payload.
type openAIChatChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []openAIChunkChoice `json:"choices"`
}

// maxOpenAIBodyBytes is the body limit for the OpenAI-compatible proxy
// endpoint. Larger than the native 1 MiB limit because OpenAI chat
// completions carry full multi-turn conversations with tool definitions.
const maxOpenAIBodyBytes = 10 << 20 // 10 MiB

// handleOpenAIChatCompletions implements POST /v1/chat/completions.
func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	// OpenAI clients send many fields graycode-router does not consume (seed, logprobs,
	// stream_options, ...). Decode leniently rather than with the strict
	// unknown-field rejection used by decodeJSONBody.
	// Use a larger body limit than the native /prompt endpoint (1 MiB) because
	// OpenAI chat completions carry full multi-turn conversations with large
	// system prompts and tool definitions.
	r.Body = http.MaxBytesReader(w, r.Body, maxOpenAIBodyBytes)
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages is required"})
		return
	}

	system, message := splitOpenAIMessages(req.Messages)
	if message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one user message is required"})
		return
	}

	opts := conversation.PromptOpts{
		Model:        req.Model,
		SystemPrompt: system,
		MaxTokens:    req.MaxTokens,
		Temperature:  req.Temperature,
		Tools:        openAIToolsToGraycodeRouter(req.Tools),
	}

	id := "chatcmpl-" + uuid.New().String()
	created := time.Now().Unix()

	if req.Stream {
		s.streamOpenAIResponse(w, r.Context(), id, created, req.Model, func(ctx context.Context) (<-chan conversation.Event, error) {
			return s.engine.Prompt(ctx, message, opts)
		})
		return
	}

	events, err := s.engine.Prompt(r.Context(), message, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var content, nodeID, errMsg string
	for ev := range events {
		switch ev.Type {
		case conversation.EventDelta:
			content += ev.Content
		case conversation.EventDone:
			nodeID = ev.NodeID
		case conversation.EventError:
			errMsg = ev.Error
		}
	}
	if errMsg != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": errMsg})
		return
	}

	usage, finish := s.openAIUsageForNode(r.Context(), nodeID)
	resp := openAIChatResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   req.Model,
		Choices: []openAIChoice{{
			Index:        0,
			Message:      openAIChatMessage{Role: "assistant", Content: content},
			FinishReason: finish,
		}},
		Usage: usage,
	}
	writeJSON(w, http.StatusOK, resp)
}

// streamOpenAIResponse drives the engine and emits OpenAI-style SSE chunks,
// terminating with the sentinel "data: [DONE]" line.
func (s *Server) streamOpenAIResponse(w http.ResponseWriter, ctx context.Context, id string, created int64, model string, start func(context.Context) (<-chan conversation.Event, error)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	events, err := start(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Initial chunk announcing the assistant role, matching OpenAI behavior.
	s.writeOpenAIChunk(w, flusher, openAIChatChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Role: "assistant"}}},
	})

	var nodeID, errMsg string
	for ev := range events {
		switch ev.Type {
		case conversation.EventDelta:
			s.writeOpenAIChunk(w, flusher, openAIChatChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{Content: ev.Content}}},
			})
		case conversation.EventDone:
			nodeID = ev.NodeID
		case conversation.EventError:
			errMsg = ev.Error
		}
	}

	_, finish := s.openAIUsageForNode(ctx, nodeID)
	if errMsg != "" {
		// Surface the error as an SSE data event so the client knows the
		// generation failed, rather than silently replacing the finish
		// reason with "stop". OpenAI clients expect a JSON error object.
		errPayload, _ := json.Marshal(map[string]string{"error": errMsg})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", errPayload)
		flusher.Flush()
		finish = "stop"
	}
	finishReason := finish
	s.writeOpenAIChunk(w, flusher, openAIChatChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []openAIChunkChoice{{Index: 0, Delta: openAIChunkDelta{}, FinishReason: &finishReason}},
	})

	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) writeOpenAIChunk(w http.ResponseWriter, flusher http.Flusher, chunk openAIChatChunk) {
	data, _ := json.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// openAIUsageForNode reads token usage and stop reason from the persisted
// assistant node, translating to OpenAI's usage block and finish_reason. A
// missing or unreadable node yields zeroed usage and a "stop" finish reason.
func (s *Server) openAIUsageForNode(ctx context.Context, nodeID string) (openAIUsage, string) {
	if nodeID == "" || s.store == nil {
		return openAIUsage{}, "stop"
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil || node == nil {
		return openAIUsage{}, "stop"
	}
	usage := openAIUsage{
		PromptTokens:     node.TokensIn,
		CompletionTokens: node.TokensOut,
		TotalTokens:      node.TokensIn + node.TokensOut,
	}
	return usage, openAIFinishReason(node.StopReason)
}

// openAIFinishReason maps graycode-router stop reasons to OpenAI finish_reason values.
func openAIFinishReason(stop string) string {
	switch stop {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "", "end_turn", "stop", "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

// splitOpenAIMessages folds system messages into a single system prompt and
// flattens the remaining conversation into one prompt string for the engine,
// which models a turn as a single user message. The trailing user turn drives
// the prompt; any earlier turns are prepended as transcript context so
// multi-turn requests retain history.
func splitOpenAIMessages(messages []openAIChatMessage) (system, prompt string) {
	var systemParts []string
	var transcript []string
	for _, m := range messages {
		switch strings.ToLower(m.Role) {
		case "system":
			if m.Content != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "user":
			transcript = append(transcript, "User: "+m.Content)
		case "assistant":
			transcript = append(transcript, "Assistant: "+m.Content)
		default:
			transcript = append(transcript, m.Content)
		}
	}
	system = strings.Join(systemParts, "\n\n")

	if len(transcript) == 0 {
		return system, ""
	}
	// Single user turn: pass the content directly, matching /prompt behavior.
	if len(transcript) == 1 {
		return system, strings.TrimPrefix(transcript[0], "User: ")
	}
	return system, strings.Join(transcript, "\n\n")
}

// openAIToolsToGraycodeRouter converts OpenAI function/tool declarations to graycode-router tools.
func openAIToolsToGraycodeRouter(tools []openAITool) []client.GraycodeRouterTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]client.GraycodeRouterTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, client.GraycodeRouterTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	return out
}
