package client

import "github.com/GrayCodeAI/flux/client/core"

// SanitizeMessages inspects messages for orphaned tool_use blocks
// and injects synthetic error results. Implementation lives in client/core.
func SanitizeMessages(messages []FluxMessage) []FluxMessage {
	return core.SanitizeMessages(messages)
}
