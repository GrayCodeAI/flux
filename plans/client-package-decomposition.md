# Feature Specification: `client` Package Decomposition

**Status:** Historical plan, superseded by
[`docs/architecture/FEATURE-MONOREPO.md`](../docs/architecture/FEATURE-MONOREPO.md).
The provider rename and feature-package split are implemented on the current
branch. References below to `RegisterDynamicProvider` and alias-based
compatibility describe the old design, not the current API.
**Author:** Claude (architecture review session)
**Date:** 2026-07-12
**Repos affected:** flux (all changes), rho (no code changes required; update
the published Flux module pin)

## Problem Statement

`flux/provider` is a 63-file, ~14k-line (source, excluding tests) single package that
mixes at least six distinct concerns:

1. **Core contract & types** — `Provider` interface, `FluxMessage`, `FluxResponse`,
   `FluxStreamEvent`, `StreamResult`, `ChatOptions`, `FluxClient` (`provider.go`,
   `options.go`, `chat.go`, `errors.go`, `retry.go`, `transport.go`, `stream.go`,
   `continuation.go`, `roles.go`, `merge.go`, `extract.go`)
2. **Protocol adapters** — `anthropic.go`, `openai.go`, `gemini.go`, `azure.go`,
   `bedrock.go`, `vertex.go`, `deepseek.go`, `zai.go`, `mimo.go`, `opencodego.go`,
   `dynamic.go`, `compat.go`, `protocol_router.go`, `provider_registry.go`
3. **Middleware decorators** (all wrap `Provider`) — `adaptive_ratelimit.go`,
   `budget_provider.go`, `callbacks.go`, `condenser.go`, `fallback.go`,
   `guardrails.go`, `stream_guardrails.go`, `lazy_provider.go`, `ratelimit.go`,
   `weighted.go`, `usage_limit.go`, `repeat_detector.go`, `response_health.go`,
   `coalesce.go`, `tracing.go`, `recorder.go`, `cassette.go`, `mock.go`
4. **Caching** — `cache.go`, `semantic_cache.go`, `cache_analytics.go`
5. **Embeddings** — `embedding.go`, `embedding_cache.go`, `embedding_client.go`,
   `embedding_defaults.go`
6. **Auxiliary capabilities** — `batch.go`, `image.go`, `moderation.go`,
   `structured.go`, `features.go`, `token_utils.go`, `cost_estimator.go`,
   `usage_tracker.go`, `call_metrics.go`, `sanitize.go`, `provider_health.go`

Consequences: no compiler-enforced boundaries (an adapter can reach into cache
internals), slow test cycles (one package = one test binary), unclear ownership,
and a public API surface far larger than what consumers use.

### Measured coupling (2026-07-12)

- Embeddings cluster uses only **4** unexported helpers from the rest of the
  package: `copyResponse`, `doWithRetry`, `formatAPIError`, `parseProviderError`.
- Adapter cluster uses **13**: the above plus `applyGuardrails`,
  `buildAnthropicCachedRequest`, `defaultTimeout`, `emit`, `openAIImageURL`,
  `parseImageString`, `parseSSEStream`, `processAnthropicStream`,
  `processOpenAIStream`, `userAgent`.
- rho (the primary consumer) accesses `flux/provider` from **4 files only** —
  it maintains its own DTO layer (`rho/internal/types/provider.go`) and converts
  at the boundary. Entry points consumed: `Client`, `FluxClient` methods
  (`Chat`, `StreamChat`, `StreamChatContinue`, `SetAPIKey`, `Ping`,
  `GetProviders`), `StreamChatWithContinuation`, `ParseInlineToolCalls`,
  `DetectProvider`, `RegisterDynamicProvider`, `DefaultContinuationConfig`,
  `NewMockProvider`/`MockModeFixed`, `Provider`, and the DTO types
  (`FluxMessage`, `FluxResponse`, `FluxStreamEvent`, `FluxUsage`,
  `StreamResult`, `ChatOptions`, `ContinuationConfig`, `ContentPart`,
  `ImageURLPart`, `InputAudioPart`, `ToolCall`, `ToolResult`, `FluxTool`,
  `FluxConfig`, `ResponseFormat`, `ToolChoiceOption`).

