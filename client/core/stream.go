package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// SSEEvent represents a single Server-Sent Event.
type SSEEvent struct {
	Event string
	Data  string
}

// SSE stream constants.
const (
	sseChannelBuffer  = 128
	sseScannerInitBuf = 64 * 1024
	sseScannerMaxBuf  = 2 * 1024 * 1024
	// StreamChannelBuffer is the default buffer size for provider event channels.
	StreamChannelBuffer = 256
)

// ParseSSEStream reads an SSE stream and sends events to a channel.
// The goroutine closes the channel and body when done or context is cancelled.
// Scanner errors are emitted as SSEEvent with Event="error" so callers can detect truncation.
func ParseSSEStream(ctx context.Context, body io.ReadCloser, logger *slog.Logger) <-chan SSEEvent {
	ch := make(chan SSEEvent, sseChannelBuffer)
	go func() {
		defer close(ch)
		defer func() { _ = body.Close() }()

		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, sseScannerInitBuf), sseScannerMaxBuf)

		var event, data strings.Builder
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if line == "" {
				if data.Len() > 0 {
					select {
					case ch <- SSEEvent{Event: strings.TrimSpace(event.String()), Data: strings.TrimSpace(data.String())}:
					case <-ctx.Done():
						return
					}
				}
				event.Reset()
				data.Reset()
				continue
			}
			if strings.HasPrefix(line, "event:") {
				event.WriteString(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimPrefix(line, "data:"))
			}
		}
		if err := scanner.Err(); err != nil {
			// Context cancellation produces "context canceled" from the
			// scanner when the body is closed; that is the expected
			// shutdown path, not a stream error. Skip the warning and
			// the synthetic error event so callers (and operators) don't
			// see noise for every normal cancel/close.
			if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(err, context.Canceled) {
				return
			}
			logger.Warn("SSE stream read error", "error", err)
			select {
			case ch <- SSEEvent{Event: "error", Data: fmt.Sprintf("stream read error: %v", err)}:
			case <-ctx.Done():
			}
		}
	}()
	return ch
}

