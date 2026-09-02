// Package operationsgraph projects Eyrie routing and normalized generation
// telemetry into the portable hawk-eco graph contract.
package operationsgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/eyrie/graph"
	llmcontracts "github.com/GrayCodeAI/eyrie/llm"
)

const SchemaVersion = "eyrie.graph/v1"

type Input struct {
	Route           *llmcontracts.ResolvedRoute
	Usage           *llmcontracts.EyrieUsage
	FinishReason    string
	RequestID       string
	Content         string
	ToolCallCount   int
	ObservedAt      time.Time
	Scope           graphcontracts.Scope
	CorrelationID   string
	ProducerVersion string
}

type Export struct {
	SchemaVersion string                 `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Scope         graphcontracts.Scope   `json:"scope,omitempty"`
	Nodes         []graphcontracts.Node  `json:"nodes"`
	Edges         []graphcontracts.Edge  `json:"edges"`
	Events        []graphcontracts.Event `json:"events"`
}

func Build(input Input) (*Export, error) {
	if input.Route == nil && input.Usage == nil {
		return nil, errors.New("operationsgraph: route or usage is required")
	}
	at := input.ObservedAt.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	observationDigest := digest(
		input.RequestID,
		input.CorrelationID,
		at.Format(time.RFC3339Nano),
	)
	out := &Export{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   at,
		Scope:         input.Scope,
		Nodes:         []graphcontracts.Node{},
		Edges:         []graphcontracts.Edge{},
		Events:        []graphcontracts.Event{},
	}
	var routeRef graphcontracts.Ref
	if input.Route != nil {
		attrs := map[string]string{
			"entity":             "route",
			"provider_digest":    digest(input.Route.Provider),
			"model_digest":       digest(input.Route.Model),
			"deployment_routing": strconv.FormatBool(input.Route.DeploymentRouting),
		}
		ref, err := addNode(out, "route", attrs, observationDigest, input, at)
		if err != nil {
			return nil, err
		}
		routeRef = ref
	}
	if input.Usage != nil {
		attrs := map[string]string{
			"entity":                "generation",
			"prompt_tokens":         strconv.Itoa(input.Usage.PromptTokens),
			"completion_tokens":     strconv.Itoa(input.Usage.CompletionTokens),
			"total_tokens":          strconv.Itoa(input.Usage.TotalTokens),
			"cache_creation_tokens": strconv.Itoa(input.Usage.CacheCreationTokens),
			"cache_read_tokens":     strconv.Itoa(input.Usage.CacheReadTokens),
			"thinking_tokens":       strconv.Itoa(input.Usage.ThinkingTokens),
			"finish_reason":         strings.TrimSpace(input.FinishReason),
			"request_id_digest":     digest(input.RequestID),
			"content_digest":        digest(input.Content),
			"tool_call_count":       strconv.Itoa(max(input.ToolCallCount, 0)),
		}
		generationRef, err := addNode(out, "generation", attrs, observationDigest, input, at)
		if err != nil {
			return nil, err
		}
		if routeRef.ID != "" {
			edge := graphcontracts.Edge{
				ID:        "eyrie/edge/" + digest(generationRef.ID, routeRef.ID),
				Kind:      graphcontracts.EdgeDependsOn,
				From:      generationRef,
				To:        routeRef,
				Scope:     input.Scope,
				CreatedAt: at,
				Provenance: graphcontracts.Provenance{
					Producer: "eyrie",
					Version:  strings.TrimSpace(input.ProducerVersion),
					SourceID: observationDigest,
				},
			}
			if err := edge.Validate(); err != nil {
				return nil, fmt.Errorf("operationsgraph: route edge: %w", err)
			}
			out.Edges = append(out.Edges, edge)
		}
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].ID < out.Edges[j].ID })
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].ID < out.Events[j].ID })
	return out, nil
}

func addNode(
	out *Export,
	entity string,
	attrs map[string]string,
	observationDigest string,
	input Input,
	at time.Time,
) (graphcontracts.Ref, error) {
	id := "eyrie/" + entity + "/" + digest(observationDigest, entity)
	ref := graphcontracts.Ref{Kind: graphcontracts.NodeOperations, ID: id}
	provenance := graphcontracts.Provenance{
		Producer: "eyrie",
		Version:  strings.TrimSpace(input.ProducerVersion),
		SourceID: observationDigest,
		Evidence: []graphcontracts.ArtifactRef{{URI: "eyrie://" + entity + "/" + observationDigest}},
	}
	node := graphcontracts.Node{
		ID: id, Kind: ref.Kind, Scope: input.Scope, CreatedAt: at,
		Provenance: provenance, Attributes: attrs,
	}
	if err := node.Validate(); err != nil {
		return graphcontracts.Ref{}, fmt.Errorf("operationsgraph: %s node: %w", entity, err)
	}
	event := graphcontracts.Event{
		ID:   "eyrie/observed/" + digest(id, at.Format(time.RFC3339Nano)),
		Type: graphcontracts.EventObserved, Subject: ref, Scope: input.Scope,
		OccurredAt: at, CorrelationID: strings.TrimSpace(input.CorrelationID),
		IdempotencyKey: digest(id, at.Format(time.RFC3339Nano)), Provenance: provenance,
	}
	out.Nodes = append(out.Nodes, node)
	out.Events = append(out.Events, event)
	return ref, nil
}

func digest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(strconv.Itoa(len(part))))
		_, _ = hash.Write([]byte{':'})
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
