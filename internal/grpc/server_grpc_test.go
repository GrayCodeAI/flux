//go:build grpc

package grpc

import (
	"context"
	"net"
	"testing"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testChatService struct{}

func (testChatService) Chat(_ context.Context, req *ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{Content: "echo: " + req.Message, NodeID: "node-1", FinishReason: "stop"}, nil
}

func TestServerChatRoundTrip(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	server, err := NewServer(testChatService{})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = lis.Close()
	})

	conn, err := googlegrpc.NewClient(
		"passthrough:///graycode-router-test",
		googlegrpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		googlegrpc.WithTransportCredentials(insecure.NewCredentials()),
		googlegrpc.WithDefaultCallOptions(googlegrpc.CallContentSubtype("json")),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var response ChatResponse
	if err := conn.Invoke(context.Background(), "/graycoderouter.v1.ChatService/Chat", &ChatRequest{Message: "hello"}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Content != "echo: hello" || response.NodeID != "node-1" || response.FinishReason != "stop" {
		t.Fatalf("unexpected response: %+v", response)
	}
}
