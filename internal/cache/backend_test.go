package eyrie

import (
	"bufio"
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestMemoryBackend(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name string
		run  func(t *testing.T, b *MemoryBackend)
	}{
		{
			name: "get missing key",
			run: func(t *testing.T, b *MemoryBackend) {
				v, ok, err := b.Get(ctx, "nope")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ok {
					t.Errorf("expected miss, got hit with %q", v)
				}
			},
		},
		{
			name: "set then get",
			run: func(t *testing.T, b *MemoryBackend) {
				if err := b.Set(ctx, "k", []byte("v"), 0); err != nil {
					t.Fatalf("Set: %v", err)
				}
				v, ok, err := b.Get(ctx, "k")
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if !ok || string(v) != "v" {
					t.Errorf("expected hit 'v', got ok=%v v=%q", ok, v)
				}
			},
		},
		{
			name: "delete",
			run: func(t *testing.T, b *MemoryBackend) {
				_ = b.Set(ctx, "k", []byte("v"), 0)
				if err := b.Delete(ctx, "k"); err != nil {
					t.Fatalf("Delete: %v", err)
				}
				if _, ok, _ := b.Get(ctx, "k"); ok {
					t.Error("expected miss after delete")
				}
				// Deleting a missing key is not an error.
				if err := b.Delete(ctx, "missing"); err != nil {
					t.Errorf("delete missing should not error: %v", err)
				}
			},
		},
		{
			name: "ttl expiry",
			run: func(t *testing.T, b *MemoryBackend) {
				_ = b.Set(ctx, "k", []byte("v"), 20*time.Millisecond)
				if _, ok, _ := b.Get(ctx, "k"); !ok {
					t.Error("expected hit before expiry")
				}
				time.Sleep(30 * time.Millisecond)
				if _, ok, _ := b.Get(ctx, "k"); ok {
					t.Error("expected miss after expiry")
				}
			},
		},
		{
			name: "stored bytes are copied",
			run: func(t *testing.T, b *MemoryBackend) {
				val := []byte("orig")
				_ = b.Set(ctx, "k", val, 0)
				val[0] = 'X' // mutate caller's slice
				v, _, _ := b.Get(ctx, "k")
				if string(v) != "orig" {
					t.Errorf("stored value was mutated: %q", v)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, NewMemoryBackend())
		})
	}
}

// fakeRedis is a minimal in-process RESP server backed by a map. It speaks just
// enough of the protocol to validate the RedisBackend client without a live Redis.
type fakeRedis struct {
	ln net.Listener

	mu    sync.Mutex
	store map[string]string
}

func newFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fr := &fakeRedis{ln: ln, store: make(map[string]string)}
	go fr.serve()
	return fr
}

func (fr *fakeRedis) addr() string { return fr.ln.Addr().String() }

func (fr *fakeRedis) close() { _ = fr.ln.Close() }

func (fr *fakeRedis) serve() {
	for {
		conn, err := fr.ln.Accept()
		if err != nil {
			return
		}
		go fr.handle(conn)
	}
}

func (fr *fakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		args, err := readCommand(r)
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}
		fr.respond(conn, args)
	}
}

func (fr *fakeRedis) respond(conn net.Conn, args []string) {
	switch args[0] {
	case "GET":
		fr.mu.Lock()
		v, ok := fr.store[args[1]]
		fr.mu.Unlock()
		if !ok {
			_, _ = conn.Write([]byte("$-1\r\n"))
			return
		}
		_, _ = conn.Write([]byte("$" + strconv.Itoa(len(v)) + "\r\n" + v + "\r\n"))
	case "SET":
		fr.mu.Lock()
		fr.store[args[1]] = args[2]
		fr.mu.Unlock()
		_, _ = conn.Write([]byte("+OK\r\n"))
	case "DEL":
		fr.mu.Lock()
		_, existed := fr.store[args[1]]
		delete(fr.store, args[1])
		fr.mu.Unlock()
		n := 0
		if existed {
			n = 1
		}
		_, _ = conn.Write([]byte(":" + strconv.Itoa(n) + "\r\n"))
	default:
		_, _ = conn.Write([]byte("-ERR unknown command\r\n"))
	}
}

// readCommand reads one RESP array-of-bulk-strings command from r.
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		return nil, nil
	}
	n, err := strconv.Atoi(string(line[1:]))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := readLine(r) // $<len>
		if err != nil {
			return nil, err
		}
		ln, err := strconv.Atoi(string(hdr[1:]))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, ln+2)
		if _, err := readFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:ln]))
	}
	return args, nil
}

func TestRedisBackendAgainstFakeServer(t *testing.T) {
	t.Parallel()
	fr := newFakeRedis(t)
	defer fr.close()

	b := NewRedisBackend(fr.addr(), time.Second)
	defer b.Close()
	ctx := context.Background()

	// Miss on a key that was never set.
	if _, ok, err := b.Get(ctx, "absent"); err != nil || ok {
		t.Fatalf("expected clean miss, got ok=%v err=%v", ok, err)
	}

	// Set then get.
	if err := b.Set(ctx, "k", []byte("hello"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok, err := b.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || string(v) != "hello" {
		t.Errorf("expected hit 'hello', got ok=%v v=%q", ok, v)
	}

	// Set with TTL (sent as PX) still round-trips via the fake server.
	if err := b.Set(ctx, "k2", []byte("world"), 5*time.Second); err != nil {
		t.Fatalf("Set with ttl: %v", err)
	}
	if v, ok, _ := b.Get(ctx, "k2"); !ok || string(v) != "world" {
		t.Errorf("expected 'world', got ok=%v v=%q", ok, v)
	}

	// Delete then miss.
	if err := b.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := b.Get(ctx, "k"); ok {
		t.Error("expected miss after delete")
	}
}

// Confirm both backends satisfy the interface at compile time.
var (
	_ CacheBackend = (*MemoryBackend)(nil)
	_ CacheBackend = (*RedisBackend)(nil)
)
