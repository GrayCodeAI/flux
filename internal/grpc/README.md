# internal/grpc — optional gRPC server

The transport is enabled with the `grpc` build tag. It uses GraycodeRouter's small
Go request/response structs with the registered `json` gRPC content subtype,
which avoids generated protobuf code while retaining gRPC framing,
interceptors, deadlines, status propagation, and HTTP/2 transport.

- `grpc.go` defines the transport-independent `ChatService` contract.
- `server_grpc.go` registers and serves `graycode-router.v1.ChatService/Chat`.
- Clients must select `grpc.CallContentSubtype("json")`.

## Running

```sh
go build -tags grpc ./...
```

Callers provide a `ChatService` implementation to `Serve` or `NewServer`.
The untagged build retains only the service contract, so consumers that do not
need a network server do not link the gRPC runtime.
