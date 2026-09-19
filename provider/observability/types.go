package observability

import (
	"context"

	"github.com/GrayCodeAI/flux/llm"
	"github.com/GrayCodeAI/flux/provider/core"
)

type (
	Provider        = core.Provider
	FluxMessage     = core.FluxMessage
	FluxResponse    = core.FluxResponse
	FluxStreamEvent = core.FluxStreamEvent
	FluxUsage       = core.FluxUsage
	ToolCall        = core.ToolCall
	ChatOptions     = core.ChatOptions
	StreamResult    = core.StreamResult
)

func NewStreamResult(events <-chan FluxStreamEvent, cancel context.CancelFunc) *StreamResult {
	return llm.NewStreamResult(events, "", cancel)
}

func NewStreamResultWithRequestID(events <-chan FluxStreamEvent, requestID string, cancel context.CancelFunc) *StreamResult {
	return llm.NewStreamResult(events, requestID, cancel)
}
