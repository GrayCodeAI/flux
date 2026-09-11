package client

import (
	"context"
	"testing"
)

func TestFilterRelationships_DropsEmpty(t *testing.T) {
	t.Parallel()
	in := []Relationship{
		{Subject: "eyrie", Predicate: "part_of", Object: "hawk-eco"},
		{Subject: "", Predicate: "x", Object: "y"},   // empty subject
		{Subject: "a", Predicate: "  ", Object: "b"}, // blank predicate
		{Subject: "c", Predicate: "rel", Object: ""}, // empty object
	}
	out := filterRelationships(in, nil)
	if len(out) != 1 || out[0].Subject != "eyrie" {
		t.Fatalf("expected only the complete triple, got %#v", out)
	}
}

func TestFilterRelationships_PredicateAllowlist(t *testing.T) {
	t.Parallel()
	in := []Relationship{
		{Subject: "a", Predicate: "depends_on", Object: "b"},
		{Subject: "a", Predicate: "Depends_On", Object: "c"},  // case-insensitive match
		{Subject: "a", Predicate: "authored_by", Object: "d"}, // not allowed
	}
	out := filterRelationships(in, []string{"depends_on"})
	if len(out) != 2 {
		t.Fatalf("expected 2 depends_on triples (case-insensitive), got %d: %#v", len(out), out)
	}
}

func TestExtractRelationships_EmptyText(t *testing.T) {
	t.Parallel()
	c := &EyrieClient{}
	_, err := c.ExtractRelationships(context.Background(), "   ", ExtractOptions{})
	if err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestRelationshipSchema_Shape(t *testing.T) {
	t.Parallel()
	s := relationshipSchema()
	props, ok := s["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing properties")
	}
	rels, ok := props["relationships"].(map[string]interface{})
	if !ok || rels["type"] != "array" {
		t.Fatalf("relationships should be an array, got %#v", rels)
	}
	items, ok := rels["items"].(map[string]interface{})
	if !ok {
		t.Fatal("array missing items schema")
	}
	req, ok := items["required"].([]interface{})
	if !ok || len(req) != 3 {
		t.Fatalf("each triple should require subject/predicate/object, got %#v", req)
	}
}
