# Flux OSS landscape roadmap: code architecture audit

**Audit date:** 2026-09-24
**Checkout:** `github.com/GrayCodeAI/flux` at `f9037a8` on `chore/oss-competitive-roadmap`
**Scope:** local source, tests, CI, build tags, SDKs, examples, and architecture documentation. No competitor benchmark was performed.
**Evidence labels:** **Confirmed defect** means the behavior follows directly from current source; **Design opportunity** means the code is intentional or plausible but leaves a contract/operational gap; **Uncertainty** means the source establishes a risk but product intent needs confirmation.

## Cited Findings

### Takeaway

Flux has a strong host-neutral foundation: canonical DTOs, a narrow engine facade, feature-oriented provider packages, validated deployment routing, atomic live snapshots, signed control-plane manifests, and a broad test/CI baseline. The largest risks are not missing packages; they are **contract features that are declared ahead of their implementations** and **middleware that changes stream/usage behavior after the provider boundary**.

The highest-value work is to close the canonical request/stream contract first: restore `ThinkingEnabled`, define one usage event policy, preserve provider blocks and route attempts, and make every stream decorator preserve cancellation. Then harden migration, budget, cache, and proxy semantics. The existing green test suite does not exercise these cross-layer cases.

### Architectural map

| Area | Responsibility | Evidence |
|---|---|---|
| Host contract | `engine`, `llm`, `graph`, and `tools` are the accepted host-facing packages; providers own credentials, catalog, routing, transports, resilience, usage, and provider telemetry. | `README.md:46-65`; `docs/architecture/HOST-ENGINE-BOUNDARY.md:6-36`; `docs/architecture/FEATURE-MONOREPO.md:11-35` |
| Engine composition | `engine.New` accepts host-owned secret store and paths, snapshots custom gateways, and composes state, catalog, deployment transport, rate limiting, and caching. | `engine/engine.go:22-117`; `engine/engine.go:303-326` |
| Canonical DTOs | `provider/core` aliases `llm` types rather than maintaining parallel wire definitions. | `provider/core/core.go:35-97`; `llm/types.go:49-75`; `engine/types.go:155-164` |
| Provider runtime | `provider` composes adapters and feature packages; `provider/core` owns neutral stream and wire primitives. | `provider/client.go`; `provider/core/core.go:1-12`; `provider/core/stream.go:31-89` |
| Catalog and control plane | Catalog compilation, deployment policy, live replacement, and signed credential-free manifests are separated. | `catalog/v1.go:584-613`; `router/live_deployment_router.go:12-59`; `router/controlplane/snapshot.go:68-140`; `router/controlplane/peers.go:65-110` |
| Delivery | HTTP and optional gRPC are delivery adapters over the conversation/provider runtime. | `internal/api/server.go:101-121`; `internal/grpc/grpc.go:1-8`; `internal/grpc/server_grpc.go:1-78` |
| Persistence and usage | SQLite conversation DAG, budget store, provider observability, and a separate internal telemetry package coexist. | `storage/sqlite.go:39-66`; `storage/budgets.go:35-68`; `provider/observability/usage_limit.go:9-35`; `internal/observability/observability.go:1-8` |

### Strengths confirmed