// --- Anthropic streaming ---

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"content_block,omitempty"`
}

// ProcessAnthropicStream converts Anthropic SSE events to EyrieStreamEvents.
// Handles text, tool_use (with input_json_delta), and thinking blocks.
func ProcessAnthropicStream(ctx context.Context, sseEvents <-chan SSEEvent, logger *slog.Logger) <-chan EyrieStreamEvent {
	return ProcessAnthropicStreamWithOpts(ctx, sseEvents, logger, time.Now())
}

func ProcessAnthropicStreamWithOpts(ctx context.Context, sseEvents <-chan SSEEvent, logger *slog.Logger, start time.Time) <-chan EyrieStreamEvent {
	ch := make(chan EyrieStreamEvent, StreamChannelBuffer)
	go func() {
		defer close(ch)

		// Track current tool call being accumulated
		type toolAccum struct {
			id, name string
			jsonBuf  strings.Builder
		}
		var currentTool *toolAccum
		blockTypes := make(map[int]string)
		var stopReason string

		// TTFT: wall-clock milliseconds to the first content or tool-call delta.
		var ttftMs int
		var ttftEmitted bool

		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sseEvents:
				if !ok {
					// Stream closed — Emit partial tool call as error if incomplete
					if currentTool != nil {
						Emit(ctx, ch, EyrieStreamEvent{
							Type:  "error",
							Error: fmt.Sprintf("stream closed with incomplete tool call: %s", currentTool.name),
						})
						currentTool = nil
					}
					return
				}
				// Propagate SSE-level errors
				if evt.Event == "error" {
					Emit(ctx, ch, EyrieStreamEvent{Type: "error", Error: evt.Data})
					return
				}
				data := strings.TrimSpace(evt.Data)
				if data == "" || data == "[DONE]" {
					continue
				}
				var ae anthropicStreamEvent
				if err := json.Unmarshal([]byte(data), &ae); err != nil {
					logger.Debug("failed to parse anthropic event", "error", err)
					continue
				}

				switch ae.Type {
				case "content_block_start":
					if ae.ContentBlock != nil {
						blockTypes[ae.Index] = ae.ContentBlock.Type
						switch ae.ContentBlock.Type {
						case "tool_use":
							currentTool = &toolAccum{id: ae.ContentBlock.ID, name: ae.ContentBlock.Name}
						case "thinking":
							// thinking block started, deltas will follow
						}
					}

				case "content_block_delta":
					if ae.Delta == nil {
						continue
					}
					switch ae.Delta.Type {
					case "text_delta":
						if ae.Delta.Text != "" {
							if strings.Contains(blockTypes[ae.Index], "thinking") {
								Emit(ctx, ch, EyrieStreamEvent{Type: "thinking", Thinking: ae.Delta.Text})
							} else {
								// TTFT: record and Emit on the first content token.
								if !ttftEmitted {
									ttftEmitted = true
									ttftMs = int(time.Since(start) / time.Millisecond)
									Emit(ctx, ch, EyrieStreamEvent{Type: "ttft", TTFT: ttftMs})
								}
								Emit(ctx, ch, EyrieStreamEvent{Type: "content", Content: ae.Delta.Text})
							}
						}
					case "input_json_delta":
						if currentTool != nil && ae.Delta.PartialJSON != "" {
							// TTFT: first tool-call argument fragment.
							if !ttftEmitted {
								ttftEmitted = true
								ttftMs = int(time.Since(start) / time.Millisecond)
								Emit(ctx, ch, EyrieStreamEvent{Type: "ttft", TTFT: ttftMs})
							}
							currentTool.jsonBuf.WriteString(ae.Delta.PartialJSON)
						}
					case "thinking_delta":
						if ae.Delta.Text != "" {
							Emit(ctx, ch, EyrieStreamEvent{Type: "thinking", Thinking: ae.Delta.Text})
						}
					}

				case "content_block_stop":
					delete(blockTypes, ae.Index)
					if currentTool != nil {
						rawJSON := currentTool.jsonBuf.String()
						var args map[string]interface{}
						if err := json.Unmarshal([]byte(rawJSON), &args); err != nil {
							logger.Warn("invalid tool call JSON accumulated", "tool", currentTool.name, "error", err)
							Emit(ctx, ch, EyrieStreamEvent{
								Type:  "error",
								Error: fmt.Sprintf("invalid tool call JSON for %s: %v", currentTool.name, err),
							})
						} else {
							Emit(ctx, ch, EyrieStreamEvent{
								Type:     "tool_call",
								ToolCall: &ToolCall{ID: currentTool.id, Name: currentTool.name, Arguments: args},
							})
						}
						currentTool = nil
					}

				case "message_stop":
					Emit(ctx, ch, EyrieStreamEvent{Type: "done", StopReason: stopReason, TTFTms: ttftMs})
					return

				case "message_delta":
					// Contains usage (output_tokens) and stop_reason
					var md struct {
						Delta *struct {
							StopReason string `json:"stop_reason"`
						} `json:"delta"`
						Usage *struct {
							OutputTokens int `json:"output_tokens"`
						} `json:"usage"`
					}
					if err := json.Unmarshal([]byte(data), &md); err != nil {
						logger.Warn("stream: failed to parse anthropic content_block_stop metadata", "error", err)
					}
					if md.Delta != nil && md.Delta.StopReason != "" {
						stopReason = md.Delta.StopReason
					}
					if md.Usage != nil && md.Usage.OutputTokens > 0 {
						Emit(ctx, ch, EyrieStreamEvent{
							Type: "usage",
							Usage: &EyrieUsage{
								CompletionTokens: md.Usage.OutputTokens,
							},
						})
					}

				case "message_start":
					// Contains input token count
					var ms struct {
						Message struct {
							Usage struct {
								InputTokens  int `json:"input_tokens"`
								OutputTokens int `json:"output_tokens"`
							} `json:"usage"`
						} `json:"message"`
					}
					if err := json.Unmarshal([]byte(data), &ms); err != nil {
						logger.Warn("stream: failed to parse anthropic message_start", "error", err)
					}
					if ms.Message.Usage.InputTokens > 0 {
						Emit(ctx, ch, EyrieStreamEvent{
							Type: "usage",
							Usage: &EyrieUsage{
								PromptTokens: ms.Message.Usage.InputTokens,
							},
						})
					}

				case "error":
					Emit(ctx, ch, EyrieStreamEvent{Type: "error", Error: data})
					return
				}
			}
		}
	}()
	return ch
}

// --- OpenAI streaming ---

type openaiStreamChoice struct {
	Delta struct {
		Content string `json:"content,omitempty"`
		// ReasoningContent carries the model's chain-of-thought on OpenAI-compatible
		// providers that expose it as a separate channel (DeepSeek, Qwen, MiniMax-M2,
		// some OpenRouter/z.ai routes). Without this field eyrie would drop it.
		ReasoningContent string `json:"reasoning_content,omitempty"`
		Role             string `json:"role,omitempty"`
		ToolCalls        []struct {
			Index    int    `json:"index"`
			ID       string `json:"id,omitempty"`
			Function struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

type openaiStreamPayload struct {
	Choices []openaiStreamChoice `json:"choices"`
	Usage   *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// thinkSplitter separates inline <think>...</think> reasoning out of streamed
// content. Some OpenAI-compatible models (DeepSeek, Qwen, and local models via
// Ollama/openrouter) Emit chain-of-thought inline in delta.content wrapped in
// <think> tags rather than in a dedicated reasoning_content field. Left in the
// content stream this leaks reasoning into the assistant's answer text. The
// splitter is fed each content delta in order and routes the pieces to the
// content and thinking channels respectively. It is byte-accurate across chunk
// boundaries: a tag split across two deltas is buffered until it can be matched.
type thinkSplitter struct {
	inThink bool
	pending string // buffered bytes that may be the start of a (partial) tag
}

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// feed consumes one content delta and returns the visible content and the
// reasoning extracted from it (either may be empty).
func (s *thinkSplitter) feed(delta string) (content, thinking string) {
	buf := s.pending + delta
	s.pending = ""
	var c, t strings.Builder
	for len(buf) > 0 {
		if s.inThink {
			idx := strings.Index(buf, thinkClose)
			if idx == -1 {
				// No close tag yet. Keep a possible partial close tag suffix
				// buffered; Emit the rest as thinking.
				keep := partialSuffixLen(buf, thinkClose)
				t.WriteString(buf[:len(buf)-keep])
				s.pending = buf[len(buf)-keep:]
				buf = ""
				continue
			}
			t.WriteString(buf[:idx])
			buf = buf[idx+len(thinkClose):]
			s.inThink = false
		} else {
			idx := strings.Index(buf, thinkOpen)
			if idx == -1 {
				keep := partialSuffixLen(buf, thinkOpen)
				c.WriteString(buf[:len(buf)-keep])
				s.pending = buf[len(buf)-keep:]
				buf = ""
				continue
			}
			c.WriteString(buf[:idx])
			buf = buf[idx+len(thinkOpen):]
			s.inThink = true
		}
	}
	return c.String(), t.String()
}

// partialSuffixLen returns the length of the longest suffix of s that is a
// proper (non-empty) prefix of tag — i.e. bytes that might begin tag but can't
// be confirmed until more input arrives.
func partialSuffixLen(s, tag string) int {
	max := len(tag) - 1
	if max > len(s) {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}

// ProcessOpenAIStream converts OpenAI SSE events to EyrieStreamEvents.
// Handles text deltas and tool call streaming by index.
// Also instruments TTFT (time-to-first-token) and runs a RepeatDetector to
// synthesise a finish_reason:"repeat" event when the stream loops.
func ProcessOpenAIStream(ctx context.Context, sseEvents <-chan SSEEvent, logger *slog.Logger) <-chan EyrieStreamEvent {
	return ProcessOpenAIStreamWithOpts(ctx, sseEvents, logger, DefaultRepeatDetector(), time.Now())
}

func ProcessOpenAIStreamWithOpts(ctx context.Context, sseEvents <-chan SSEEvent, logger *slog.Logger, repeat *RepeatDetector, start time.Time) <-chan EyrieStreamEvent {
	ch := make(chan EyrieStreamEvent, StreamChannelBuffer)
	go func() {
		defer close(ch)

		// Accumulate tool calls by index
		type toolAccum struct {
			id, name string
			argsBuf  strings.Builder
		}
		tools := make(map[int]*toolAccum)
		toolsEmitted := false
		var splitter thinkSplitter

		// Health signals accumulated across the stream so a reasoning-only or
		// empty response can be reported as a precise diagnostic at the end.
		var sawReasoning bool
		var contentLen int
		var finishReason string

		// TTFT: wall-clock milliseconds to the first content or tool-call delta.
		var ttftMs int
		var ttftEmitted bool

		emitTools := func() {
			if toolsEmitted {
				return
			}
			toolsEmitted = true
			// Sort by index so tool calls are emitted in the order the
			// model produced them, not in random map-iteration order.
			indices := make([]int, 0, len(tools))
			for idx := range tools {
				indices = append(indices, idx)
			}
			sort.Ints(indices)
			for _, idx := range indices {
				t := tools[idx]
				args := map[string]interface{}{}
				if err := json.Unmarshal([]byte(t.argsBuf.String()), &args); err != nil {
					// On parse failure, pass the raw string as _raw so
					// the caller sees something rather than a nil map.
					args = map[string]interface{}{"_raw": t.argsBuf.String()}
				}
				Emit(ctx, ch, EyrieStreamEvent{
					Type:     "tool_call",
					ToolCall: &ToolCall{ID: t.id, Name: t.name, Arguments: args},
				})
			}
		}

		// finish emits the terminal done event, preceded by a non-fatal health
		// diagnostic when the response produced no usable content/tool-calls.
		// The diagnostic rides an error-type event (so consumers that only
		// read .Error still see it) but is marked non-fatal via the Warning
		// field: downstream layers must surface it without terminating the
		// stream, because the terminal done event follows immediately.
		finish := func(stopReason string) {
			emitTools()
			health := DetectResponseHealth(ResponseSignals{
				SawReasoning: sawReasoning,
				ContentLen:   contentLen,
				ToolCalls:    len(tools),
				FinishReason: finishReason,
				StreamEnded:  true,
			})
			if d := health.Diagnostic(); d != "" {
				Emit(ctx, ch, EyrieStreamEvent{Type: "error", Error: d, Warning: d})
			}
			Emit(ctx, ch, EyrieStreamEvent{Type: "done", StopReason: stopReason, TTFTms: ttftMs})
		}

		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sseEvents:
				if !ok {
					finish("")
					return
				}
				// Propagate SSE-level errors
				if evt.Event == "error" {
					Emit(ctx, ch, EyrieStreamEvent{Type: "error", Error: evt.Data})
					return
				}
				data := strings.TrimSpace(evt.Data)
				if data == "" || data == "[DONE]" {
					finish("")
					return
				}

				var oe openaiStreamPayload
				if err := json.Unmarshal([]byte(data), &oe); err != nil {
					logger.Debug("failed to parse openai event", "error", err)
					continue
				}

				// Emit usage if present (final chunk with stream_options.include_usage)
				if oe.Usage != nil {
					Emit(ctx, ch, EyrieStreamEvent{
						Type: "usage",
						Usage: &EyrieUsage{
							PromptTokens:     oe.Usage.PromptTokens,
							CompletionTokens: oe.Usage.CompletionTokens,
							TotalTokens:      oe.Usage.TotalTokens,
						},
					})
				}

				if len(oe.Choices) == 0 {
					continue
				}
				choice := oe.Choices[0]

				// Reasoning exposed as a dedicated channel (DeepSeek/Qwen/etc.):
				// route it to a thinking event so it never lands in the answer.
				if choice.Delta.ReasoningContent != "" {
					sawReasoning = true
					Emit(ctx, ch, EyrieStreamEvent{Type: "thinking", Thinking: choice.Delta.ReasoningContent})
				}

				// Text content. Split out any inline <think>...</think> reasoning
				// so models that embed chain-of-thought in content don't leak it.
				if choice.Delta.Content != "" {
					content, thinking := splitter.feed(choice.Delta.Content)
					if thinking != "" {
						sawReasoning = true
						Emit(ctx, ch, EyrieStreamEvent{Type: "thinking", Thinking: thinking})
					}
					if content != "" {
						// TTFT: record and Emit a dedicated event on the first token.
						if !ttftEmitted {
							ttftEmitted = true
							ttftMs = int(time.Since(start) / time.Millisecond)
							Emit(ctx, ch, EyrieStreamEvent{Type: "ttft", TTFT: ttftMs})
						}
						contentLen += len(content)
						Emit(ctx, ch, EyrieStreamEvent{Type: "content", Content: content})
						// Repeat detection: abort if stream is looping.
						if repeat != nil {
							repeat.Feed(content)
							if repeat.IsRepeating() {
								finishReason = "repeat"
								finish("repeat")
								return
							}
						}
					}
				}

				// Tool calls by index
				for _, tc := range choice.Delta.ToolCalls {
					// TTFT: first tool-call argument fragment counts as the first token.
					if !ttftEmitted && tc.Function.Arguments != "" {
						ttftEmitted = true
						ttftMs = int(time.Since(start) / time.Millisecond)
						Emit(ctx, ch, EyrieStreamEvent{Type: "ttft", TTFT: ttftMs})
					}
					t, ok := tools[tc.Index]
					if !ok {
						t = &toolAccum{}
						tools[tc.Index] = t
					}
					if tc.ID != "" {
						t.id = tc.ID
					}
					if tc.Function.Name != "" {
						t.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						t.argsBuf.WriteString(tc.Function.Arguments)
					}
				}

				if choice.FinishReason != nil {
					finishReason = *choice.FinishReason
					finish(*choice.FinishReason)
					return
				}
			}
		}
	}()
	return ch
}

func Emit(ctx context.Context, ch chan<- EyrieStreamEvent, evt EyrieStreamEvent) {
	select {
	case ch <- evt:
	case <-ctx.Done():
	}
}

// ParseInlineToolCalls detects and extracts tool calls embedded in text content.
// Three text-embedded formats are recognized, tried in order:
//
//   - Moonshot/kimi (canopywave): <|tool_calls_section_begin|> <|tool_call_begin|>
//     functions.ToolName:0 <|tool_call_argument_begin|> {"arg":"val"} <|tool_call_end|>
//     <|tool_calls_section_end|>
//   - Hermes/Nous (Qwen, and most OpenAI-compatible local models served via
//     vLLM/SGLang/Ollama): each call is a JSON object {"name":...,"arguments":{...}}
//     wrapped in <tool_call>...</tool_call> XML tags, with parallel calls emitted
//     as repeated tag pairs. This is the format Qwen2.5/QwQ Emit; Qwen3-Coder uses
//     a structurally similar but distinct "qwen3_xml" variant not handled here.
//   - Bare JSON brace-match (last resort): a {"name":...,"arguments":{...}} object
//     found via strings.Index(text, `{"`) / strings.LastIndex(text, "}") without
//     any wrapper tags. Used by some fine-tuned models and direct JSON outputs.
//
// Providers that already surface tool calls through the structured tool_calls
// channel never reach this path — it is a fallback for models that inline them.
func ParseInlineToolCalls(text string) (cleanText string, toolCalls []ToolCall) {
	const marker = "<|tool_calls_section_begin|>"
	idx := strings.Index(text, marker)
	if idx < 0 {
		// Try Hermes/Nous <tool_call> format first.
		clean, calls := parseHermesToolCalls(text)
		if len(calls) > 0 {
			return clean, calls
		}
		// Tier 3: bare brace-match JSON fallback.
		return parseBraceMatchToolCall(text)
	}

	cleanText = strings.TrimSpace(text[:idx])
	section := text[idx:]

	// Extract individual tool calls
	calls := strings.Split(section, "<|tool_call_begin|>")
	for _, call := range calls[1:] { // skip first empty part
		endIdx := strings.Index(call, "<|tool_call_end|>")
		if endIdx < 0 {
			continue
		}
		call = call[:endIdx]

		// Parse function name: "functions.ToolName:0"
		nameStart := strings.TrimSpace(call)
		argStart := strings.Index(nameStart, "<|tool_call_argument_begin|>")
		if argStart < 0 {
			continue
		}
		funcLine := strings.TrimSpace(nameStart[:argStart])
		argJSON := strings.TrimSpace(nameStart[argStart+len("<|tool_call_argument_begin|>"):])

		// Extract tool name from "functions.ToolName:0"
		toolName := funcLine
		if dotIdx := strings.Index(funcLine, "."); dotIdx >= 0 {
			toolName = funcLine[dotIdx+1:]
		}
		if colonIdx := strings.Index(toolName, ":"); colonIdx >= 0 {
			toolName = toolName[:colonIdx]
		}

		// Parse arguments JSON
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argJSON), &args); err != nil {
			args = map[string]interface{}{"_raw": argJSON}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        fmt.Sprintf("inline_%s_%d", toolName, len(toolCalls)),
			Name:      toolName,
			Arguments: args,
		})
	}

	return cleanText, toolCalls
}

