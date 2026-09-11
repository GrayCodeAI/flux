package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Relationship is a subject-predicate-object triple extracted from text. It is
// the unit of a knowledge graph: subject and object are entities (nouns), and
// predicate is the typed relation between them.
//
// This mirrors the typed-extraction pattern popularized by CocoIndex's
// ExtractByLlm(output_type=list[Relationship]) — instead of free-form text, the
// model is constrained to emit a list of these triples, validated against a JSON
// schema with automatic retry.
type Relationship struct {
	// Subject is the entity the relation originates from. A noun/noun phrase.
	Subject string `json:"subject"`
	// Predicate is the typed relation (e.g. "depends_on", "authored_by").
	Predicate string `json:"predicate"`
	// Object is the entity the relation points to. A noun/noun phrase.
	Object string `json:"object"`
}

// ExtractOptions configures triple extraction. The zero value is valid and uses
// sensible defaults (noun-constrained entities, 2 validation retries).
type ExtractOptions struct {
	// Chat carries provider/model/temperature for the extraction call. If Model
	// is empty the provider default is used.
	Chat ChatOptions
	// Instruction overrides the default extraction instruction. Use it to scope
	// what relations to extract (e.g. "extract only code dependency relations").
	// When empty, a general noun-constrained instruction is used.
	Instruction string
	// AllowedPredicates, when non-empty, constrains the predicate vocabulary —
	// the model is told to use only these relation types, and triples with other
	// predicates are dropped after extraction. This is the lightweight analogue
	// of CocoIndex's EntityTypeConfig schema constraint.
	AllowedPredicates []string
	// MaxRetries is the schema-validation retry budget. Defaults to 2.
	MaxRetries int
}

// relationshipSchema is the JSON schema for a list of Relationship triples,
// passed to ChatWithStructuredOutput for validation + retry.
func relationshipSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"relationships": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"subject":   map[string]interface{}{"type": "string"},
						"predicate": map[string]interface{}{"type": "string"},
						"object":    map[string]interface{}{"type": "string"},
					},
					"required": []interface{}{"subject", "predicate", "object"},
				},
			},
		},
		"required": []interface{}{"relationships"},
	}
}

const defaultExtractInstruction = `Extract the relationships expressed in the text as subject-predicate-object triples.

Rules:
- subject and object MUST be nouns or noun phrases naming concrete entities (people, systems, concepts) — never verbs, sentences, or pronouns.
- predicate is a short typed relation in snake_case (e.g. depends_on, authored_by, part_of).
- Extract only relationships actually stated or clearly implied; do not invent facts.
- Deduplicate: emit each distinct triple at most once.`

// ExtractRelationships extracts subject-predicate-object triples from text using
// schema-validated structured output with retry. It is a typed convenience layer
// over ChatWithStructuredOutput, modeled on CocoIndex's ExtractByLlm; Harrier and
// other knowledge-graph consumers can call it instead of hand-rolling extraction
// prompts and JSON parsing.
func (c *EyrieClient) ExtractRelationships(ctx context.Context, text string, opts ExtractOptions) ([]Relationship, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("eyrie: extract: text must not be empty")
	}

	instruction := opts.Instruction
	if instruction == "" {
		instruction = defaultExtractInstruction
	}
	if len(opts.AllowedPredicates) > 0 {
		instruction += "\n\nUse ONLY these predicates: " + strings.Join(opts.AllowedPredicates, ", ") + "."
	}

	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	messages := []EyrieMessage{
		{Role: "system", Content: instruction},
		{Role: "user", Content: text},
	}

	schema := relationshipSchema()
	resp, err := c.ChatWithStructuredOutput(ctx, messages, opts.Chat, SchemaValidation{
		Schema:     schema,
		MaxRetries: maxRetries,
	})
	if err != nil {
		return nil, fmt.Errorf("eyrie: extract relationships: %w", err)
	}

	var parsed struct {
		Relationships []Relationship `json:"relationships"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, fmt.Errorf("eyrie: extract relationships: decode: %w", err)
	}

	return filterRelationships(parsed.Relationships, opts.AllowedPredicates), nil
}

// filterRelationships drops empty triples and, when a predicate allowlist is
// given, any triple whose predicate is not in it. Predicate matching is
// case-insensitive to tolerate model casing drift.
func filterRelationships(rels []Relationship, allowed []string) []Relationship {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, p := range allowed {
		allowSet[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
	}

	out := make([]Relationship, 0, len(rels))
	for _, r := range rels {
		if strings.TrimSpace(r.Subject) == "" || strings.TrimSpace(r.Predicate) == "" || strings.TrimSpace(r.Object) == "" {
			continue
		}
		if len(allowSet) > 0 {
			if _, ok := allowSet[strings.ToLower(strings.TrimSpace(r.Predicate))]; !ok {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
