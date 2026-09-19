package controlplane

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GrayCodeAI/flux/catalog"
	"github.com/GrayCodeAI/flux/provider/core"
	"github.com/GrayCodeAI/flux/router"
)

type replyProvider string

func (p replyProvider) Name() string               { return string(p) }
func (p replyProvider) Ping(context.Context) error { return nil }
func (p replyProvider) Chat(context.Context, []core.FluxMessage, core.ChatOptions) (*core.FluxResponse, error) {
	return &core.FluxResponse{Content: string(p)}, nil
}
func (p replyProvider) StreamChat(context.Context, []core.FluxMessage, core.ChatOptions) (*core.StreamResult, error) {
	return nil, nil
}

func testManifest(revision uint64, deployment string) Manifest {
	return Manifest{
		Revision: revision,
		Catalog:  catalog.SeedCatalog(),
		Routing: router.RoutingPolicy{Default: []router.RoutingStage{{
			Deployments: []router.DeploymentChoice{{DeploymentID: deployment, Weight: 100}},
		}}},
		Deployments: []Deployment{{ID: deployment}},
	}
}

func testReplica(t *testing.T) *Replica {
	t.Helper()
	r, err := NewReplica(func(_ context.Context, id string) (core.Provider, error) {
		return replyProvider(id), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func chatReply(t *testing.T, r *Replica) string {
	t.Helper()
	response, err := r.Chat(context.Background(), nil, core.ChatOptions{Model: "anthropic/claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	return response.Content
}

func TestReplicaAtomicApplyAndLastGood(t *testing.T) {
	r := testReplica(t)
	initial := testManifest(1, "anthropic-direct")
	if err := r.Apply(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	if got := chatReply(t, r); got != "anthropic-direct" {
		t.Fatalf("reply = %q", got)
	}
	// Mutating caller-owned state after Apply cannot affect an active route.
	initial.Deployments[0].ID = "broken"
	initial.Routing.Default[0].Deployments[0].DeploymentID = "broken"
	if got := chatReply(t, r); got != "anthropic-direct" {
		t.Fatalf("caller mutated active route: %q", got)
	}
	before := manifestFromHandler(t, r.Handler())
	servedDigest, err := manifestDigest(before)
	if err != nil {
		t.Fatal(err)
	}
	if servedDigest != r.current.Load().digest {
		t.Fatal("catalog compilation changed the served manifest after digesting")
	}
	if before.Routing.Default[0].Deployments[0].DeploymentID != "anthropic-direct" {
		t.Fatal("served manifest was mutated by compilation or caller")
	}
	if err := r.Apply(context.Background(), before); err != nil {
		t.Fatalf("identical revision should be idempotent: %v", err)
	}
	conflict := before
	conflict.Routing.Default = []router.RoutingStage{{Deployments: []router.DeploymentChoice{{DeploymentID: "anthropic-vertex", Weight: 100}}}}
	if err := r.Apply(context.Background(), conflict); err == nil {
		t.Fatal("same-revision conflict accepted")
	}
	invalid := testManifest(2, "anthropic-direct")
	invalid.Catalog.Deployments = nil
	if err := r.Apply(context.Background(), invalid); err == nil {
		t.Fatal("invalid catalog accepted")
	}
	if r.Revision() != 1 || chatReply(t, r) != "anthropic-direct" {
		t.Fatal("rejected update changed last-good route")
	}
	if err := r.Apply(context.Background(), testManifest(2, "anthropic-vertex")); err != nil {
		t.Fatal(err)
	}
	if r.Revision() != 2 || chatReply(t, r) != "anthropic-vertex" {
		t.Fatal("new route was not published")
	}
	if err := r.Apply(context.Background(), testManifest(1, "anthropic-direct")); err == nil {
		t.Fatal("stale revision accepted")
	}
}

func manifestFromHandler(t *testing.T, handler http.Handler) Manifest {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("manifest HTTP status = %d", recorder.Code)
	}
	var manifest Manifest
	if err := json.Unmarshal(recorder.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func serveInMemory(source *PeerSource, handlers map[string]http.Handler) {
	source.client = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		handler := handlers[req.URL.Path]
		if handler == nil {
			return nil, errors.New("peer unavailable")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})}
}

func TestPeerSourceSignedReplicationAndTampering(t *testing.T) {
	ctx := context.Background()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leader := testReplica(t)
	if err := leader.Apply(ctx, testManifest(3, "anthropic-direct")); err != nil {
		t.Fatal(err)
	}
	handler, err := leader.SignedHandler("publisher", private)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewPeerSource([]string{"http://localhost/leader"}, map[string]ed25519.PublicKey{"publisher": public})
	if err != nil {
		t.Fatal(err)
	}
	serveInMemory(source, map[string]http.Handler{"/leader": handler})
	follower := testReplica(t)
	if err := follower.Refresh(ctx, source); err != nil {
		t.Fatal(err)
	}
	if follower.Revision() != 3 || chatReply(t, follower) != "anthropic-direct" {
		t.Fatal("signed manifest did not replicate")
	}
	serveInMemory(source, nil)
	if err := follower.Refresh(ctx, source); err == nil {
		t.Fatal("unreachable source unexpectedly succeeded")
	}
	if follower.Revision() != 3 || chatReply(t, follower) != "anthropic-direct" {
		t.Fatal("source outage interrupted last-good route")
	}
	bad := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(SignedManifest{
			Manifest: testManifest(4, "anthropic-vertex"), KeyID: "publisher", Signature: []byte("invalid"),
		})
	})
	badSource, err := NewPeerSource([]string{"http://localhost/bad"}, map[string]ed25519.PublicKey{"publisher": public})
	if err != nil {
		t.Fatal(err)
	}
	serveInMemory(badSource, map[string]http.Handler{"/bad": bad})
	if err := follower.Refresh(ctx, badSource); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("invalid signature result = %v", err)
	}
	if follower.Revision() != 3 {
		t.Fatal("bad signature changed revision")
	}
}

func TestPeerSourceChoosesHighestRevisionAndRejectsTopConflict(t *testing.T) {
	serve := func(manifest Manifest, delay time.Duration) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(delay)
			_ = json.NewEncoder(w).Encode(manifest)
		})
	}
	lowA := serve(testManifest(1, "anthropic-direct"), 0)
	lowB := serve(testManifest(1, "anthropic-vertex"), 0)
	high := serve(testManifest(2, "anthropic-direct"), 30*time.Millisecond)
	source, err := NewPeerSource([]string{"http://localhost/low-a", "http://localhost/low-b", "http://localhost/high"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	serveInMemory(source, map[string]http.Handler{"/low-a": lowA, "/low-b": lowB, "/high": high})
	latest, err := source.Latest(context.Background())
	if err != nil || latest.Revision != 2 {
		t.Fatalf("highest revision = %d, %v", latest.Revision, err)
	}
	highB := serve(testManifest(2, "anthropic-vertex"), 0)
	conflicted, err := NewPeerSource([]string{"http://localhost/high", "http://localhost/high-b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	serveInMemory(conflicted, map[string]http.Handler{"/high": high, "/high-b": highB})
	if _, err := conflicted.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("top-revision conflict result = %v", err)
	}
}

type changingSource struct {
	mu       sync.Mutex
	manifest Manifest
	err      error
}

func (s *changingSource) Latest(context.Context) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.manifest, s.err
}

func TestReplicaRunRefreshesAndPreservesLastGood(t *testing.T) {
	r := testReplica(t)
	source := &changingSource{manifest: testManifest(1, "anthropic-direct")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, source, 5*time.Millisecond, nil) }()
	t.Cleanup(func() { cancel(); <-done })
	deadline := time.After(time.Second)
	for r.Revision() != 1 {
		select {
		case <-deadline:
			t.Fatal("initial refresh did not publish")
		case <-time.After(time.Millisecond):
		}
	}
	source.mu.Lock()
	source.manifest = testManifest(2, "anthropic-vertex")
	source.mu.Unlock()
	for r.Revision() != 2 {
		select {
		case <-deadline:
			t.Fatal("periodic refresh did not publish")
		case <-time.After(time.Millisecond):
		}
	}
	source.mu.Lock()
	source.err = errors.New("source unavailable")
	source.mu.Unlock()
	time.Sleep(15 * time.Millisecond)
	if r.Revision() != 2 || chatReply(t, r) != "anthropic-vertex" {
		t.Fatal("source failure interrupted last-good route")
	}
}

func TestPeerSourceRejectsUnsafeEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://example.com/manifest", "http://user@localhost/manifest", "file:///tmp/manifest"} {
		if _, err := NewPeerSource([]string{endpoint}, nil); err == nil {
			t.Errorf("unsafe URL %q accepted", endpoint)
		}
	}
	if _, err := NewPeerSource(make([]string, maxPeerEndpoints+1), nil); err == nil {
		t.Fatal("unbounded peer list accepted")
	}
}

func TestReplicaRejectsBrokenRouteBeforePublishing(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Manifest)
	}{
		{"unknown deployment", func(m *Manifest) { m.Routing.Default[0].Deployments[0].DeploymentID = "not-configured" }},
		{"zero weight", func(m *Manifest) { m.Routing.Default[0].Deployments[0].Weight = 0 }},
		{"negative retries", func(m *Manifest) { m.Routing.Default[0].Retries = -1 }},
		{"excessive retries", func(m *Manifest) { m.Routing.Default[0].Retries = 1000000 }},
		{"duplicate deployment", func(m *Manifest) { m.Deployments = append(m.Deployments, m.Deployments[0]) }},
		{"empty model mapping", func(m *Manifest) {
			m.Deployments[0].ModelMappings = map[string]string{"anthropic/claude-sonnet-4-6": ""}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := testManifest(1, "anthropic-direct")
			tc.edit(&m)
			r := testReplica(t)
			if err := r.Apply(context.Background(), m); err == nil {
				t.Fatal("broken route was published")
			}
			if r.Revision() != 0 {
				t.Fatal("rejected manifest became active")
			}
		})
	}
}
