package client

// userMsg builds a single-user-message conversation. Shared by tests that
// previously relied on the helper defined in the (since-moved) embedding
// cache tests.
func userMsg(s string) []EyrieMessage {
	return []EyrieMessage{{Role: "user", Content: s}}
}
