package core

// CopyResponse returns a deep copy of an GraycodeRouterResponse so that callers
// cannot mutate the cached version.
func CopyResponse(resp *GraycodeRouterResponse) *GraycodeRouterResponse {
	if resp == nil {
		return nil
	}
	cp := *resp
	if resp.Usage != nil {
		u := *resp.Usage
		cp.Usage = &u
	}
	if len(resp.ToolCalls) > 0 {
		cp.ToolCalls = make([]ToolCall, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			cp.ToolCalls[i] = tc
			if tc.Arguments != nil {
				cp.ToolCalls[i].Arguments = deepCopyMap(tc.Arguments)
			}
		}
	}
	return &cp
}

// deepCopyMap returns a deep copy of a map[string]interface{}.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			cp[k] = deepCopyMap(val)
		case []interface{}:
			cp[k] = deepCopySlice(val)
		default:
			cp[k] = v
		}
	}
	return cp
}

// deepCopySlice returns a deep copy of a []interface{}.
func deepCopySlice(s []interface{}) []interface{} {
	cp := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			cp[i] = deepCopyMap(val)
		case []interface{}:
			cp[i] = deepCopySlice(val)
		default:
			cp[i] = v
		}
	}
	return cp
}