- **Host boundary is explicit and compile-checked.** `engine/contract_assert.go` asserts the engine and stream implement the `llm` ports; `engine/types.go:142-146` re-exports the provider/stream contract. The accepted boundary document explicitly says Rho owns UX, sessions, tools, permissions, and agent loops (`docs/architecture/HOST-ENGINE-BOUNDARY.md:8-12`, `:137-154`).
- **DTO ownership is centralized.** `llm/types.go:1-10` calls itself the canonical contract, and `provider/core/core.go:35-86` aliases those types. This avoids the older provider/client DTO duplication described in the migration plan.
- **Provider layering is mechanically checked.** `scripts/check-provider-layering.sh:10-24` prevents `provider/core` and feature packages from importing siblings or the provider facade. The ecosystem script rejects Rho imports (`scripts/check-ecosystem-boundaries.sh:7-23`).
- **Deployment routing validates more than the legacy router.** `NewDeploymentRouter` rejects missing catalog, empty deployments, nil providers, and inconsistent deployment IDs (`router/deployment_router.go:74-112`). It filters by tools and circuit-breaker state (`:365-389`).
- **Live routing is atomic and last-good-safe.** `LiveDeploymentRouter.Replace` validates a complete replacement before publishing an atomic snapshot (`router/live_deployment_router.go:41-59`); control-plane replicas reject stale/conflicting revisions and retain the prior route on failure (`router/controlplane/snapshot.go:68-140`).
- **Control-plane trust is materially stronger than ordinary catalog refresh.** Remote peers require HTTPS and pinned Ed25519 keys, cap manifests at 16 MiB, reject redirects, and reject disagreement at the newest revision (`router/controlplane/peers.go:65-110`, `:113-169`).
- **Provider state persistence is defensive.** `SaveProviderConfig` rejects credential fields, writes a restrictive temporary file, fsyncs, and atomically renames (`config/provider_env.go:490-548`); credential migration support has a broad table of recognized fields (`config/provider_secrets.go:22-146`).
- **Transport streams are genuinely streaming-first.** The SSE parser uses a bounded scanner and cancellation-aware forwarding (`provider/core/stream.go:31-89`); Anthropic and OpenAI processors emit content, reasoning, tool, usage, TTFT, and terminal events rather than buffering complete responses (`:110-291`, `:396-588`).
- **Testing and CI are broad and currently green.** See the verification record under Gaps. The repository has extensive adapter, routing, storage, API, and contract tests; `go test -race`, vet, lint, formatting, vulnerability scanning, and boundary checks all pass.

### Confirmed defects and contract gaps

#### F1. Host `ThinkingEnabled` is silently dropped

**Classification:** Confirmed defect, P1.

`engine.toClientOptions` maps `GLMThinkingEnabled` but never maps `advanced.ThinkingEnabled` (`engine/convert.go:15-34`). The field is part of both the host request contract and client options (`llm/provider.go:104-113`; `llm/types.go:135-155`). Provider adapters do consume it: Anthropic gives it precedence in `resolveThinking` (`provider/adapters/anthropic.go:177-181`), and OpenAI-compatible translation uses it as the canonical toggle (`provider/adapters/openai.go:409-411`).

The result is host/provider behavior divergence: a host can request reasoning enabled through the new field, the engine drops it, and the adapter falls back to deprecated or budget-based behavior. Existing `engine/convert_test.go` cases do not assert `ThinkingEnabled`.

**Remediation:** map `opts.ThinkingEnabled = advanced.ThinkingEnabled`, retain the deprecated alias only as an explicit fallback policy, and add adapter-to-engine contract tests for enabled, disabled, and nil cases.

#### F2. The stream contract declares provider metadata that the engine drops

**Classification:** Confirmed defect, P1.

`llm.FluxStreamEvent` carries `ProviderBlock` and `Route` (`llm/types.go:265-287`), and the stable event vocabulary includes `provider_block` (`engine/types.go:65-84`). However, `engine.normalizeEvent` copies content, thinking, request ID, usage, stop reason, and TTFT, then maps only selected event types; it never copies `event.ProviderBlock` or `event.Route` (`engine/stream.go:124-165`). The initial synthetic `route_selected` event is present (`engine/stream.go:82-86`), but provider event route metadata is lost.

`StreamErrorInfo` is declared (`llm/types.go:90-98`) but is not a field on `FluxStreamEvent`; parser errors are emitted as only `Type:"error", Error:string` (`provider/core/stream.go:283-285`, `:485-488`). `FluxResponse.Warnings` is also declared (`llm/types.go:248-263`) but no provider adapter populates it.