The narrow consumed surface makes a **non-breaking, alias-based decomposition**
practical.

## Proposed Solution

Extract a leaf `core` package holding the contract and wire-level plumbing, then
move each concern into its own subpackage that imports `core`. The `client`
package remains as a compatibility facade: every moved type becomes a Go type
alias (`type FluxMessage = core.FluxMessage`), every moved function a thin
wrapper or function variable. Type aliases preserve type identity, so **no
consumer code changes and no version bump semantics change**.

Target layout:

```
flux/
  provider/            // facade: aliases + wrappers (shrinks each phase)
    core/            // Provider, messages, options, errors, retry, transport, SSE
    adapters/        // one file per protocol family; imports core only
    middleware/      // decorators over core.Provider
    llmcache/        // response + semantic caches
    embeddings/      // embedding client + cache + defaults
    aux/             // batch, image, moderation, structured, tokens, cost
```

Dependency rule (CI-enforced): `core` imports none of the siblings;
`adapters`/`middleware`/`llmcache`/`embeddings`/`aux` import `core` only;
the `client` facade imports all of them; nothing imports the facade from
inside the tree.

## Alternatives Considered

- **Big-bang rename (`provider/v2`)** — breaks every consumer including examples
  and SDK bindings; rejected.
- **Move whole package to `internal/`** — rho and examples import it; rejected.
- **Split without a core package** (e.g., extract embeddings directly) — impossible
  without import cycles: subpackages need client types while the facade re-exports
  subpackage API. The core extraction is the unlock; everything else follows.
- **Do nothing, document layers in comments** — no compiler enforcement; the
  package grew to 63 files precisely because nothing pushes back.

## Implementation Plan

### Phase 1: core extraction (the unlock)
Phase 1 is DONE (2026-07-12):
- [x] Created `provider/core` with `Provider`, message/response/stream/usage/tool
      types, `ChatOptions`, `ResponseFormat`, `ToolChoiceOption`,
      `ContinuationConfig`, `FluxConfig`, `FluxError`, `RetryConfig` +
      `DoWithRetry`, `ParseProviderError`/`FormatAPIError`, `CopyResponse`.
      (SSE parsing, transport, `userAgent`, `defaultTimeout` deferred to the
      adapters phase — embeddings did not need them.)
- [x] Internal call sites use explicit `provider/core` imports; production
      aliases are not retained.
- [x] `go test ./...` green in flux; rho builds + tests green.

### Phase 2: embeddings (smallest proven cluster, 4 deps)

Phase 2 is DONE (2026-07-12):
- [x] Moved embedding DTOs, `Embedder`, defaults, and `EmbeddingCachedProvider`
      to `provider/embeddings` (imports `core` only).
- [x] `OpenAIClient.CreateEmbedding` / `FluxClient.CreateEmbedding` stayed in
      `client` (`embedding_methods.go`) — methods must live with their
      receiver's package; they implement `embeddings.Embedder`.
- [x] Embedding API is owned by `provider/embeddings`; the composition root
      depends on it explicitly.
- [x] Layering guard live early: `scripts/check-provider-layering.sh`, wired
      into `make boundaries`.

Learned in Phases 1–2 (apply to later phases):
- BSD sed has no `\b`; use perl for identifier renames.
- Unexported fields (`StreamResult.cancel`) force constructor use at move
  time — added `core.NewStreamResultWithRequestID`.
- Tests that exercise facade types (e.g. `NewMockProvider`) cannot move with
  the cluster; give the subpackage a local test double instead.

