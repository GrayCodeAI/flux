package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/eyrie/conversation"
	eyrie "github.com/GrayCodeAI/eyrie/internal/health"
	"github.com/GrayCodeAI/eyrie/internal/httputil"
	"github.com/GrayCodeAI/eyrie/storage"
)

const maxRequestBodyBytes = 1 << 20

type Server struct {
	engine        *conversation.Engine
	store         storage.Store
	analytics     storage.AnalyticsStore
	healthChecker *eyrie.HealthChecker
	reranker      Reranker // optional: provider-backed /rerank; nil => lexical fallback
	apiKey        string
	virtualKeyFor func(token string) string // optional: maps a bearer token to a virtual key id
	mux           *http.ServeMux
	handler       http.Handler // traced handler wrapping mux
	bgCtx         context.Context
	bgCancel      context.CancelFunc // cancelled on Shutdown to release bgCtx
	httpSrv       *http.Server
}

type Config struct {
	Store         storage.Store
	Analytics     storage.AnalyticsStore // optional: enables /api/usage, /api/costs
	Provider      client.Provider
	HealthChecker *eyrie.HealthChecker // optional: enables /api/health/providers
	Reranker      Reranker             // optional: provider-backed /rerank; nil => lexical fallback
	APIKey        string
	Port          int
	// VirtualKeyResolver optionally maps an inbound bearer/API-key token to a
	// logical virtual key id. When set, the resolved id is injected into the
	// request context so a BudgetProvider in the provider chain can enforce
	// per-key budgets. When nil, requests are unmetered (existing behavior).
	VirtualKeyResolver func(token string) string
}