**Remediation:** either remove the unimplemented fields from the stable contract or make them mandatory end-to-end: parse and preserve provider blocks, attach route/attempt metadata to terminal events, and add a structured error field or a documented error-classification path. Add black-box tests that feed a provider block through adapter → core → engine.

#### F3. Provider-native reasoning blocks cannot round-trip

**Classification:** Confirmed defect, P1.

The canonical message/response contract explicitly promises opaque provider state for Anthropic thinking signatures, redacted thinking, Gemini thought state, and OpenAI reasoning items (`llm/types.go:58-75`, `:248-260`). The Anthropic wire response type captures `thinking` and `signature`, but the parser concatenates only thinking text and silently skips redacted blocks (`provider/adapters/anthropic.go:233-300`). `buildAnthropicMessages` handles tool use, tool results, content parts, and images, but has no branch that emits `m.Thinking` or `m.ProviderBlocks` (`:303-429`). Repository-wide adapter search found no `ProviderBlock` or `ProviderBlocks` construction.

This means a host cannot safely continue a signed-thinking conversation even though the DTO comments instruct it to store and replay those blocks. The API currently loses protocol state rather than exposing a clear unsupported warning.

**Remediation:** define the provider-block wire envelope per adapter, preserve the raw block exactly, filter by protocol on replay, and test Anthropic signed/redacted thinking and Gemini/OpenAI reasoning metadata across two turns.

#### F4. Real streaming usage is ignored by conversation persistence and the OpenAI proxy

**Classification:** Confirmed defect, P1.

Anthropic emits input usage on `message_start` and output usage on `message_delta` as separate `usage` events (`provider/core/stream.go:236-281`). OpenAI emits a `usage` event before the terminal event when `stream_options.include_usage` is present (`provider/core/stream.go:502-512`). Gemini attaches usage to its terminal `done` event (`provider/adapters/gemini.go:625-644`).

`conversation.streamAndSave` records usage only in the `case "done"` branch (`conversation/engine.go:272-290`). It therefore persists zero tokens for Anthropic and OpenAI streaming calls. The OpenAI proxy reads usage back from the persisted node (`internal/api/openai_proxy.go:267-283`), so its compatibility response can also report zero usage.

The current mocks hide the defect: `conversation/engine_test.go:23-28` and `internal/api/server_test.go:26-31` put usage directly on `done`, unlike the real Anthropic/OpenAI processors. The Anthropic adapter test also asserts content only and does not assert usage (`provider/adapters/anthropic_test.go:98-125`).

**Remediation:** define one canonical usage event contract (prefer a final aggregate event), have every adapter conform, aggregate split usage events in the conversation layer, and add per-adapter tests that assert persisted tokens and proxy usage. Do not infer “usage on done” from mocks.

#### F5. Stream decorators drop the upstream cancellation function

**Classification:** Confirmed defect, P1.

`llm.StreamResult.Close` is the contract cleanup mechanism (`llm/types.go:289-307`). `TracingProvider` wraps the event channel in a goroutine but returns a struct literal with no cancellation callback (`provider/observability/tracing.go:91-135`). `GuardrailProvider` does the same (`provider/resilience/guardrails.go:83-118`). A caller that closes the returned result cannot cancel the underlying provider stream; the forwarding goroutine can remain blocked until the provider completes.

The same code paths do forward on context cancellation (`tracing.go:120-128`, `guardrails.go:106-111`), but the public `Close` path cannot trigger that context. `UsageLimitProvider` and `BudgetProvider` demonstrate the safer pattern by returning `NewStreamResult(wrappedCh, result.Close)` (`provider/observability/usage_limit.go:83-104`; `provider/observability/budget_provider.go:112-133`).

**Remediation:** return a constructor that preserves the original `Close` callback, and add a test that closes before the producer finishes and asserts the provider context/body is canceled.

#### F6. Streaming guardrails do not provide the cross-chunk behavior already implemented elsewhere

**Classification:** Confirmed defect/contract gap, P1/P2.

