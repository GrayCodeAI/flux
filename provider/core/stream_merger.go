package core

// StreamMerger is a schema-agnostic SSE delta merger. It accumulates streaming
// deltas into a single result map without knowing the provider's schema upfront.
//
// StreamFields are merged by string concatenation (e.g. "content", "arguments").
// IndexFields name the key within array elements that identifies their position
// (e.g. "index" in choices[N].delta), enabling correct out-of-order multi-choice
// reassembly: element A at index 2 is placed at slot 2 regardless of arrival order.
//
// Pattern ported from moonpalace/merge/merger.go (MIT).
type StreamMerger struct {
	StreamFields []string
	IndexFields  []string
	result       map[string]interface{}
}

// NewStreamMerger returns a StreamMerger with the given stream and index field names.
func NewStreamMerger(streamFields, indexFields []string) *StreamMerger {
	return &StreamMerger{
		StreamFields: streamFields,
		IndexFields:  indexFields,
		result:       make(map[string]interface{}),
	}
}

// DefaultStreamMerger returns a StreamMerger with Kimi/OpenAI defaults:
// StreamFields = ["content", "arguments"], IndexFields = ["index"].
func DefaultStreamMerger() *StreamMerger {
	return NewStreamMerger([]string{"content", "arguments"}, []string{"index"})
}

// Merge incorporates a parsed delta map into the accumulated result.
// It returns the updated accumulator (same map, mutated in place).
func (m *StreamMerger) Merge(delta map[string]interface{}) map[string]interface{} {
	m.mergeObject(m.result, delta)
	return m.result
}

// Result returns the current accumulated state.
func (m *StreamMerger) Result() map[string]interface{} {
	return m.result
}

func (m *StreamMerger) isStreamField(key string) bool {
	for _, f := range m.StreamFields {
		if f == key {
			return true
		}
	}
	return false
}

func (m *StreamMerger) mergeObject(prev, delta map[string]interface{}) {
	for k, v := range delta {
		existing, ok := prev[k]
		if !ok {
			prev[k] = v
			continue
		}
		switch dv := v.(type) {
		case string:
			if m.isStreamField(k) {
				if s, ok2 := existing.(string); ok2 {
					prev[k] = s + dv
				} else {
					prev[k] = dv
				}
			} else {
				prev[k] = dv
			}
		case map[string]interface{}:
			if em, ok2 := existing.(map[string]interface{}); ok2 {
				m.mergeObject(em, dv)
			} else {
				prev[k] = dv
			}
		case []interface{}:
			if ea, ok2 := existing.([]interface{}); ok2 {
				prev[k] = m.mergeArray(ea, dv)
			} else {
				prev[k] = dv
			}
		default:
			prev[k] = v
		}
	}
}

// mergeArray places each incoming element at its declared index position,
// growing the accumulator slice as needed.
func (m *StreamMerger) mergeArray(prev, delta []interface{}) []interface{} {
	for _, elem := range delta {
		em, ok := elem.(map[string]interface{})
		if !ok {
			prev = append(prev, elem)
			continue
		}
		idx := m.findIndex(em)
		if idx < 0 {
			prev = append(prev, em)
			continue
		}
		// Grow the accumulator to accommodate this index.
		for len(prev) <= idx {
			prev = append(prev, map[string]interface{}{})
		}
		if existing, ok2 := prev[idx].(map[string]interface{}); ok2 {
			m.mergeObject(existing, em)
		} else {
			prev[idx] = em
		}
	}
	return prev
}

// findIndex returns the first index-field value found in the element map,
// or -1 if none is present or the value is not a non-negative integer.
func (m *StreamMerger) findIndex(elem map[string]interface{}) int {
	for _, f := range m.IndexFields {
		v, ok := elem[f]
		if !ok {
			continue
		}
		switch iv := v.(type) {
		case float64:
			if iv >= 0 {
				return int(iv)
			}
		case int:
			if iv >= 0 {
				return iv
			}
		}
	}
	return -1
}
