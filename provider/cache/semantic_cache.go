package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/GrayCodeAI/flux/provider/core"
)

// CacheConfig controls the behavior of CachedProvider.
type CacheConfig struct {
	// MaxAge is how long cache entries remain valid. Default: 5 minutes.
	MaxAge time.Duration
	// MaxSize is the maximum number of cached responses. Default: 100.
	// When exceeded, the least-recently-used entry is evicted.
	MaxSize int
	// Enabled toggles caching. Default: true.
	// When false, the CachedProvider passes all requests through unchanged.
	Enabled bool
	// TemperatureThreshold is the temperature above which responses are not cached.
	// Default: 0.5. Responses with temperature > threshold are expected to vary,
	// so caching them would defeat the purpose.
	TemperatureThreshold float64
}

// DefaultCacheConfig returns a CacheConfig with sensible defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxAge:               5 * time.Minute,
		MaxSize:              100,
		Enabled:              true,
		TemperatureThreshold: 0.5,
	}
}

// cachedResponse holds a cached API response along with LRU metadata.
type cachedResponse struct {
	response   *FluxResponse
	createdAt  time.Time
	lastAccess time.Time

	// Doubly-linked list pointers for LRU ordering.
	prev, next *cachedResponse
	key        string
}

// CachedProvider wraps a Provider and caches non-streaming responses based on
// a hash of the input parameters. Inspired by maximhq/bifrost's caching layer.
//
// CachedProvider is safe for concurrent use.
type CachedProvider struct {
	inner Provider

	mu      sync.RWMutex
	cache   map[string]*cachedResponse
	maxAge  time.Duration
	maxSize int
	enabled bool
	tempMax float64

	// LRU doubly-linked list: head is most-recently-used, tail is least-recently-used.
	head, tail *cachedResponse
}

// Compile-time check that CachedProvider implements Provider.
var _ Provider = (*CachedProvider)(nil)

// NewCachedProvider wraps inner with a response cache configured by cfg.
// Zero-value fields in cfg are replaced with defaults.
func NewCachedProvider(inner Provider, cfg CacheConfig) *CachedProvider {
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 5 * time.Minute
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 100
	}
	if cfg.TemperatureThreshold <= 0 {
		cfg.TemperatureThreshold = 0.5
	}
	return &CachedProvider{
		inner:   inner,
		cache:   make(map[string]*cachedResponse, cfg.MaxSize),
		maxAge:  cfg.MaxAge,
		maxSize: cfg.MaxSize,
		enabled: cfg.Enabled,
		tempMax: cfg.TemperatureThreshold,
	}
}

// Name returns the inner provider's name.
func (cp *CachedProvider) Name() string {
	return cp.inner.Name()
}

// Ping delegates to the inner provider (no caching).
func (cp *CachedProvider) Ping(ctx context.Context) error {
	return cp.inner.Ping(ctx)
}

// Chat checks the cache first. On a miss, it calls the inner provider and caches
// the response (if the temperature is not too high).
func (cp *CachedProvider) Chat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*FluxResponse, error) {
	if !cp.enabled {
		return cp.inner.Chat(ctx, messages, opts)
	}

	// Skip caching for high-temperature requests -- the user wants varied output.
	if opts.Temperature != nil && *opts.Temperature > cp.tempMax {
		return cp.inner.Chat(ctx, messages, opts)
	}

	key := buildCacheKey(messages, opts)
	if key == "" {
		return cp.inner.Chat(ctx, messages, opts)
	}

	// Fast path: read lock lookup.
	if resp, ok := cp.get(key); ok {
		return resp, nil
	}

	// Cache miss: call the inner provider.
	resp, err := cp.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}

	cp.put(key, resp)
	return resp, nil
}

// StreamChat delegates to the inner provider without caching. Streaming responses
// are inherently incremental and not suitable for simple response caching.
func (cp *CachedProvider) StreamChat(ctx context.Context, messages []FluxMessage, opts ChatOptions) (*StreamResult, error) {
	return cp.inner.StreamChat(ctx, messages, opts)
}

// CacheStats returns the current number of entries in the cache.
func (cp *CachedProvider) CacheStats() CacheStatsResult {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return CacheStatsResult{
		Size:    len(cp.cache),
		MaxSize: cp.maxSize,
		Enabled: cp.enabled,
	}
}

// CacheStatsResult holds cache statistics.
type CacheStatsResult struct {
	Size    int  `json:"size"`
	MaxSize int  `json:"max_size"`
	Enabled bool `json:"enabled"`
}

// ClearCache removes all cached entries.
func (cp *CachedProvider) ClearCache() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.cache = make(map[string]*cachedResponse, cp.maxSize)
	cp.head = nil
	cp.tail = nil
}

