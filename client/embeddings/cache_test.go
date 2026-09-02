package embeddings

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/GrayCodeAI/eyrie/client/core"
	"github.com/GrayCodeAI/eyrie/llm"
)

// echoMock is a minimal core.Provider that echoes the last user message and
// counts calls. It replaces client.NewMockProvider, which this
// package cannot import without a cycle.
type echoMock struct {
	mu    sync.Mutex
	calls int
}

func (m *echoMock) Chat(_ context.Context, msgs []core.EyrieMessage, _ core.ChatOptions) (*core.EyrieResponse, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	content := ""
	if len(msgs) > 0 {
		content = msgs[len(msgs)-1].Content
	}
	return &core.EyrieResponse{Content: "echo: " + content, FinishReason: "stop"}, nil
}

func (m *echoMock) StreamChat(ctx context.Context, msgs []core.EyrieMessage, opts core.ChatOptions) (*core.StreamResult, error) {
	resp, err := m.Chat(ctx, msgs, opts)
	if err != nil {
		return nil, err
	}
	ch := make(chan core.EyrieStreamEvent, 2)
	ch <- core.EyrieStreamEvent{Type: "content", Content: resp.Content}
	ch <- core.EyrieStreamEvent{Type: "done", StopReason: "stop"}
	close(ch)
	return llm.NewStreamResult(ch, "", func() {}), nil
}

func (m *echoMock) Ping(context.Context) error { return nil }
func (m *echoMock) Name() string               { return "echo-mock" }

func (m *echoMock) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// stubEmbedder returns a deterministic vector chosen by keyword in the input,
// so the test can control which requests are "similar".
type stubEmbedder struct{}

func (stubEmbedder) CreateEmbedding(_ context.Context, req EmbeddingRequest) (*EmbeddingResponse, error) {
	in := ""
	if len(req.Input) > 0 {
		in = req.Input[0]
	}
	var vec []float32
	switch {
	case strings.Contains(in, "weather"):
		vec = []float32{1, 0, 0}
	case strings.Contains(in, "forecast"):
		vec = []float32{0.99, 0.1, 0} // ~0.995 cosine vs weather → hit
	case strings.Contains(in, "database"):
		vec = []float32{0, 1, 0} // orthogonal → miss
	default:
		vec = []float32{0, 0, 1}
	}
	return &EmbeddingResponse{Embeddings: [][]float32{vec}}, nil
}

func userMsg(s string) []core.EyrieMessage { return []core.EyrieMessage{{Role: "user", Content: s}} }

func TestEmbeddingCache_HitOnSimilarPrompt(t *testing.T) {
	t.Parallel()
	mock := &echoMock{}
	sp := NewEmbeddingCachedProvider(mock, stubEmbedder{}, DefaultSemanticCacheConfig())
	ctx := context.Background()

	first, err := sp.Chat(ctx, userMsg("what is the weather today"), core.ChatOptions{})
	if err != nil {
		t.Fatalf("first chat: %v", err)
	}

	// A differently-worded but semantically-similar prompt should return the
	// cached response, not a fresh echo.
	second, err := sp.Chat(ctx, userMsg("give me the weather please"), core.ChatOptions{})
	if err != nil {
		t.Fatalf("second chat: %v", err)
	}
	if second.Content != first.Content {
		t.Errorf("expected cache hit returning first response %q, got %q", first.Content, second.Content)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected inner provider called once, got %d", mock.CallCount())
	}
	if st := sp.Stats(); st.Hits != 1 || st.Misses != 1 {
		t.Errorf("unexpected stats: %+v", st)
	}
}