func NewServer(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		engine:        conversation.New(cfg.Store, cfg.Provider),
		store:         cfg.Store,
		analytics:     cfg.Analytics,
		healthChecker: cfg.HealthChecker,
		reranker:      cfg.Reranker,
		apiKey:        cfg.APIKey,
		virtualKeyFor: cfg.VirtualKeyResolver,
		mux:           http.NewServeMux(),
		bgCtx:         ctx,
		bgCancel:      cancel,
	}
	s.routes()
	s.handler = httputil.SecurityHeaders(TracingMiddleware(s.mux))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	if err := httputil.ValidateAuthConfig(addr, s.apiKey); err != nil {
		return err
	}
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute, // long for streaming LLM responses
		IdleTimeout:       120 * time.Second,
	}
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server without interrupting active connections.
func (s *Server) Shutdown() error {
	if s.bgCancel != nil {
		s.bgCancel()
	}
	if s.httpSrv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ready", s.handleReady)
	s.mux.HandleFunc("POST /prompt", s.auth(s.handlePrompt))
	s.mux.HandleFunc("POST /rerank", s.auth(s.handleRerank))

	// OpenAI-compatible (LiteLLM-style) proxy endpoint.
	s.mux.HandleFunc("POST /v1/chat/completions", s.auth(s.handleOpenAIChatCompletions))
	s.mux.HandleFunc("POST /nodes/{id}/prompt", s.auth(s.handlePromptFrom))
	s.mux.HandleFunc("GET /nodes", s.auth(s.handleListNodes))
	s.mux.HandleFunc("GET /nodes/{id}", s.auth(s.handleGetNode))
	s.mux.HandleFunc("GET /nodes/{id}/tree", s.auth(s.handleGetTree))
	s.mux.HandleFunc("DELETE /nodes/{id}", s.auth(s.handleDeleteNode))
	s.mux.HandleFunc("PUT /nodes/{id}/aliases/{alias}", s.auth(s.handleCreateAlias))
	s.mux.HandleFunc("DELETE /aliases/{alias}", s.auth(s.handleDeleteAlias))

	// Analytics and health dashboard endpoints.
	s.mux.HandleFunc("GET /api/usage", s.auth(s.handleUsageAnalytics))
	s.mux.HandleFunc("GET /api/costs", s.auth(s.handleCostSummary))
	s.mux.HandleFunc("GET /api/health/providers", s.auth(s.handleProviderHealth))
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := httputil.ExtractBearerToken(r)

		if s.apiKey != "" {
			if !httputil.ConstantTimeEqual(token, s.apiKey) {
				httputil.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}

		// Attribute the request to a virtual key for downstream budget
		// enforcement, if a resolver is configured.
		if s.virtualKeyFor != nil {
			if vk := s.virtualKeyFor(token); vk != "" {
				r = r.WithContext(client.WithVirtualKey(r.Context(), vk))
			}
		}

		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady is a readiness probe (distinct from /health, which is a liveness
// probe). It returns 200 only once the server's core dependencies are
// initialized — a conversation engine and a backing store — and 503 otherwise,
// so load balancers and orchestrators do not route traffic before the server
// can actually serve it.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if s.engine == nil || s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type promptRequest struct {
	Message      string             `json:"message"`
	Model        string             `json:"model,omitempty"`
	SystemPrompt string             `json:"system_prompt,omitempty"`
	MaxTokens    int                `json:"max_tokens,omitempty"`
	Stream       bool               `json:"stream,omitempty"`
	Tools        []client.EyrieTool `json:"tools,omitempty"`
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req promptRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	opts := conversation.PromptOpts{
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		MaxTokens:    req.MaxTokens,
		Tools:        req.Tools,
	}

	if req.Stream {
		s.streamResponse(w, r.Context(), func(ctx context.Context) (<-chan conversation.Event, error) {
			return s.engine.Prompt(ctx, req.Message, opts)
		})
		return
	}

	events, err := s.engine.Prompt(r.Context(), req.Message, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.collectAndRespond(w, events)
}

func (s *Server) handlePromptFrom(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	var req promptRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	opts := conversation.PromptOpts{
		Model:        req.Model,
		SystemPrompt: req.SystemPrompt,
		MaxTokens:    req.MaxTokens,
		Tools:        req.Tools,
	}

	if req.Stream {
		s.streamResponse(w, r.Context(), func(ctx context.Context) (<-chan conversation.Event, error) {
			return s.engine.PromptFrom(ctx, nodeID, req.Message, opts)
		})
		return
	}

	events, err := s.engine.PromptFrom(r.Context(), nodeID, req.Message, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.collectAndRespond(w, events)
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.ListRootNodes(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	node, err := s.engine.ResolveNode(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (s *Server) handleGetTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodes, err := s.store.GetSubtree(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteNode(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleCreateAlias(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	alias := r.PathValue("alias")
	if err := s.store.CreateAlias(r.Context(), alias, nodeID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"alias": alias, "node_id": nodeID})
}

func (s *Server) handleDeleteAlias(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")
	if err := s.store.DeleteAlias(r.Context(), alias); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) streamResponse(w http.ResponseWriter, ctx context.Context, start func(context.Context) (<-chan conversation.Event, error)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	events, err := start(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for ev := range events {
		data, _ := json.Marshal(ev)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

func (s *Server) collectAndRespond(w http.ResponseWriter, events <-chan conversation.Event) {
	var content string
	var nodeID string
	var errMsg string
	for ev := range events {
		switch ev.Type {
		case conversation.EventDelta:
			content += ev.Content
		case conversation.EventDone:
			nodeID = ev.NodeID
		case conversation.EventError:
			errMsg = ev.Error
		}
	}
	if errMsg != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": errMsg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content": content,
		"node_id": nodeID,
	})
}

// writeJSON and decodeJSONBody delegate to internal/httputil for the
// canonical implementation. They remain as package-local wrappers so
// existing call sites in rerank.go, analytics.go, and openai_proxy.go
// don't need to be updated individually.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	httputil.WriteJSON(w, status, v)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	return httputil.DecodeJSONBody(w, r, dst)
}