// SetEnabled toggles caching at runtime.
func (cp *CachedProvider) SetEnabled(enabled bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.enabled = enabled
}

// ---------- internal ----------

// get looks up a cache entry by key. It returns a copy of the response if the
// entry exists and has not expired, and promotes it to the head of the LRU list.
// Expired entries are evicted inline.
func (cp *CachedProvider) get(key string) (*FluxResponse, bool) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	entry, ok := cp.cache[key]
	if !ok {
		return nil, false
	}

	// TTL check.
	if time.Since(entry.createdAt) > cp.maxAge {
		cp.removeLocked(entry)
		return nil, false
	}

	// Promote to head (most-recently-used).
	entry.lastAccess = time.Now()
	cp.promoteToHeadLocked(entry)

	return core.CopyResponse(entry.response), true
}

// put stores a response in the cache, evicting the LRU entry if necessary.
func (cp *CachedProvider) put(key string, resp *FluxResponse) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// If the key already exists, update it.
	if existing, ok := cp.cache[key]; ok {
		existing.response = core.CopyResponse(resp)
		existing.createdAt = time.Now()
		existing.lastAccess = time.Now()
		cp.promoteToHeadLocked(existing)
		return
	}

	// Evict expired entries only when approaching capacity (80% threshold).
	if len(cp.cache) >= cp.maxSize*4/5 {
		cp.evictExpiredLocked()
	}

	// Evict LRU entries until we have room.
	for len(cp.cache) >= cp.maxSize {
		if cp.tail == nil {
			break
		}
		cp.removeLocked(cp.tail)
	}

	entry := &cachedResponse{
		response:   core.CopyResponse(resp),
		createdAt:  time.Now(),
		lastAccess: time.Now(),
		key:        key,
	}
	cp.cache[key] = entry
	cp.pushHeadLocked(entry)
}

// removeLocked removes an entry from both the map and the LRU list.
// Caller must hold cp.mu.
func (cp *CachedProvider) removeLocked(entry *cachedResponse) {
	delete(cp.cache, entry.key)
	cp.unlinkLocked(entry)
}

// unlinkLocked removes an entry from the doubly-linked list.
func (cp *CachedProvider) unlinkLocked(entry *cachedResponse) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		cp.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		cp.tail = entry.prev
	}
	entry.prev = nil
	entry.next = nil
}

// pushHeadLocked inserts an entry at the head of the LRU list.
func (cp *CachedProvider) pushHeadLocked(entry *cachedResponse) {
	entry.prev = nil
	entry.next = cp.head
	if cp.head != nil {
		cp.head.prev = entry
	}
	cp.head = entry
	if cp.tail == nil {
		cp.tail = entry
	}
}

// promoteToHeadLocked moves an entry to the head of the LRU list.
func (cp *CachedProvider) promoteToHeadLocked(entry *cachedResponse) {
	if cp.head == entry {
		return // already at head
	}
	cp.unlinkLocked(entry)
	cp.pushHeadLocked(entry)
}

// evictExpiredLocked removes all entries older than maxAge.
func (cp *CachedProvider) evictExpiredLocked() {
	now := time.Now()
	// Walk from tail (oldest access) towards head.
	for entry := cp.tail; entry != nil; {
		prev := entry.prev
		if now.Sub(entry.createdAt) > cp.maxAge {
			cp.removeLocked(entry)
		}
		entry = prev
	}
}

// cacheKeyVersion is part of every key so a change to the key derivation can
// never serve an entry written under the old one.
const cacheKeyVersion = "flux-response-cache/v2"

// buildCacheKey hashes everything that can change a response: every
// ChatOptions field (tools, tool choice, max tokens, stop sequences, sampling,
// thinking, response format, identity and routing hints) and every message
// field (content parts, images, tool calls and results, provider blocks). A
// spurious miss only costs a provider call; a spurious hit serves a reply
// produced for a different request.
//
// JSON is the request's wire form, so requests that encode identically are
// identical to the provider. It returns "" when the request cannot be
// encoded (for example an unsupported Conversation value or a NaN sampling
// parameter); callers must then bypass the cache.
func buildCacheKey(messages []FluxMessage, opts ChatOptions) string {
	payload, err := json.Marshal(struct {
		Version  string        `json:"version"`
		Options  ChatOptions   `json:"options"`
		Messages []FluxMessage `json:"messages"`
	}{cacheKeyVersion, opts, messages})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// BuildCacheKey returns the deterministic key used by the response cache, or
// "" when the request cannot be cached.
func BuildCacheKey(messages []FluxMessage, opts ChatOptions) string {
	return buildCacheKey(messages, opts)
}

// CopyResponse and the deep-copy helpers live in provider/core.
// the package-local name is provided by this feature package.