// hermesToolCallOpen/Close delimit a Hermes/Nous-format tool call. Qwen and most
// OpenAI-compatible local models Emit calls as <tool_call>{json}</tool_call>.
const (
	hermesToolCallOpen  = "<tool_call>"
	hermesToolCallClose = "</tool_call>"
)

// parseHermesToolCalls extracts Hermes/Nous <tool_call>{"name","arguments"}</tool_call>
// blocks from text. It returns the text with every (well-formed) tool-call block
// removed and the parsed calls. A block whose body cannot be parsed as a function
// call is left in the cleaned text untouched, so malformed output is surfaced to
// the user rather than silently dropped.
func parseHermesToolCalls(text string) (cleanText string, toolCalls []ToolCall) {
	if !strings.Contains(text, hermesToolCallOpen) {
		return text, nil
	}

	var clean strings.Builder
	rest := text
	for {
		open := strings.Index(rest, hermesToolCallOpen)
		if open < 0 {
			clean.WriteString(rest)
			break
		}
		body := rest[open+len(hermesToolCallOpen):]
		close := strings.Index(body, hermesToolCallClose)
		if close < 0 {
			// Unterminated tag — keep the remainder verbatim and stop.
			clean.WriteString(rest)
			break
		}
		inner := strings.TrimSpace(body[:close])
		after := body[close+len(hermesToolCallClose):]

		if tc, ok := parseHermesCall(inner, len(toolCalls)); ok {
			// Drop the block from the visible text.
			clean.WriteString(rest[:open])
			toolCalls = append(toolCalls, tc)
		} else {
			// Not a recognizable call: leave the whole block in place.
			clean.WriteString(rest[:open+len(hermesToolCallOpen)+close+len(hermesToolCallClose)])
		}
		rest = after
	}

	cleanText = strings.TrimSpace(clean.String())
	return cleanText, toolCalls
}

