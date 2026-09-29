// Package grpc is Flux's optional gRPC transport for the conversation engine.
//
// This untagged file holds the transport-independent ChatService contract,
// its request/response structs, a no-op default (NewChatService) and
// EngineChatService, which serves a unary Chat as one conversation.Engine
// prompt. It does not import google.golang.org/grpc.
//
// server_grpc.go (build tag "grpc") imports google.golang.org/grpc, registers
// a "json" codec and serves flux.v1.ChatService/Chat. There are no .proto
// files or generated stubs: clients select grpc.CallContentSubtype("json").
// Because of that tagged file, google.golang.org/grpc is a direct requirement
// in go.mod and appears in consumers' module graphs, although untagged builds
// do not link it. The package is internal, so hosts cannot import it, and
// nothing in Flux starts the server. See README.md.
package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/GrayCodeAI/flux/conversation"
)

// ChatRequest is the unary Chat request payload. It mirrors the HTTP
// /prompt request fields so a gRPC implementation can reuse the conversation
// engine without translation.
type ChatRequest struct {
	Model        string
	SystemPrompt string
	Message      string
	MaxTokens    int
}

// ChatResponse is the unary Chat response payload.
type ChatResponse struct {
	Content      string
	NodeID       string
	FinishReason string
}

// ChatService is the flux gRPC service contract: a single unary Chat RPC.
// EngineChatService is the conversation.Engine-backed implementation.
type ChatService interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

// noopChatService is the placeholder ChatService. It returns ErrUnimplemented
// so callers get a clear signal that no backend was supplied.
type noopChatService struct{}

// ErrUnimplemented is returned by the placeholder ChatService from
// NewChatService and by an EngineChatService built with a nil engine.
var ErrUnimplemented = errUnimplemented{}

type errUnimplemented struct{}

func (errUnimplemented) Error() string { return "flux/grpc: ChatService not implemented" }

func (noopChatService) Chat(_ context.Context, _ *ChatRequest) (*ChatResponse, error) {
	return nil, ErrUnimplemented
}

// NewChatService returns the placeholder (no-op) ChatService. Use
// NewEngineChatService for a working backend.
func NewChatService() ChatService {
	return noopChatService{}
}

// EngineChatService adapts conversation.Engine to the ChatService contract: a
// unary Chat RPC becomes a single Prompt over the conversation engine, with
// the streamed assistant content aggregated into the response.
type EngineChatService struct {
	engine *conversation.Engine
}

// NewEngineChatService returns a ChatService backed by a conversation.Engine.
// Pass it to NewServer or Serve (build tag "grpc") to expose the engine over
// gRPC.
func NewEngineChatService(engine *conversation.Engine) ChatService {
	return &EngineChatService{engine: engine}
}

// Chat runs a single prompt through the conversation engine and aggregates the
// streamed assistant content into a ChatResponse.
func (s *EngineChatService) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if s.engine == nil {
		return nil, ErrUnimplemented
	}
	if req == nil {
		return nil, fmt.Errorf("flux/grpc: chat request is required")
	}
	ch, err := s.engine.Prompt(ctx, req.Message, conversation.PromptOpts{
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		MaxTokens:    req.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	var content string
	var nodeID string
	for ev := range ch {
		switch ev.Type {
		case conversation.EventDelta:
			content += ev.Content
		case conversation.EventError:
			if ev.Error != "" {
				return nil, errors.New(ev.Error)
			}
		case conversation.EventDone:
			nodeID = ev.NodeID
		}
	}
	return &ChatResponse{Content: content, NodeID: nodeID, FinishReason: "stop"}, nil
}
