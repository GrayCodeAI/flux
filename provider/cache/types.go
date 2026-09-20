package cache

import "github.com/GrayCodeAI/flux/provider/core"

// Cache decorators use the shared provider contract; cache has no dependency
// on the provider composition root.
type (
	Provider     = core.Provider
	FluxMessage  = core.FluxMessage
	FluxResponse = core.FluxResponse
	ChatOptions  = core.ChatOptions
	StreamResult = core.StreamResult
)