func TestEmbeddingCache_MissOnDissimilarPrompt(t *testing.T) {
	t.Parallel()
	mock := &echoMock{}
	sp := NewEmbeddingCachedProvider(mock, stubEmbedder{}, DefaultSemanticCacheConfig())
	ctx := context.Background()

	if _, err := sp.Chat(ctx, userMsg("weather forecast lookup"), core.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := sp.Chat(ctx, userMsg("database schema migration"), core.ChatOptions{}); err != nil {
		t.Fatal(err)
	}
	if mock.CallCount() != 2 {
		t.Errorf("expected 2 inner calls for dissimilar prompts, got %d", mock.CallCount())
	}
}

func TestEmbeddingCache_SkipsHighTemperature(t *testing.T) {
	t.Parallel()
	mock := &echoMock{}
	sp := NewEmbeddingCachedProvider(mock, stubEmbedder{}, DefaultSemanticCacheConfig())
	ctx := context.Background()
	temp := 0.9

	_, _ = sp.Chat(ctx, userMsg("weather today"), core.ChatOptions{Temperature: &temp})
	_, _ = sp.Chat(ctx, userMsg("weather today"), core.ChatOptions{Temperature: &temp})
	if mock.CallCount() != 2 {
		t.Errorf("high-temperature requests should bypass cache; got %d calls", mock.CallCount())
	}
}

func TestEmbeddingCache_DegradesOnEmbedError(t *testing.T) {
	t.Parallel()
	mock := &echoMock{}
	sp := NewEmbeddingCachedProvider(mock, errEmbedder{}, DefaultSemanticCacheConfig())
	if _, err := sp.Chat(context.Background(), userMsg("weather"), core.ChatOptions{}); err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected inner call on embed failure, got %d", mock.CallCount())
	}
}

type errEmbedder struct{}

func (errEmbedder) CreateEmbedding(_ context.Context, _ EmbeddingRequest) (*EmbeddingResponse, error) {
	return nil, errEmptyEmbedding
}

// TestEmbeddingCache_ModelIsolation pins fix C: an entry stored under one
// embedding model must NOT be served to a request embedded by a different model,
// even when the vectors are identical — they live in incompatible spaces.
func TestEmbeddingCache_ModelIsolation(t *testing.T) {
	t.Parallel()
	mock := &echoMock{}
	cfg := DefaultSemanticCacheConfig()
	cfg.EmbeddingModel = "model-A"
	sp := NewEmbeddingCachedProvider(mock, stubEmbedder{}, cfg)
	ctx := context.Background()

	// Warm the cache under model-A.
	if _, err := sp.Chat(ctx, userMsg("what is the weather today"), core.ChatOptions{}); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if mock.CallCount() != 1 {
		t.Fatalf("setup expected 1 inner call, got %d", mock.CallCount())
	}

	// Simulate the embedding model being swapped under the same cache. The prior
	// entry (tagged model-A) must be skipped, forcing a fresh inner call rather
	// than a cross-model false hit.
	sp.model = "model-B"
	if _, err := sp.Chat(ctx, userMsg("give me the weather please"), core.ChatOptions{}); err != nil {
		t.Fatalf("post-swap: %v", err)
	}
	if mock.CallCount() != 2 {
		t.Errorf("model swap must invalidate cross-model reuse; expected 2 inner calls, got %d", mock.CallCount())
	}

	// Requests under model-B should now cache and hit among themselves.
	if _, err := sp.Chat(ctx, userMsg("weather report now"), core.ChatOptions{}); err != nil {
		t.Fatalf("model-B hit: %v", err)
	}
	if mock.CallCount() != 2 {
		t.Errorf("expected a within-model-B hit (still 2 inner calls), got %d", mock.CallCount())
	}
}

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()
	if got := cosineSimilarity([]float32{1, 0}, []float32{1, 0}); got < 0.999 {
		t.Errorf("identical vectors should be ~1, got %f", got)
	}
	if got := cosineSimilarity([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Errorf("orthogonal vectors should be 0, got %f", got)
	}
	if got := cosineSimilarity([]float32{1, 0}, []float32{1}); got != 0 {
		t.Errorf("mismatched lengths should be 0, got %f", got)
	}
}