The streaming decorator checks each content chunk and applies only redactions (`provider/resilience/guardrails.go:70-105`). A block rule does return an error from `Guardrails.Check` and is converted to a terminal error (`provider/core/guardrails.go:152-201`), so blocking itself is not wholly absent. However:

- PII patterns split across chunks are not accumulated.
- `Warn` violations are not logged or emitted despite the documented behavior (`guardrails.go:14-18`).
- The richer `core.StreamGuardrails` implementation explicitly supports buffering, injection blocking, and final PII checks (`provider/core/stream_guardrails.go:62-166`) but is not wired into the provider decorator.

**Remediation:** make `GuardrailProvider.StreamChat` use `StreamGuardrails`, define whether a final retrospective block is possible after bytes have already been delivered, and add chunk-boundary tests for PII, secrets, and warning events.

#### F7. The response cache is exact, incomplete-key, and not safely deep-copied

**Classification:** Confirmed defect, P1/P2.

The engine describes caching as semantic and deterministic by default (`engine/engine.go:45-51`; `README.md:136-140`), but the implementation is a deterministic hash/LRU cache (`provider/cache/semantic_cache.go:52-75`, `:105-138`). The key includes model, system, temperature, role/content, tool calls, and tool results (`:296-339`), but omits response-affecting fields such as `ContentParts`, images, thinking, provider blocks, `MaxTokens`, `TopP`, `TopK`, stop sequences, tool choice, response format, reasoning controls, provider identity, and metadata. Two materially different requests can therefore share a cached response.

There are two additional correctness/configuration gaps:

- `CacheConfig.Enabled` is documented as defaulting to true (`:15-28`, `:31-38`), but `NewCachedProvider` copies the zero value `false` and does not apply that default (`:73-92`). `Engine.Options.EnableCaching` with a zero `CacheConfig` therefore silently disables the cache.
- `core.CopyResponse` deep-copies usage and tool arguments but leaves response slices such as `ProviderBlocks` and `Warnings` aliased (`provider/core/copy.go:3-23`). A caller can mutate cached metadata.

A separate unused `internal/cache` package implements another in-memory/Redis backend and a hard-coded Anthropic cache warmer (`internal/cache/backend.go:1-9`, `:97-121`; `internal/cache/cache_warmer.go:10-31`), increasing architectural ambiguity.

**Remediation:** rename the feature to exact response cache unless true similarity is implemented; version and hash all response-affecting canonical fields; normalize zero config through `DefaultCacheConfig`; deep-copy every response slice/raw block; and either integrate or remove the internal cache subsystem.

#### F8. Deployment response provenance is not populated

**Classification:** Confirmed defect, P1/P2.

`ResolvedRoute` documents `DeploymentID` as the backend that actually served the request and `Attempts` as the number of attempts (`llm/types.go:235-246`). `DeploymentRouter` tracks the selected deployment internally and returns the adapter response unchanged (`router/deployment_router.go:134-180`, `:432-439`); it does not assign `resp.Route`. The engine then overwrites any response route with the route selected before the call (`engine/convert.go:88-96`). A repository-wide search found no production assignment to `DeploymentID` or `Attempts` outside tests.

The result is correct route selection but incomplete operational attribution, especially for failover, usage pricing, and the operations graph projection.

**Remediation:** attach a resolved route at the deployment boundary after success, including actual deployment and attempt count; preserve it through engine conversion and stream terminal events; add a failover test asserting provider `A` attempt 1 and provider `B` attempt 2 are reported distinctly.

#### F9. Budget enforcement is not atomic across check and record

**Classification:** Confirmed defect, P1.

`BudgetProvider.Chat` and `StreamChat` call `CheckBudget`, invoke the provider, and later call `RecordUsage` (`provider/observability/budget_provider.go:73-91`, `:94-133`). The SQLite store’s `CheckBudget` is a read and `RecordUsage` is a later transaction (`storage/budgets.go:157-208`). Concurrent requests can all pass the same pre-charge check and then collectively exceed the limit. The single connection serializes individual statements, not the business-level check/use/record sequence.

