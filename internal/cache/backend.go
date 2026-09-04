// Pluggable distributed cache backend interface.
//
// CacheBackend abstracts the storage layer behind the in-process cache so that
// callers can swap in a distributed store (e.g. Redis) without changing the
// surrounding code. The default is an in-memory implementation; callers that do
// not opt in keep the existing in-process behavior unchanged.
//
// Everything here is stdlib-only. The Redis backend speaks RESP directly over a
// net.Conn rather than pulling in a redis client dependency.
package graycoderouter

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// CacheBackend is the storage abstraction used by the cache. Implementations
// must be safe for concurrent use by multiple goroutines.
type CacheBackend interface {
	// Get returns the value for key. The second return value reports whether the
	// key was present (and unexpired). A miss is (nil, false, nil), not an error.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Set stores val under key. A non-positive ttl means "no expiry".
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	// Delete removes key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// --- In-memory backend (default) ---

// memEntry is a single stored value with an optional expiry.
type memEntry struct {
	val       []byte
	expiresAt time.Time // zero means no expiry
}

// MemoryBackend is the default in-process CacheBackend. It stores values in a
// map guarded by a mutex and lazily evicts expired entries on access.
type MemoryBackend struct {
	mu      sync.Mutex
	entries map[string]memEntry
}

// NewMemoryBackend creates an empty in-memory CacheBackend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		entries: make(map[string]memEntry),
	}
}

// Get implements CacheBackend.
func (m *MemoryBackend) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	if !ok {
		return nil, false, nil
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		delete(m.entries, key)
		return nil, false, nil
	}
	// Return a copy so callers cannot mutate stored bytes.
	cp := make([]byte, len(e.val))
	copy(cp, e.val)
	return cp, true, nil
}

// Set implements CacheBackend.
func (m *MemoryBackend) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	cp := make([]byte, len(val))
	copy(cp, val)
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	m.mu.Lock()
	m.entries[key] = memEntry{val: cp, expiresAt: expiresAt}
	m.mu.Unlock()
	return nil
}

// Delete implements CacheBackend.
func (m *MemoryBackend) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
	return nil
}

// --- Redis backend (skeleton, stdlib-only RESP) ---

// ErrRedisNil is returned by the RESP client for a nil bulk-string reply
// (i.e. a missing key). It is handled internally and never surfaced from Get.
var ErrRedisNil = errors.New("graycode-router: redis nil reply")

// maxRespBulkLen caps the size of a single RESP bulk-string reply to prevent
// memory-exhaustion from a malicious or buggy Redis server. 64 MB is well
// beyond any legitimate cache value.
const maxRespBulkLen = 64 * 1024 * 1024

// RedisBackend is a minimal, dependency-free CacheBackend backed by Redis.
//
// It implements only the subset of commands the cache needs:
//
//	GET <key>
//	SET <key> <val> [PX <ttl-ms>]
//	DEL <key>
//
// (EXPIRE is expressed via SET ... PX rather than a separate command.)
//
// The client speaks RESP over a single net.Conn protected by a mutex. This is a
// SKELETON: it is intentionally simple (one connection, no pooling, no pipelining,
// no AUTH/SELECT) and is meant as a starting point. For production use, extend
// with connection pooling, AUTH/db selection, and automatic reconnection.
type RedisBackend struct {
	addr    string
	timeout time.Duration

	mu sync.Mutex
	// conn is lazily dialed on first use and re-dialed after an I/O error.
	conn net.Conn
	rw   *bufio.ReadWriter
}

// NewRedisBackend creates a RedisBackend targeting addr (host:port). The
// connection is established lazily on first use. A non-positive timeout falls
// back to a 5s default applied per operation.
func NewRedisBackend(addr string, timeout time.Duration) *RedisBackend {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &RedisBackend{addr: addr, timeout: timeout}
}

// Close closes the underlying connection, if any.
func (r *RedisBackend) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conn != nil {
		err := r.conn.Close()
		r.conn = nil
		r.rw = nil
		return err
	}
	return nil
}

