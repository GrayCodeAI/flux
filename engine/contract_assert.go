package engine

import "github.com/GrayCodeAI/graycode-router/llm"

// Compile-time assertions: the host facade implements the shared port.
// If a method is added to llm.Provider (or EventStreamer), this file fails
// to compile until Engine/Stream grow matching methods — catching drift early.
var (
	_ llm.Provider      = (*Engine)(nil)
	_ llm.EventStreamer = (*Stream)(nil)
	_ llm.Generator     = (*Engine)(nil)
)
