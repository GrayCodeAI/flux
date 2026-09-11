package embeddings

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/eyrie/client/core"
)

// errEmptyEmbedding is returned internally when the embedder yields no vector.
var errEmptyEmbedding = errors.New("eyrie: embedder returned no embeddings")

// SemanticCacheConfig controls the behavior of EmbeddingCachedProvider.
type SemanticCacheConfig struct {
	// MaxAge is how long cache entries remain valid. Default: 5 minutes.
	MaxAge time.Duration
	// MaxSize is the maximum number of cached responses. Default: 100.
	// When exceeded, the least-recently-used entry is evicted.
	MaxSize int
	// Enabled toggles caching. Default: true.
	Enabled bool
	// TemperatureThreshold is the temperature above which responses are not
	// cached. Default: 0.5.
	TemperatureThreshold float64
	// SimilarityThreshold is the minimum cosine similarity (0..1) for a cached
	// entry to be considered a hit. Default: 0.95.
	SimilarityThreshold float64
	// EmbeddingModel is the model passed to the Embedder. Required.
	EmbeddingModel string
}

// DefaultSemanticCacheConfig returns a SemanticCacheConfig with sensible defaults.
func DefaultSemanticCacheConfig() SemanticCacheConfig {
	return SemanticCacheConfig{
		MaxAge:               5 * time.Minute,
		MaxSize:              100,
		Enabled:              true,
		TemperatureThreshold: 0.5,
		SimilarityThreshold:  0.95,
	}
}

// semanticEntry holds a cached response keyed by its request embedding.
type semanticEntry struct {
	vector    []float32
	model     string // embedding model that produced vector; gates cross-model reuse
	response  *core.EyrieResponse
	createdAt time.Time

	// Doubly-linked list pointers for LRU ordering.
	prev, next *semanticEntry
}

// EmbeddingCachedProvider wraps a core.Provider and serves cached responses when a
// new request is semantically similar (cosine similarity above a threshold) to
// a previously seen request. Unlike CachedProvider's exact-match SHA256 keying,
// this tolerates paraphrases and minor wording changes.
//
// It is safe for concurrent use. StreamChat is passed through unchanged.
type EmbeddingCachedProvider struct {
	inner    core.Provider
	embedder Embedder

	mu      sync.Mutex
	entries []*semanticEntry
	head    *semanticEntry
	tail    *semanticEntry

	maxAge    time.Duration
	maxSize   int
	enabled   bool
	tempMax   float64
	threshold float64
	model     string

	hits   int
	misses int
}

// Compile-time check that EmbeddingCachedProvider implements core.Provider.
var _ core.Provider = (*EmbeddingCachedProvider)(nil)

// NewEmbeddingCachedProvider wraps inner with a semantic (embedding-similarity)
// cache. embedder is used to embed requests; cfg zero-value fields are replaced
// with defaults.
func NewEmbeddingCachedProvider(inner core.Provider, embedder Embedder, cfg SemanticCacheConfig) *EmbeddingCachedProvider {
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 5 * time.Minute
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 100
	}
	if cfg.TemperatureThreshold <= 0 {
		cfg.TemperatureThreshold = 0.5
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.95
	}
	return &EmbeddingCachedProvider{
		inner:     inner,
		embedder:  embedder,
		maxAge:    cfg.MaxAge,
		maxSize:   cfg.MaxSize,
		enabled:   cfg.Enabled,
		tempMax:   cfg.TemperatureThreshold,
		threshold: cfg.SimilarityThreshold,
		model:     cfg.EmbeddingModel,
	}
}

// Name returns the inner provider's name.
func (sp *EmbeddingCachedProvider) Name() string { return sp.inner.Name() }

// Ping delegates to the inner provider.
func (sp *EmbeddingCachedProvider) Ping(ctx context.Context) error { return sp.inner.Ping(ctx) }

// Chat returns a semantically-cached response on a hit; otherwise calls the
// inner provider and caches the result.
func (sp *EmbeddingCachedProvider) Chat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.EyrieResponse, error) {
	if !sp.enabled || sp.embedder == nil {
		return sp.inner.Chat(ctx, messages, opts)
	}
	// Skip caching for high-temperature requests -- the user wants variety.
	if opts.Temperature != nil && *opts.Temperature > sp.tempMax {
		return sp.inner.Chat(ctx, messages, opts)
	}

	vec, err := sp.embed(ctx, messages, opts)
	if err != nil {
		// On embedding failure, degrade gracefully to a direct call.
		return sp.inner.Chat(ctx, messages, opts)
	}

	if resp, ok := sp.lookup(vec); ok {
		return resp, nil
	}

	resp, err := sp.inner.Chat(ctx, messages, opts)
	if err != nil {
		return nil, err
	}
	sp.store(vec, resp)
	return resp, nil
}