// Get implements CacheBackend.
func (r *RedisBackend) Get(ctx context.Context, key string) ([]byte, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	reply, err := r.do(ctx, "GET", key)
	if err != nil {
		if errors.Is(err, ErrRedisNil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if reply == nil {
		return nil, false, nil
	}
	return reply, true, nil
}

// Set implements CacheBackend. A positive ttl is sent as a PX (millisecond) expiry.
func (r *RedisBackend) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	args := []string{"SET", key, string(val)}
	if ttl > 0 {
		args = append(args, "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	}
	_, err := r.do(ctx, args...)
	return err
}

// Delete implements CacheBackend.
func (r *RedisBackend) Delete(ctx context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.do(ctx, "DEL", key)
	return err
}

// do dials if needed, writes a RESP command array, and reads one reply.
// Caller must hold r.mu. On any I/O error the connection is dropped so the next
// call re-dials.
func (r *RedisBackend) do(ctx context.Context, args ...string) ([]byte, error) {
	if err := r.ensureConn(ctx); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(r.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = r.conn.SetDeadline(deadline)

	if err := writeCommand(r.rw, args); err != nil {
		r.dropConn()
		return nil, err
	}
	if err := r.rw.Flush(); err != nil {
		r.dropConn()
		return nil, err
	}
	reply, err := readReply(r.rw.Reader)
	if err != nil && !errors.Is(err, ErrRedisNil) {
		r.dropConn()
	}
	return reply, err
}

// ensureConn dials a new connection if none is open. Caller must hold r.mu.
func (r *RedisBackend) ensureConn(ctx context.Context) error {
	if r.conn != nil {
		return nil
	}
	d := net.Dialer{Timeout: r.timeout}
	conn, err := d.DialContext(ctx, "tcp", r.addr)
	if err != nil {
		return fmt.Errorf("graycode-router: redis dial %s: %w", r.addr, err)
	}
	r.conn = conn
	r.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	return nil
}

// dropConn closes and clears the connection. Caller must hold r.mu.
func (r *RedisBackend) dropConn() {
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
		r.rw = nil
	}
}

// --- Minimal RESP encoding/decoding (stdlib-only) ---

// writeCommand encodes args as a RESP array of bulk strings.
func writeCommand(w *bufio.ReadWriter, args []string) error {
	if _, err := fmt.Fprintf(w, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, a := range args {
		if _, err := fmt.Fprintf(w, "$%d\r\n", len(a)); err != nil {
			return err
		}
		if _, err := w.WriteString(a); err != nil {
			return err
		}
		if _, err := w.WriteString("\r\n"); err != nil {
			return err
		}
	}
	return nil
}

// readReply reads a single RESP reply. It supports the reply types this client
// can receive: simple strings (+), errors (-), integers (:), and bulk strings ($).
// A nil bulk string ($-1) returns ErrRedisNil. Arrays are not needed here.
func readReply(r *bufio.Reader) ([]byte, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+': // simple string, e.g. "+OK"
		return line, nil
	case '-': // error
		return nil, fmt.Errorf("graycode-router: redis error: %s", string(line))
	case ':': // integer, e.g. DEL count
		return line, nil
	case '$': // bulk string
		n, err := strconv.Atoi(string(line))
		if err != nil {
			return nil, fmt.Errorf("graycode-router: bad bulk length %q: %w", line, err)
		}
		if n < 0 {
			return nil, ErrRedisNil
		}
		if n > maxRespBulkLen {
			return nil, fmt.Errorf("graycode-router: redis bulk string length %d exceeds max %d", n, maxRespBulkLen)
		}
		buf := make([]byte, n+2) // include trailing CRLF
		if _, err := readFull(r, buf); err != nil {
			return nil, err
		}
		return buf[:n], nil
	default:
		return nil, fmt.Errorf("graycode-router: unexpected RESP prefix %q", string(prefix))
	}
}

// readLine reads up to and including CRLF and returns the content without CRLF.
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	// Strip trailing \r\n (or lone \n).
	n := len(line)
	if n >= 2 && line[n-2] == '\r' {
		return line[:n-2], nil
	}
	return line[:n-1], nil
}

// readFull reads len(buf) bytes from r.
func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
