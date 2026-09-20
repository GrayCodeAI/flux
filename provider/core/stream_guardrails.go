package core

import (
	"log/slog"
	"strings"
	"sync"
)

// StreamGuardrailConfig controls how streaming guardrails behave.
type StreamGuardrailConfig struct {
	// Enabled turns streaming guardrails on or off.
	Enabled bool `json:"enabled"`
	// MaxChunkSize is the maximum allowed size of a single chunk (in bytes).
	// Chunks exceeding this limit are split. A value of 0 disables splitting.
	MaxChunkSize int `json:"max_chunk_size,omitempty"`
	// AccumulateForPII buffers chunks before running PII detection, since
	// patterns like SSNs or credit-card numbers may span chunk boundaries.
	AccumulateForPII bool `json:"accumulate_for_pii"`
	// BlockOnInjection causes an immediate block when a prompt-injection
	// pattern is detected in any chunk, without waiting for accumulation.
	BlockOnInjection bool `json:"block_on_injection"`
}

// StreamGuardrailResult is returned after processing a single chunk.
type StreamGuardrailResult struct {
	// Chunk is the original chunk text.
	Chunk string `json:"chunk"`
	// Blocked indicates that the chunk (or accumulated content) was blocked
	// by a guardrail with Action=Block.
	Blocked bool `json:"blocked"`
	// Violations lists any rules matched during this chunk's processing.
	Violations []GuardrailViolation `json:"violations,omitempty"`
	// ModifiedChunk is the chunk after redactions have been applied.
	// When no redactions are needed, it equals Chunk.
	ModifiedChunk string `json:"modified_chunk"`
}

// StreamGuardrails validates LLM output chunks incrementally as they arrive,
// rather than waiting for the full response. It is safe for concurrent use.
type StreamGuardrails struct {
	config     StreamGuardrailConfig
	guardrails *Guardrails
	buffer     strings.Builder
	violations []GuardrailViolation
	blocked    bool
	mu         sync.Mutex
}

// NewStreamGuardrails creates a StreamGuardrails with the given guardrail
// rules and configuration. guardrails must not be nil.
func NewStreamGuardrails(guardrails *Guardrails, config StreamGuardrailConfig) *StreamGuardrails {
	if guardrails == nil {
		slog.Error("NewStreamGuardrails guardrails must not be nil; returning nil")
		return nil
	}
	return &StreamGuardrails{
		config:     config,
		guardrails: guardrails,
	}
}

// ProcessChunk validates a single streaming chunk against the registered
// guardrails. If AccumulateForPII is enabled, the chunk is appended to an
// internal buffer and PII checks are deferred until Flush. If BlockOnInjection
// is enabled, prompt-injection rules are evaluated immediately on the chunk.
//
// The returned StreamGuardrailResult contains the (possibly redacted) chunk
// and any violations found.
func (sg *StreamGuardrails) ProcessChunk(chunk string) StreamGuardrailResult {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	if !sg.config.Enabled {
		return StreamGuardrailResult{
			Chunk:         chunk,
			ModifiedChunk: chunk,
		}
	}

	result := StreamGuardrailResult{
		Chunk:         chunk,
		ModifiedChunk: chunk,
	}

	// Always accumulate into the buffer so Flush can check cross-boundary patterns.
	sg.buffer.WriteString(chunk)

	// Check injection rules immediately if configured.
	if sg.config.BlockOnInjection {
		blockViolations := sg.checkRules(chunk, GuardrailPromptInjection)
		result.Violations = append(result.Violations, blockViolations...)
		for _, v := range blockViolations {
			if v.Rule.Action == GuardrailBlock {
				sg.blocked = true
				result.Blocked = true
			}
		}
	}

	// Check non-PII rules (secrets, harmful content, custom) immediately on the chunk.
	immediateViolations := sg.checkRulesExcluding(chunk, GuardrailPII, GuardrailPromptInjection)
	for _, v := range immediateViolations {
		if v.Rule.Action == GuardrailBlock {
			sg.blocked = true
			result.Blocked = true
		}
		if v.Rule.Action == GuardrailRedact {
			result.ModifiedChunk = strings.ReplaceAll(result.ModifiedChunk, v.MatchedText, v.RedactedResult)
		}
	}
	result.Violations = append(result.Violations, immediateViolations...)

	// If AccumulateForPII is off, also check PII on the chunk itself.
	if !sg.config.AccumulateForPII {
		piiViolations := sg.checkRules(chunk, GuardrailPII)
		for _, v := range piiViolations {
			if v.Rule.Action == GuardrailRedact {
				result.ModifiedChunk = strings.ReplaceAll(result.ModifiedChunk, v.MatchedText, v.RedactedResult)
			}
		}
		result.Violations = append(result.Violations, piiViolations...)
	}

	sg.violations = append(sg.violations, result.Violations...)
	return result
}

