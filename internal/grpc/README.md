# internal/grpc — optional gRPC server

The transport is enabled with the `grpc` build tag. It uses Flux's small
Go request/response structs with the registered `json` gRPC content subtype,
which avoids generated protobuf code while retaining gRPC framing,
interceptors, deadlines, status propagation, and HTTP/2 transport.

- `grpc.go` defines the transport-independent `ChatService` contract and
  `EngineChatService`, which serves a unary Chat as one
  `conversation.Engine` prompt.
- `server_grpc.go` registers and serves `flux.v1.ChatService/Chat`.
- Clients must select `grpc.CallContentSubtype("json")`.
- There are no `.proto` files or generated stubs.
- The package is internal: hosts cannot import it, and nothing in Flux starts
  the server.

## Running

```sh
go build -tags grpc ./...
```

Callers provide a `ChatService` implementation to `Serve` or `NewServer`.
The untagged build retains only the service contract, so consumers that do not
need a network server do not link the gRPC runtime. `google.golang.org/grpc`
is still a direct requirement in `go.mod` (the tagged file needs it), so it
appears in consumers' module graphs and `go.sum`.