The streaming wrapper also records each usage event independently (`:119-123`), while Anthropic supplies split input/output events; the accounting policy should explicitly aggregate or debit deltas.

**Remediation:** reserve estimated cost atomically before the provider call, reconcile with actual usage in a transaction, and define behavior for over-reservation, cancellation, mid-stream failure, and split usage events.

#### F10. Credential migration can delete plaintext that was not successfully migrated

**Classification:** Confirmed security defect, P1.

`migrateEnvFileAt` reads all key/value pairs, attempts only recognized discovery keys, and removes the file when `len(secrets) > 0` even if `migrated == 0` (`credentials/migrate.go:50-85`). Thus a file containing only unknown keys, or recognized keys whose keyring writes all fail, is deleted without a durable replacement. `MigrateEnvFileCredentials` then writes a completion marker (`:31-47`).

The existing migration tests cover empty files and successful/key-existing cases (`credentials/migrate_test.go:142-270`) but do not cover partial or total write failure.

**Remediation:** remove a plaintext file only after every recognized secret is confirmed stored (or explicitly acknowledged as unmapped), preserve the original file on failure, and make the migration marker conditional on successful completion.

#### F11. Conversation SQLite sidecars are not permission-hardened

**Classification:** Confirmed security defect, P1/P2.

`storage.Open` chmods only the main database path after opening WAL mode (`storage/sqlite.go:39-66`). SQLite creates `-wal` and `-shm` sidecars, and those files can contain conversation pages. The budget store demonstrates the missing hardening explicitly by chmodding all three paths (`storage/budgets.go:58-67`). The security tests cover SQL escaping but not filesystem sidecar modes (`storage/security_test.go:9-220`).

**Remediation:** create/verify the parent directory mode, chmod the main file and existing sidecars, and add platform-aware tests that inspect sidecar permissions after writes.

#### F12. The OpenAI-compatible proxy is a convenience adapter, not semantic compatibility

**Classification:** Confirmed compatibility defect, P1/P2.

The endpoint claims that existing OpenAI clients can talk to Flux unchanged (`internal/api/openai_proxy.go:16-20`). Its request message type contains only `Role` and string `Content` (`:22-26`), and `splitOpenAIMessages` flattens the entire conversation into one prompt string (`:300-331`). Tool-call messages, tool results, multimodal content, tool choice, response format, `stop`, sampling controls, and several other accepted fields are either ignored or unavailable (`internal/api/openai_proxy.go:28-44`; `handleOpenAIChatCompletions` explicitly documents lenient partial decoding at `:109-140`).

This is acceptable only if the endpoint is clearly documented as a lossy single-turn compatibility facade. It is not a faithful OpenAI conversation adapter.

**Remediation:** either implement a typed translation into the engine request/message contract, including tool lifecycle and structured output, or narrow the endpoint contract and add explicit capability warnings/headers for unsupported fields.

#### F13. The deployment catalog has two protocol sources of truth

**Classification:** Confirmed design drift, P1/P2.

The registry declares Anthropic and Gemini as native protocols (`catalog/registry/providers.go:20-29`, `:42-50`, `:183-194`), and its protocol-matrix tests explicitly require `anthropic-messages` for Anthropic/Bedrock (`catalog/registry/protocol_matrix_test.go:76-87`). The v1 bootstrap catalog, however, assigns `openai-chat-completions` to `anthropic-direct` and `anthropic-bedrock` (`catalog/v1_defaults.go:43-49`) and only defines OpenAI protocols (`:35-40`). `EnsureCredentialRegistryInCatalog` fills missing deployments but does not correct existing ones (`catalog/credential_registry.go:20-35`).

This is a data-model consistency defect even if current adapter selection works: route metadata, credential sync, protocol diagnostics, and future generated catalogs can disagree.

**Remediation:** make one registry authoritative, generate the other catalog table from it, and add a test that every deployment protocol equals the registry protocol and is present in the catalog protocol map.

