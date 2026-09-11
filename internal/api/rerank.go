package api

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// This file implements POST /rerank: given a query and a set of candidate
// documents, it returns the documents ranked by relevance to the query.
//
// When no provider-backed reranker is configured (the default), eyrie falls
// back to a zero-dependency lexical scorer (cosine similarity over
// term-frequency vectors) so the endpoint is always functional. A
// provider-backed path (e.g. Cohere rerank) can be injected via Config.Reranker.

// rerankRequest is the request body for POST /rerank. It mirrors the common
// Cohere/LiteLLM rerank shape.
type rerankRequest struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResult is a single scored document, identified by its original index
// in the request's documents slice.
type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// rerankResponse is the response body for POST /rerank.
type rerankResponse struct {
	Model   string         `json:"model,omitempty"`
	Results []rerankResult `json:"results"`
}

// Reranker is the provider-backed reranking interface. A concrete
// implementation (e.g. backed by Cohere's /rerank API or a cross-encoder model)
// can be injected so /rerank uses model-quality scores instead of the local
// lexical fallback.
type Reranker interface {
	// Rerank returns relevance scores in [0,1] for each document, in the same
	// order as the input documents.
	Rerank(ctx context.Context, model, query string, documents []string) ([]float64, error)
}

// handleRerank implements POST /rerank.
func (s *Server) handleRerank(w http.ResponseWriter, r *http.Request) {
	var req rerankRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}
	if len(req.Documents) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documents is required"})
		return
	}

	var scores []float64
	// Provider-backed path: used only when a Reranker is configured.
	if s.reranker != nil {
		var err error
		scores, err = s.reranker.Rerank(r.Context(), req.Model, req.Query, req.Documents)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if len(scores) != len(req.Documents) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reranker returned invalid score count"})
			return
		}
	} else {
		// Zero-dependency lexical fallback.
		scores = lexicalRerankScores(req.Query, req.Documents)
	}

	results := make([]rerankResult, len(req.Documents))
	for i, sc := range scores {
		results[i] = rerankResult{Index: i, RelevanceScore: sc}
	}
	// Stable sort by descending score; ties keep ascending original index.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	if req.TopN > 0 && req.TopN < len(results) {
		results = results[:req.TopN]
	}

	writeJSON(w, http.StatusOK, rerankResponse{Model: req.Model, Results: results})
}

// lexicalRerankScores scores each document against the query using cosine
// similarity over term-frequency vectors. Scores fall in [0,1]; a document
// sharing no terms with the query scores 0. This is intentionally dependency
// free and deterministic, providing a sensible default ordering when no
// model-backed reranker is configured.
func lexicalRerankScores(query string, documents []string) []float64 {
	qVec := termFreq(query)
	scores := make([]float64, len(documents))
	for i, doc := range documents {
		scores[i] = cosineSim(qVec, termFreq(doc))
	}
	return scores
}

// termFreq tokenizes s into lowercased word tokens and returns their counts.
func termFreq(s string) map[string]float64 {
	tf := make(map[string]float64)
	for _, tok := range tokenizeLexical(s) {
		tf[tok]++
	}
	return tf
}

// tokenizeLexical splits s on any non-letter/non-digit rune and lowercases the
// resulting tokens.
func tokenizeLexical(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return fields
}

// cosineSim computes the cosine similarity between two term-frequency vectors.
// It returns 0 when either vector is empty.
func cosineSim(a, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for term, av := range a {
		normA += av * av
		if bv, ok := b[term]; ok {
			dot += av * bv
		}
	}
	for _, bv := range b {
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