// parseHermesCall parses a single Hermes tool-call body — a JSON object with
// "name" and "arguments" keys. "arguments" may be either a nested object or a
// JSON-encoded string (both forms appear in the wild); a string is decoded one
// level. seq disambiguates the synthetic call ID when the model omits one.
func parseHermesCall(inner string, seq int) (ToolCall, bool) {
	var raw struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(inner), &raw); err != nil || raw.Name == "" {
		return ToolCall{}, false
	}

	args := map[string]interface{}{}
	if len(raw.Arguments) > 0 {
		if err := json.Unmarshal(raw.Arguments, &args); err != nil {
			// "arguments" came through as a JSON string — decode once more.
			var asString string
			if json.Unmarshal(raw.Arguments, &asString) == nil && asString != "" {
				if err := json.Unmarshal([]byte(asString), &args); err != nil {
					args = map[string]interface{}{"_raw": asString}
				}
			} else {
				args = map[string]interface{}{"_raw": string(raw.Arguments)}
			}
		}
	}

	return ToolCall{
		ID:        fmt.Sprintf("hermes_%s_%d", raw.Name, seq),
		Name:      raw.Name,
		Arguments: args,
	}, true
}

// parseBraceMatchToolCall is the third and last tier in ParseInlineToolCalls.
// It looks for a bare {"name":...,"arguments":{...}} JSON object in text
// (without any wrapper tags) using a start/end brace search and attempts to
// parse it as a Hermes-shape tool call. On success it returns the text before
// the JSON object as clean text and a single parsed tool call. On failure —
// including malformed JSON or a missing "name" field — it returns the original
// text verbatim with no tool calls, so the caller sees the raw model output
// rather than a silent drop.
func parseBraceMatchToolCall(text string) (cleanText string, toolCalls []ToolCall) {
	start := strings.Index(text, `{"`)
	if start < 0 {
		return text, nil
	}
	end := strings.LastIndex(text, "}")
	if end <= start {
		return text, nil
	}
	// The JSON object must run to the end of the response. Trailing prose
	// after a JSON-shaped snippet is a strong signal the snippet is an
	// example or explanation (e.g. inside a code fence, or mid-sentence),
	// not an actual tool call — executing such a false positive would be
	// surprising and potentially destructive.
	if strings.TrimSpace(text[end+1:]) != "" {
		return text, nil
	}
	candidate := text[start : end+1]
	if tc, ok := parseHermesCall(candidate, 0); ok {
		return strings.TrimSpace(text[:start]), []ToolCall{tc}
	}
	return text, nil
}

// Error-body parsing now lives in provider_errors.go (parseProviderError +
// formatAPIError), which classifies the failure and echoes the upstream
// correlation id uniformly across all provider request paths.