### Other confirmed lower-priority defects

- **Adaptive rate-limit zero mode is contradictory.** The configuration says `MaxDelay == 0` returns an error instead of delaying (`provider/resilience/adaptive_ratelimit.go:153-164`), but the constructor replaces every non-positive value with 10 seconds (`:214-226`). The test labels zero as “don't delay” but does not assert the outcome (`provider/resilience/adaptive_ratelimit_test.go:149-177`).
- **Usage tracker zero limits block all calls.** `CanProceed` uses `>=` without a disabled sentinel (`:90-115`), so a zero hourly/daily/session/cost limit is immediately exhausted. `UsageLimitProvider` records cost as zero on both blocking and streaming paths (`:56-67`, `:83-116`), so its cost limit never advances. Either document `<=0` as unlimited or normalize it to a safe default.
- **Batch retry reuses a consumed request body.** `Submit` creates one request and repeatedly calls `Do` without rewinding or invoking `GetBody` (`provider/batch/batch.go:97-121`). The adapter request path explicitly sets `GetBody` for retries (`provider/adapters/anthropic.go:549-557`), but batch submission does not. `WaitUntilDone` also leaves 429/5xx response bodies open before sleeping (`provider/batch/batch_async.go:103-131`), and `Poll` decodes non-200 bodies without checking status (`provider/batch/batch.go:142-166`).
- **Structured output option is a no-op.** `provider/media.WithStructuredOutput` returns an empty `core.ClientOption` and ignores both arguments (`provider/media/structured.go:195-198`). The separate validator supports only a small subset of JSON Schema (`:26-145`), and the array implementation reads a `minimum` keyword where JSON Schema normally uses `minItems` (`:120-145`).
- **Same-role merging is lossy for modern message parts.** `MergeConsecutiveRoles` merges only text and legacy images (`:28-38`) even though canonical messages also carry `Thinking`, `ContentParts`, and `ProviderBlocks` (`llm/types.go:49-64`). It is currently mainly exercised through compatibility tests/aliases, so the immediate blast radius is smaller than the contract suggests, but the exported helper is not safe for multimodal/provider-state history.
- **Graph validation is intentionally minimum-only, not structural.** `GraphSpec.Validate` checks IDs/types and edge fields but not edge endpoint existence, duplicate node IDs, or cycles (`graph/graph.go:254-273`). This is acceptable for a data vocabulary, but should be documented as such or paired with a structural validator before runtime consumers assume more.
- **Small contract inconsistencies remain.** `types.ToAgentId` returns `(nil, nil)` for an invalid ID (`types/types.go:19-25`); `BehaviorPresetFrom` accepts the zero value despite its comment saying empty is invalid (`tools/versioning.go:12-31`); `runtime.ModelIDs` promises sorted output but never sorts (`runtime/runtime.go:164-174`); `operationsgraph/operations_graph.go:1` has the wrong package comment name.
- **Legacy router construction is under-validated.** `router.New` dereferences `e.Provider.Name()` while collecting stats and accepts nil providers, zero/negative weights, and fallback-only configurations (`router/router.go:55-85`). The newer `DeploymentRouter` validates much more carefully (`router/deployment_router.go:74-112`).

### Design drift and simplification opportunities

