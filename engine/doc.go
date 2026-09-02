// Package engine is the stable, host-facing Eyrie API.
//
// Hosts should prefer this package over assembling client, catalog, config,
// credentials, runtime, and setup packages directly. The lower-level packages
// remain public for backward compatibility and advanced integrations.
//
// Engine is intentionally stateless with respect to product conversations:
// the host owns conversation history, tools, permissions, and checkpoints;
// Eyrie owns credential, catalog, selection, routing, and model transport.
//
// Host-facing DTOs and the Provider port live in
// github.com/GrayCodeAI/eyrie/llm; this package re-exports them
// as type aliases and *Engine implements llm.Provider (see contract_assert.go).
package engine
