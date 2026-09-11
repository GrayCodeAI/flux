package client

import "github.com/GrayCodeAI/eyrie/client/core"

// SanitizeMessages inspects messages for orphaned tool_use blocks
// and injects synthetic error results. Implementation lives in client/core.
func SanitizeMessages(messages []EyrieMessage) []EyrieMessage {
	return core.SanitizeMessages(messages)
}