- **Old multi-package public surface remains.** `runtime/runtime.go:1-22` calls itself the recommended host entry point and lists `provider`, `catalog`, `config`, `credentials`, `setup`, and `storage` as public API, while the accepted boundary says hosts should use `engine` (`README.md:46-65`; `docs/architecture/HOST-ENGINE-BOUNDARY.md:33-36`). Make `runtime` a deprecated compatibility package or update its documentation and ownership.
- **Provider registry composition is coupled.** `provider/adapters/provider_registry.go:5-10` imports catalog, config, and credentials to derive maps and detect providers. The target architecture says adapters translate protocols and do not own routing/credentials (`docs/architecture/FEATURE-MONOREPO.md:48-56`). Move registry composition into `provider/` or `setup`, and make the layering guard cover the full direction rule rather than only provider subpackage imports.
- **Retry models are duplicated.** `types.RetryConfig` (`types/retry.go:5-12`), `provider/core.RetryConfig` (`provider/core/retry.go:15-21`), and `router.RetryConfig` (`router/retry.go:10-16`) represent overlapping policy. Keep one canonical policy plus transport/router-specific behavior, or add compile-time conversion tests and a deprecation plan.
- **Observability has two stacks.** OTel wrappers use `provider/observability/tracing.go:12-136`; the separate zero-dependency package describes its own OTel-compatible model (`internal/observability/observability.go:1-8`, `:23-42`) but has no production import hits. Choose OTel as the execution path and retain only a deliberately isolated test/export implementation.
- **Configuration duplicates provider metadata.** `config/profiles.go:39-64` and `catalog/registry/providers.go:16-29` both define provider modes, endpoints, credentials, and model behavior. `config/provider_env.go:16` also describes the config as mirroring Rho. Generate config profiles from the registry or explicitly scope config as legacy host state.
- **Rho-specific paths remain in supposedly host-neutral code.** `credentials/migrate.go:10-30` hard-codes `~/.rho` and `~/.hawk`; `runtime` documents Rho-specific assembly (`runtime/runtime.go:1-22`). This is acceptable only as an explicit legacy migration package, not as the default engine surface.
- **The optional gRPC surface is intentionally skeletal.** The untagged contract and default service return `ErrUnimplemented` (`internal/grpc/grpc.go:42-67`); the tagged server uses a hand-written JSON codec and a single unary method (`internal/grpc/server_grpc.go:15-78`). The tagged test proves the loopback round trip (`internal/grpc/server_grpc_test.go:21-52`), but there are no generated protobuf clients or SDK gRPC clients. Keep it clearly experimental or make the compatibility contract explicit.
- **The internal cache warmer contains hard-coded pricing and Anthropic assumptions.** `internal/cache/cache_warmer.go:10-31` embeds a model-specific price and cache multipliers. If retained, source pricing/capability data from the catalog and make the warmer provider/profile-specific.

### Cross-layer test gaps

The passing suite is broad, but the most important cross-layer cases are absent:

- No engine conversion test asserts `GenerationOptions.ThinkingEnabled` reaches `ChatOptions.ThinkingEnabled`.
- No adapter-to-engine test carries `ProviderBlock`, `ProviderMetadata`, or structured stream errors.
- Anthropic/OpenAI stream tests do not assert the split `usage` events are persisted by `conversation.Engine`; mocks currently put usage on `done`.
- No test closes a tracing/guardrail-wrapped stream early and verifies upstream cancellation.
- Cache tests do not vary all response-affecting fields or mutate returned provider blocks/warnings.
- Migration tests do not simulate all keyring writes failing or leave unknown secrets in the plaintext file.
- Budget tests are sequential and do not exercise concurrent check/use/record interleavings.
- Storage security tests do not inspect `-wal`/`-shm` permissions.
- Batch tests do not verify body replay after a transient submit response, non-2xx polling, or response-body closure on retry.
- No failover test asserts actual `DeploymentID` and `Attempts` in returned route metadata.
- No black-box conformance matrix runs the same request/stream contract against every adapter.

## Inferences

### Priority roadmap

1. **Close the canonical generation contract before adding more providers.**
   - Restore `ThinkingEnabled` mapping.
   - Standardize aggregate usage events and update conversation/API tests.
   - Decide whether provider blocks, warnings, and structured stream errors are supported or remove them from the stable DTO.
   - Preserve actual deployment route/attempt provenance.
   - Add an adapter conformance harness that treats provider-native metadata as a required contract, not a per-adapter extra.

2. **Fix stream lifecycle and safety wrappers.**
   - Preserve `Close` in every wrapper.
   - Wire `core.StreamGuardrails` or remove the duplicate implementation.
   - Define cancellation behavior for provider errors, pre-first-byte errors, and mid-stream failures.
   - Add fault-injection tests for abandoned streams and slow consumers.