### Phase 3a: wire layer to core — DONE 2026-07-12
- [x] Moved to `provider/core` with exported names:
      `stream.go` (`ParseSSEStream`, `ProcessAnthropicStream[WithOpts]`,
      `ProcessOpenAIStream[WithOpts]`, `Emit`, `ParseInlineToolCalls`,
      `StreamChannelBuffer`), `transport.go` (`NewPooledHTTPClient`,
      `CloseIdleConnections`, `DefaultTimeout`, `Version`/`SetVersion`/
      `UserAgent`), `repeat_detector.go`, `image.go` (`OpenAIImageURL`,
      `ParseImageString`, `NormalizeImageSource`), `response_health.go`
      (`DetectResponseHealth`, `ResponseHasContent`, health constants).
      Seven wire-layer test files moved with them.
- [x] `provider.SetVersion` forwards to `core.SetVersion` from root package
      wiring.

### Phase 3b-i: options decoupled from adapter types — DONE 2026-07-12
- [x] `ClientOption` no longer holds `applyFn func(*AnthropicClient)` /
      `applyOpenAIFn func(*OpenAIClient)`. It applies through the unexported
      `clientConfigurable` interface (exported `Set*` methods, implemented by
      both adapters in `adapter_config.go`). All `With*` constructors,
      `WithProviderName`/`WithMimoAuth` (mimo.go), and
      `WithStructuredOutput` (structured.go) rewritten; behavior identical.

### Phase 3b-ii: guardrails engine to core — DONE 2026-07-12
- [x] Guardrail engine (rule types/constants, `Guardrails`, `Check`,
      `ApplyRedactions`, default rule sets, `ApplyGuardrails`, and the
      incremental `StreamGuardrails` scanner) moved to `core`; the
      `GuardrailProvider` middleware wrapper stays in the facade
      (`provider/guardrails.go`). Full public API aliased.

### Phase 3b-iii: adapter file move — DONE 2026-07-13
- [x] Moved provider protocol implementations and construction helpers to
      `provider/adapters`; the package imports `provider/core` only.
- [x] Removed the old `client` path and the production alias facade. Consumers
      import `provider/core`, `provider/adapters`, or the relevant feature
      package directly.
- [x] Kept embedding DTO ownership in `provider/core` where adapters need it;
      `provider/embeddings` remains a sibling that imports only core.
- [x] Moved Anthropic cache request construction, protocol routing, dynamic
      provider registration, and provider construction into the adapter layer.
- [x] Updated tests to exercise the adapter package directly where internals
      are required while retaining facade compatibility coverage.

### Phase 4: middleware, cache, aux
- [x] Feature packages now own resilience, cache, media, batch, and
      observability domains.

### Phase 5: enforcement + deprecation
- [x] Add `scripts/check-provider-layering.sh` (mirror of rho's
      `check-flux-client-imports.sh`) to CI: fail on any sibling→sibling import
      that bypasses `core`, and on any in-tree import of the facade.
- [x] Removed facade aliases and migrated flux-internal callers
      (`conversation`, `router`, `runtime`, `setup`,
      examples) to the subpackages; external callers must migrate explicitly.

## Testing Strategy

- Unit tests: move with their files; each phase must keep `go test ./...` green.
- Integration tests: `catalogtest` + rho `internal/engine` suite against the
  branch via `go.work` replace.
- E2E tests: `rho path` smoke + one live streamed chat per protocol family
  (anthropic-messages, openai-chat-completions, gemini-generate-content) before
  each merge.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Hidden unexported coupling beyond the measured sets | med | Phases are one-cluster-at-a-time; the compiler finds every missed reference at move time; abort/expand `core` rather than weaken boundaries |
| Type identity breakage for consumers doing type switches | high | Keep one canonical type in `provider/core`; make breaking ownership changes explicit |
| Method sets split from their types | high | Methods move with their receiver's file into the same subpackage — never leave methods behind |
| Flux module-pin drift in Rho during the refactor | low | Land phases as individual PRs; update Rho after each; `make sync` reports drift |
| A new feature leaks into the composition root | low | Phase 5 CI boundary check and package ownership review |

## References

- rho's boundary script: `rho/scripts/check-flux-client-imports.sh`
- rho's DTO layer (proof the consumer surface is narrow): `rho/internal/types/provider.go`
- Session decomposition precedent: `rho/docs/session-decomposition.md`
