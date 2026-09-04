package client

import "github.com/GrayCodeAI/graycode-router/client/core"

// SanitizeMessages inspects messages for orphaned tool_use blocks
// and injects synthetic error results. Implementation lives in client/core.
func SanitizeMessages(messages []GraycodeRouterMessage) []GraycodeRouterMessage {
	return core.SanitizeMessages(messages)
}
