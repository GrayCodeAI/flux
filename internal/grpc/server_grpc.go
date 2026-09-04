//go:build grpc

package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

// JSONCodec is the wire codec used by GraycodeRouter's dependency-light gRPC surface.
// Clients select it with grpc.CallContentSubtype("json").
type JSONCodec struct{}

func (JSONCodec) Name() string { return "json" }
func (JSONCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func init() {
	encoding.RegisterCodec(JSONCodec{})
}

// NewServer creates a gRPC server and registers the supplied ChatService.
func NewServer(svc ChatService, opts ...googlegrpc.ServerOption) (*googlegrpc.Server, error) {
	if svc == nil {
		return nil, fmt.Errorf("graycode-router/grpc: chat service is required")
	}
	server := googlegrpc.NewServer(opts...)
	server.RegisterService(&chatServiceDesc, svc)
	return server, nil
}

// Serve registers svc and serves requests until the listener is closed or the
// server is stopped.
func Serve(lis net.Listener, svc ChatService, opts ...googlegrpc.ServerOption) error {
	if lis == nil {
		return fmt.Errorf("graycode-router/grpc: listener is required")
	}
	server, err := NewServer(svc, opts...)
	if err != nil {
		return err
	}
	return server.Serve(lis)
}

func chatHandler(srv interface{}, ctx context.Context, decode func(interface{}) error, interceptor googlegrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(ChatRequest)
	if err := decode(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ChatService).Chat(ctx, req)
	}
	info := &googlegrpc.UnaryServerInfo{Server: srv, FullMethod: "/graycoderouter.v1.ChatService/Chat"}
	handler := func(ctx context.Context, request interface{}) (interface{}, error) {
		return srv.(ChatService).Chat(ctx, request.(*ChatRequest))
	}
	return interceptor(ctx, req, info, handler)
}

var chatServiceDesc = googlegrpc.ServiceDesc{
	ServiceName: "graycoderouter.v1.ChatService",
	HandlerType: (*ChatService)(nil),
	Methods: []googlegrpc.MethodDesc{{
		MethodName: "Chat",
		Handler:    chatHandler,
	}},
	Metadata: "graycode-router/v1/chat.json",
}
