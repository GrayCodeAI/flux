// Package engine is the stable, host-facing Flux API.
//
// Hosts should prefer this package over assembling provider, catalog, config,
// credentials, runtime, and setup packages directly. The lower-level packages
// remain public for backward compatibility and advanced integrations.
//
// A fixed set of engine-internal symbols is reachable from this package (for
// example Options.SecretStore is a credentials.Store and OperationsGraphInput
// aliases operationsgraph.Input). Those symbols are frozen as part of the
// contract; docs/architecture/HOST-ENGINE-BOUNDARY.md lists them and
// host_surface_test.go fails when the reachable set changes.
//
// Engine is intentionally stateless with respect to product conversations:
// the host owns conversation history, tools, permissions, and checkpoints;
// Flux owns credential, catalog, selection, routing, and model transport.
//
// Host-facing DTOs and the Provider port live in
// github.com/GrayCodeAI/flux/llm; this package re-exports them
// as type aliases and *Engine implements llm.Provider (see contract_assert.go).
package engine