// StreamChat delegates to the inner provider without caching.
func (sp *EmbeddingCachedProvider) StreamChat(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	return sp.inner.StreamChat(ctx, messages, opts)
}

// SemanticCacheStats reports cache occupancy and hit/miss counts.
type SemanticCacheStats struct {
	Size    int  `json:"size"`
	MaxSize int  `json:"max_size"`
	Hits    int  `json:"hits"`
	Misses  int  `json:"misses"`
	Enabled bool `json:"enabled"`
}

// Stats returns current cache statistics.
func (sp *EmbeddingCachedProvider) Stats() SemanticCacheStats {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return SemanticCacheStats{
		Size:    len(sp.entries),
		MaxSize: sp.maxSize,
		Hits:    sp.hits,
		Misses:  sp.misses,
		Enabled: sp.enabled,
	}
}

// ---------- internal ----------

// embed builds an embedding for the request from the system prompt and message
// contents.
func (sp *EmbeddingCachedProvider) embed(ctx context.Context, messages []core.EyrieMessage, opts core.ChatOptions) ([]float32, error) {
	var b strings.Builder
	if opts.System != "" {
		b.WriteString(opts.System)
		b.WriteByte('\n')
	}
	for _, m := range messages {
		b.WriteString(m.Role)
		b.WriteByte(':')
		b.WriteString(m.Content)
		b.WriteByte('\n')
	}
	resp, err := sp.embedder.CreateEmbedding(ctx, EmbeddingRequest{
		Model: sp.model,
		Input: []string{b.String()},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Embeddings) == 0 {
		return nil, errEmptyEmbedding
	}
	return resp.Embeddings[0], nil
}

// lookup returns the response of the most-similar cached entry whose cosine
// similarity meets the threshold and which has not expired.
func (sp *EmbeddingCachedProvider) lookup(vec []float32) (*core.EyrieResponse, bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	var best *semanticEntry
	bestSim := sp.threshold
	now := time.Now()
	for _, e := range sp.entries {
		if now.Sub(e.createdAt) > sp.maxAge {
			continue
		}
		// Never compare across embedding models: vectors from a different model
		// live in an incompatible space, so the cosine score would be meaningless
		// (and on a same-dimension model swap could serve a wrong response).
		if e.model != sp.model {
			continue
		}
		sim := cosineSimilarity(vec, e.vector)
		if sim >= bestSim {
			bestSim = sim
			best = e
		}
	}

	if best == nil {
		sp.misses++
		return nil, false
	}
	sp.hits++
	sp.promoteLocked(best)
	return core.CopyResponse(best.response), true
}

// store inserts a new entry, evicting expired/LRU entries as needed.
func (sp *EmbeddingCachedProvider) store(vec []float32, resp *core.EyrieResponse) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	sp.evictExpiredLocked()
	for len(sp.entries) >= sp.maxSize && sp.tail != nil {
		sp.removeLocked(sp.tail)
	}

	e := &semanticEntry{
		vector:    vec,
		model:     sp.model,
		response:  core.CopyResponse(resp),
		createdAt: time.Now(),
	}
	sp.entries = append(sp.entries, e)
	sp.pushHeadLocked(e)
}

func (sp *EmbeddingCachedProvider) pushHeadLocked(e *semanticEntry) {
	e.prev = nil
	e.next = sp.head
	if sp.head != nil {
		sp.head.prev = e
	}
	sp.head = e
	if sp.tail == nil {
		sp.tail = e
	}
}

func (sp *EmbeddingCachedProvider) unlinkLocked(e *semanticEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		sp.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		sp.tail = e.prev
	}
	e.prev = nil
	e.next = nil
}

func (sp *EmbeddingCachedProvider) promoteLocked(e *semanticEntry) {
	if sp.head == e {
		return
	}
	sp.unlinkLocked(e)
	sp.pushHeadLocked(e)
}

func (sp *EmbeddingCachedProvider) removeLocked(e *semanticEntry) {
	sp.unlinkLocked(e)
	for i, x := range sp.entries {
		if x == e {
			sp.entries = append(sp.entries[:i], sp.entries[i+1:]...)
			break
		}
	}
}

func (sp *EmbeddingCachedProvider) evictExpiredLocked() {
	now := time.Now()
	for _, e := range append([]*semanticEntry(nil), sp.entries...) {
		if now.Sub(e.createdAt) > sp.maxAge {
			sp.removeLocked(e)
		}
	}
}

// cosineSimilarity returns the cosine similarity of two vectors in [-1, 1].
// Mismatched lengths or zero vectors yield 0.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
