package graph_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GrayCodeAI/eyrie/graph"
)

// TestSchemaParity pins the wire schema of graph.Node to the exact JSON the
// eagle graph contract produces, so a schema drift between eagle and eyrie is
// caught at build time.
func TestSchemaParity(t *testing.T) {
	n := graph.Node{
		ID:        "n1",
		Kind:      graph.NodeOperations,
		Scope:     graph.Scope{RepositoryID: "repo"},
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Provenance: graph.Provenance{
			Producer: "test",
		},
	}

	if err := n.Validate(); err != nil {
		t.Fatalf("fixture must validate: %v", err)
	}

	got, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	want := `{"id":"n1","kind":"operations","scope":{"repository_id":"repo"},"created_at":"2024-01-01T00:00:00Z","effective_at":"0001-01-01T00:00:00Z","provenance":{"producer":"test"}}`
	if string(got) != want {
		t.Fatalf("schema parity mismatch\n got: %s\nwant: %s", got, want)
	}
}
