package adapters

import (
	"strings"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/hawk-core-contracts/llm"
)

// AnthropicBaseFromOpenAIV1 strips a trailing /v1 from an OpenAI-compatible base URL.
// OpenCode Go Anthropic models share the host without the /v1 suffix.
func AnthropicBaseFromOpenAIV1(openAIBase string) string {
	base := strings.TrimRight(strings.TrimSpace(openAIBase), "/")
	if strings.HasSuffix(base, "/v1") {
		return strings.TrimSuffix(base, "/v1")
	}
	return base
}

// streamResultFromChat synthesizes a StreamResult from a completed chat response.
// Used by Poolside when recovering a non-streaming answer into the stream path.
func streamResultFromChat(resp *core.EyrieResponse) *core.StreamResult {
	out := make(chan core.EyrieStreamEvent, core.StreamChannelBuffer)
	go func() {
		defer close(out)
		if resp == nil {
			return
		}
		if strings.TrimSpace(resp.Thinking) != "" {
			out <- core.EyrieStreamEvent{Type: "thinking", Thinking: resp.Thinking}
		}
		if strings.TrimSpace(resp.Content) != "" {
			out <- core.EyrieStreamEvent{Type: "content", Content: resp.Content}
		}
		for i := range resp.ToolCalls {
			tc := resp.ToolCalls[i]
			out <- core.EyrieStreamEvent{Type: "tool_call", ToolCall: &tc}
		}
		if resp.Usage != nil {
			out <- core.EyrieStreamEvent{Type: "usage", Usage: resp.Usage}
		}
		stop := resp.FinishReason
		if stop == "" {
			stop = "stop"
		}
		out <- core.EyrieStreamEvent{Type: "done", StopReason: stop}
	}()
	return llm.NewStreamResult(out, "", func() {})
}
