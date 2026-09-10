package grpc

import (
	"context"
	"testing"
)

// TestEngineChatServiceContract verifies the constructor surface and the noop
// fallback. A full engine-backed round-trip is covered by server_grpc_test.go
// (build tag "grpc") and requires a store-backed conversation.Engine.
func TestEngineChatServiceContract(t *testing.T) {
	if NewChatService() == nil {
		t.Fatal("NewChatService returned nil")
	}
	if svc := NewEngineChatService(nil); svc == nil {
		t.Fatal("NewEngineChatService returned nil")
	}
	if _, err := NewEngineChatService(nil).Chat(context.Background(), &ChatRequest{Message: "hi"}); err != ErrUnimplemented {
		t.Fatalf("expected ErrUnimplemented for nil engine, got %v", err)
	}
}