3. **Make budgets and credential migration fail closed.**
   - Replace check-then-record with reservation/reconciliation.
   - Preserve plaintext files until all recognized secrets are durably stored.
   - Define cost/token aggregation for split stream events.
   - Add race tests and migration failure injection.

4. **Consolidate source-of-truth surfaces.**
   - Make the provider registry generate catalog/config profiles.
   - Mark `runtime` and legacy `router.Router` as compatibility APIs or remove them from host guidance.
   - Choose one observability implementation.
   - Rename the exact response cache and remove/ integrate the unused internal cache stack.

5. **Make compatibility boundaries explicit.**
   - Either implement full OpenAI request/message translation or label the proxy lossy and expose unsupported-field diagnostics.
   - Keep gRPC experimental until protobuf/client parity exists.
   - Add source-level tests for host import boundaries, not just provider-internal layering.

6. **Harden operational state.**
   - Secure SQLite sidecars and test actual filesystem modes.
   - Use per-cache unique temporary files and verify atomic rename under concurrent refresh.
   - Enforce HTTPS/host/signature policy for remote catalog refreshes, or document explicit operator trust.
   - Avoid mutating compiled catalog input/owned slices.

### Recommended acceptance criteria

A future “contract v2 hardened” milestone should require:

- every documented `GenerationOptions` field has a conversion test;
- every documented stream event has a producer and host-preservation test;
- usage, route, request ID, and finish reason survive adapter, router, conversation, and proxy boundaries;
- stream `Close` cancels every wrapper and provider body;
- budget tests demonstrate no overspend under concurrent requests;
- migration tests demonstrate no plaintext loss on keyring failure;
- failover responses identify the actual deployment and attempt count;
- every registered provider has a catalog/protocol/config fixture generated from one registry;
- all adapters pass the same local `httptest` streaming/error/usage contract suite.

## Gaps

### Verification record

All commands below passed on the audited checkout:

- `go test ./...`
- `go test -race ./...`
- `go test ./... -count=1 -timeout=120s`
- `go test -race ./... -count=1 -timeout=180s`
- `go vet ./...`
- `go test -tags grpc ./...`
- `golangci-lint run ./... --timeout=5m` — 0 issues
- `gofumpt -l .` — no output
- `goimports -l .` — no output
- `govulncheck ./...` — no vulnerabilities found
- `scripts/check-provider-layering.sh` — passed
- `scripts/check-ecosystem-boundaries.sh` — passed
- `scripts/check-no-replace-directives.sh` — passed

`make ci` was not run because its `tidy` and `fmt` phases mutate files; the read-only equivalents above were run instead. No source files were changed during this audit.

### Remaining uncertainties

- The default catalog’s OpenAI protocol for Anthropic deployments may be a deliberate legacy normalization rather than an accidental mismatch; the registry matrix and current comments disagree, so the owning team must choose the canonical meaning.
- “Semantic caching” may be a product roadmap label for a future similarity implementation; current code is unequivocally exact hashing. The public name and behavior should be reconciled.
- `runtime` may need to remain temporarily for backward compatibility. The boundary is clear, but the deprecation/migration path is not.
- Deployment route fields may be intended to be populated by a host-facing wrapper rather than the lower-level router. Current source has no such wrapper in this repository.
- SQLite sidecar exposure depends on process umask and SQLite creation timing; the source still lacks an explicit guarantee and regression test.
- No live provider credentials, production workloads, or multi-process control-plane deployment were exercised. The signed-manifest and budget findings are static contract findings, not production traffic measurements.

### Audit boundary

This report is a local architectural audit, not a competitor benchmark, security certification, provider conformance certification, or production readiness sign-off. External landscape research is kept in the companion files under `research_notes/Flux OSS landscape roadmap/`; this document records only repository architecture, implementation behavior, and test evidence.