// Flush checks the accumulated buffer for violations that may span chunk
// boundaries (primarily PII patterns when AccumulateForPII is enabled).
// It returns all violations found during the entire stream session so far
// that were not already reported. Call Flush after the last chunk has been
// processed.
func (sg *StreamGuardrails) Flush() []GuardrailViolation {
	sg.mu.Lock()
	defer sg.mu.Unlock()

	if !sg.config.Enabled {
		return nil
	}

	accumulated := sg.buffer.String()

	// Run PII checks on the full accumulated buffer.
	piiViolations := sg.checkRules(accumulated, GuardrailPII)

	// Deduplicate violations already reported per-chunk.
	existing := make(map[string]bool, len(sg.violations))
	for _, v := range sg.violations {
		key := v.Rule.Name + ":" + v.MatchedText
		existing[key] = true
	}

	var newViolations []GuardrailViolation
	for _, v := range piiViolations {
		key := v.Rule.Name + ":" + v.MatchedText
		if existing[key] {
			continue
		}
		newViolations = append(newViolations, v)
		if v.Rule.Action == GuardrailBlock {
			sg.blocked = true
		}
	}

	sg.violations = append(sg.violations, newViolations...)
	return newViolations
}

// Reset clears the internal buffer and accumulated violations.
func (sg *StreamGuardrails) Reset() {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	sg.buffer.Reset()
	sg.violations = nil
	sg.blocked = false
}

// IsBlocked reports whether any guardrail with Action=Block has been triggered
// during the stream session.
func (sg *StreamGuardrails) IsBlocked() bool {
	sg.mu.Lock()
	defer sg.mu.Unlock()
	return sg.blocked
}

// checkRules runs all rules of the given type against the text and returns
// violations.
func (sg *StreamGuardrails) checkRules(text string, typ GuardrailType) []GuardrailViolation {
	var violations []GuardrailViolation
	for _, rule := range sg.guardrails.Rules() {
		if rule.Type != typ || rule.compiled == nil {
			continue
		}
		matches := rule.compiled.FindAllString(text, -1)
		for _, match := range matches {
			v := GuardrailViolation{
				Rule:        rule,
				MatchedText: match,
			}
			if rule.Action == GuardrailRedact {
				v.RedactedResult = strings.Repeat("*", len(match))
			}
			violations = append(violations, v)
		}
	}
	return violations
}

// checkRulesExcluding runs all rules whose type is NOT in the excluded set.
func (sg *StreamGuardrails) checkRulesExcluding(text string, exclude ...GuardrailType) []GuardrailViolation {
	excludeSet := make(map[GuardrailType]bool, len(exclude))
	for _, t := range exclude {
		excludeSet[t] = true
	}

	var violations []GuardrailViolation
	for _, rule := range sg.guardrails.Rules() {
		if excludeSet[rule.Type] || rule.compiled == nil {
			continue
		}
		matches := rule.compiled.FindAllString(text, -1)
		for _, match := range matches {
			v := GuardrailViolation{
				Rule:        rule,
				MatchedText: match,
			}
			if rule.Action == GuardrailRedact {
				v.RedactedResult = strings.Repeat("*", len(match))
			}
			violations = append(violations, v)
		}
	}
	return violations
}
