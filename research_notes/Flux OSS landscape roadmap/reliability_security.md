# Flux Reliability and Security Audit

**Audit date:** 2026-09-24
**Scope:** reliability, routing and resilience, streaming and continuation, credentials, storage, API boundaries, observability, catalog, batch, and adjacent security controls.
**Mode:** read-only source review; no production code was changed.

## Executive Summary

Flux builds cleanly and the normal, race, vet, formatting, lint, vulnerability, and tagged gRPC checks pass. The repository nevertheless has several production-significant gaps at the boundaries where requests become provider calls and streams:

- The host engine reloads catalog/config and reconstructs the provider stack for each request, so opt-in circuit/rate/cache state does not survive and credential lookups can occur repeatedly.
- The conversation path drops structured tool calls, thinking/provider replay state, and Anthropic's separate usage events; a `max_tokens` response without usage can drive continuation indefinitely.
- Stream wrappers do not consistently preserve cancellation, and clean EOF can be presented as successful completion without a terminal `done` event.
- Budget checks and usage recording are not an atomic reservation/idempotency operation, and streaming wrappers can over- or under-record usage.
- The embedded HTTP server can be mounted without the bind-address safety check, and its default listener is plain HTTP.
- Routing can append automatic fallback even when the caller did not permit fallback, can silently strip tools, and does not reliably report the deployment that actually served a request.

No single issue proves a universal critical vulnerability, but the combination of stream state loss, budget races, credential migration behavior, and weak admission/deadline defaults makes the current system unsuitable for an untrusted multi-tenant deployment without additional host controls.

## Verification Performed

All commands were run from `/Users/lakshmanpatel/Desktop/OSS2026/graycode-eco/flux` without changing source files.

| Check | Result |
|---|---|
| `go test ./...` | PASS |
| `go test ./... -race -count=1 -shuffle=on -timeout=300s` | PASS |
| `go vet ./...` | PASS; no diagnostics |
| `govulncheck ./...` | PASS; `No vulnerabilities found.` |
| `gofumpt -d .` | PASS; no diff |
| `goimports -d .` | PASS; no diff |
| `golangci-lint run ./...` | PASS; `0 issues.` |
| `git diff --check` | PASS |
| `go test -tags grpc ./internal/grpc` | PASS |

These checks establish build and static-analysis health; they do not cover the lifecycle, security, and concurrency gaps below.

## Findings

### F-01 — High — Per-request state reload resets runtime protections

**Classification:** Confirmed when the engine's opt-in wrappers are enabled; the state reload itself occurs on the normal path.

**Evidence**

- Selection loads runtime state in `engine/engine.go:329-372`, then transport resolution loads it again in `engine/engine.go:303-326`.
- `loadRuntimeState` reads and compiles the catalog, reloads provider configuration, and scans credential accounts in `engine/state.go:86-129`.
- `DeploymentProviderFromState` constructs a new deployment router in `setup/deployment.go:86-110`.
- The adaptive limiter and response cache are constructed inside every `defaultTransport` call in `engine/engine.go:315-325`.

**Impact**

Circuit-breaker history, adaptive rate-limit windows, and response-cache entries are reset per request. Every request can also perform catalog/config I/O and multiple keychain lookups. Under load, this adds avoidable latency and can turn a healthy provider into an apparently unhealthy one because failure history never accumulates. Selection and transport can observe different state if files or credentials change between the two loads.

**Recommendation**

Build one immutable runtime snapshot containing the compiled catalog, provider configuration, resolved adapters, and middleware. Select and execute from the same snapshot. Recompile only after an explicit config/catalog mutation or refresh, with generation-based invalidation. Add a single request admission/runtime owner that persists limiter, breaker, and cache state.

**Tests to add**

Assert one state load per generation, stable limiter/breaker/cache instances across requests, and deterministic invalidation after config or credential rotation.

### F-02 — High — Conversation streaming loses tools, usage, reasoning, and continuation state

**Classification:** Confirmed.

**Evidence**

- `conversation.Engine.streamAndSave` only handles `content`, `done`, and `error` events in `conversation/engine.go:266-291`; `tool_call`, `thinking`, `usage`, `warning`, and provider-block events are ignored.
- It persists only text and token usage attached to the `done` event in `conversation/engine.go:315-335`.
- Anthropic streams prompt usage in `message_start` and output usage in `message_delta`, both as separate `usage` events, in `provider/core/stream.go:236-281`.
- Continuation is based only on `stopReason == "max_tokens"` and cumulative completion tokens in `conversation/engine.go:305-313`; there is no hard continuation-call limit.
- The API accepts tools and passes them into `PromptOpts` in `internal/api/server.go:163-187`, but the conversation path does not convert emitted tool calls into persisted tool-call nodes.

**Impact**

- Tool-using responses can be returned as empty or text-only responses, with the model request for execution silently lost.
- Anthropic streaming requests can persist zero token usage and incorrect cost data.
- A provider that emits `max_tokens` without usage leaves `cumulativeOut` at zero, so `cumulativeOut < groupBudget` remains true and the conversation goroutine can continue indefinitely.
- Thinking text and provider replay state are not retained in the conversation DAG.

**Recommendation**

Use a single event accumulator that understands the complete normalized event vocabulary. Persist tool-call/tool-result nodes, thinking metadata, provider blocks, and exactly one finalized usage record. Make continuation limits mandatory and count both continuations and total provider calls. Treat missing usage as a provider/accounting error when a terminal response claims usage-dependent behavior rather than as zero forever.

**Tests to add**

Cover Anthropic streaming usage, tool calls, warning events followed by `done`, a `max_tokens` response with no usage, hard continuation limits, and replaying a stored assistant turn.

### F-03 — High — Stream terminal and cancellation contracts are inconsistent

**Classification:** Confirmed.

**Evidence**

- `engine.Stream.forward` treats a closed source channel as a normal return and does not require a terminal event in `engine/stream.go:82-107`.
- `engine/continuation.go:62-70` synthesizes a `done` event when the upstream stream ended without one.
- OpenAI parsing calls `finish("")` on a clean channel close or `[DONE]` in `provider/core/stream.go:455-494`; a transport EOF is therefore indistinguishable from a provider-completed response.
- The SSE parser does not flush a final event when EOF arrives before a blank separator in `provider/core/stream.go:43-89`.
- Guardrails and tracing return stream literals without a cancel function in `provider/resilience/guardrails.go:83-118` and `provider/observability/tracing.go:91-135`.
- The wrapper contract in `llm/types.go:289-307` relies on `Close` invoking the cancellation function.

**Impact**

A truncated or cancelled provider stream can be reported as a successful empty response. A consumer that abandons a wrapped stream can leave the forwarding goroutine and upstream HTTP body alive because `Close` has no cancellation action. Provider-specific health warnings can also be lost or converted into fatal errors by different layers.

**Recommendation**

Define one stream terminal state machine: `done`, `error`, or `cancelled`; require an explicit terminal event or return a typed truncation error. Make every wrapper use a request-scoped child context, preserve the source cancel function, and close/drain the source when the consumer stops. Flush a final SSE event only when the protocol permits it; otherwise emit a typed error.

**Tests to add**

Use a blocking fake provider to assert that `Close` cancels the source, a truncated body never yields a clean `done`, a warning/error event reaches the expected normalized type, and EOF-only streams are classified consistently.

### F-04 — High — Missing admission and overly broad default deadlines permit resource exhaustion

**Classification:** Confirmed defaults; impact is highest when the HTTP API or another host exposes the engine to concurrent callers.

**Evidence**

- The shared transport has no `MaxConnsPerHost` or `MaxConns` in `provider/core/transport.go:28-45`.
- Provider clients use a ten-minute end-to-end timeout in `provider/core/transport.go:10-11,56-63`.
- `requestContext` applies a timeout only when the caller supplies one in `engine/engine.go:508-513`.
- The embedded server uses a ten-minute write timeout and has no request or stream admission limit in `internal/api/server.go:73-85`.
- `DeploymentRouter.StreamChat` starts a goroutine and returns a stream without a global queue or concurrency bound in `router/deployment_router.go:182-247`.
- The API config documents virtual-key budget enforcement as optional and unmetered fallback behavior in `internal/api/server.go:35-48`.

**Impact**

Slow or stalled upstreams can retain connections, goroutines, memory, and budget reservations for long periods. A caller can open many streams without a bounded number of in-flight requests, causing upstream throttling, local memory pressure, and noisy retry amplification.

**Recommendation**

Add a shared admission controller with per-provider, per-tenant, and global limits. Use separate bounded deadlines for connect, response headers, first event, inter-event idle time, and total request duration. Enforce queue size and cancellation while waiting. Make limits explicit in the host-facing engine rather than relying on the ten-minute provider default.

### F-05 — High — Budget enforcement is not an atomic reservation or idempotent charge

**Classification:** Confirmed.

**Evidence**

- `CheckBudget` only reads current totals in `storage/budgets.go:157-178`.
- `RecordUsage` performs the increment and ledger insert later in a separate transaction in `storage/budgets.go:181-209`.
- `BudgetProvider` checks before the provider call and records afterward in `provider/observability/budget_provider.go:73-91,94-142`.
- `UsageLimitProvider` performs a pre-check and records later in `provider/observability/usage_limit.go:53-68,70-105`.
- Streaming wrappers record every `usage` event and ignore `RecordUsage` errors in `provider/observability/budget_provider.go:116-123` and `provider/observability/usage_limit.go:83-94`.
- The SQLite ledger stores an empty model and has no request/idempotency key in `storage/budgets.go:99-108,202-205`.

**Impact**

Concurrent requests can all pass the same pre-check and exceed a limit. Retries, reconnects, or multiple/cumulative usage events can create duplicate ledger entries. Cancellation can cause a successful provider call to go uncharged because the record error is discarded. Cost attribution cannot reliably reconstruct which request or model incurred a charge.

**Recommendation**

Introduce a reservation operation that atomically reserves estimated spend, followed by an idempotent finalization keyed by a provider request ID or Flux operation ID. Define whether usage events are incremental or cumulative, finalize once per request, and make ledger/model attribution mandatory. Treat a failed charge as an operational error or a durable retryable outbox event, not an ignored value.

### F-06 — High — Embedded API exposure has unsafe default and mounting behavior

**Classification:** Confirmed API design; exploitability depends on how the host mounts or exposes it.

**Evidence**

- Auth bind validation is called only by `ListenAndServe` in `internal/api/server.go:73-85` and `internal/httputil/httputil.go:88-103`.
- `Server.ServeHTTP` directly serves the handler in `internal/api/server.go:69-71`; a host mounting that handler on a non-loopback listener bypasses the safety check.
- `ValidateAuthConfig` allows any non-loopback bind when an API key is configured, but `ListenAndServe` uses plain HTTP in `internal/api/server.go:77-85`.
- `NewServer` does not validate a provider/store/dependency combination and readiness only checks wrapper pointers in `internal/api/server.go:50-66,150-161`.
- Node, alias, analytics, and provider-health routes have no tenant/owner scope in `internal/api/server.go:101-121`.

**Impact**

A mounted handler with an empty API key can be exposed without the loopback safeguard. A non-loopback plain-HTTP deployment sends bearer credentials and prompts in cleartext unless an external TLS terminator is correctly configured. In a shared store, any authenticated caller can potentially inspect or mutate conversations not owned by that caller. A server with a non-nil conversation wrapper but unusable provider can report ready.

**Recommendation**

Make server construction validate required dependencies and return an error. Put authorization and tenant scope on every sensitive route, not only at the listener boundary. Require TLS termination or an explicit `InsecureLocalOnly` mode for plain HTTP; refuse a non-loopback insecure bind even when an API key is set. Make readiness probe provider/configuration readiness, not just pointer presence.

### F-07 — High/Medium — Deployment fallback can violate caller policy and capability requirements

**Classification:** Confirmed in the deployment router; whether a caller is affected depends on route configuration.

**Evidence**

- Explicit routing is always followed by automatic fallback in `router/deployment_router.go:320-350,683-707`; there is no `AllowFallback` field in `core.ChatOptions` or the deployment router call.
- If no route supports all requested server tools, `eligibleChoices` returns the non-tool-capable choices in `router/deployment_router.go:365-389`.
- `optsForOffering` then silently removes unsupported tools in `router/deployment_router.go:554-576`.
- `streamWithDeployment` buffers every non-output event before the first output without a bound in `router/deployment_router.go:441-503`.
- The router records every failure before checking whether the error is transient in `router/deployment_router.go:160-173,212-236`.

**Impact**

An exact-model request can fail over even when the caller intended no fallback. A request that requires a tool can be sent to a provider without that tool, producing a semantically different request. A non-transient auth or client error can open a deployment circuit. A provider that emits an unbounded number of pre-output metadata events can consume memory before the first visible token.

**Recommendation**

Propagate an explicit fallback policy and original requirements into the router. Reject a stage if required capabilities cannot be preserved; never silently drop tools. Bound metadata buffering and expose a typed pre-output overflow error. Classify cancellation and non-transient errors before updating circuit state. Treat budget/auth failures separately from endpoint availability.

### F-08 — Medium — Route attribution and resilience telemetry describe the wrong attempt

**Classification:** Confirmed.

**Evidence**

- The host route contract includes actual deployment and attempt fields in `llm/provider.go:235-246`.
- Engine attaches the initially selected route in `engine/convert.go:88-97` and `engine/engine.go:283-300`.
- Deployment routing selects and retries deployments in `router/deployment_router.go:134-179,182-247`, but does not populate the response route with the successful deployment or attempt count.
- `EventRouteChanged` is declared in `engine/types.go:65-80`, but no production emission was found.
- Usage wrappers construct new stream results without preserving the request ID in `provider/observability/types.go:21-26` and `provider/observability/budget_provider.go:116-133`.

**Impact**

Billing, telemetry, debugging, and cache attribution can point at the first route rather than the deployment that actually served the request. Retry/fallback behavior is difficult to audit, and provider request IDs disappear through budget/usage wrappers.

**Recommendation**

Return a route result from each deployment attempt and attach the successful deployment/attempt to both blocking and streaming events. Emit `route_changed` when a retry or fallback changes the serving deployment. Preserve request IDs in every wrapper, preferably through a context or immutable stream metadata object rather than manually reconstructing wrappers.

### F-09 — Medium — Response cache correctness is not safe across providers, tenants, or response state

**Classification:** Confirmed.

**Evidence**

- The documented default says `CacheConfig.Enabled` is true, but the zero value is false and `NewCachedProvider` never enables it; see `provider/cache/semantic_cache.go:15-37,73-92`.
- The key includes model, system, temperature, and basic message content/tool data only in `provider/cache/semantic_cache.go:296-340`. It omits provider/deployment, tenant, tool choice, response format, seed, multimodal parts, stop sequences, top-p, and other behavior-affecting options.
- `core.CopyResponse` copies usage and some tool argument maps but not route, warnings, provider blocks, or arbitrary nested typed values in `provider/core/copy.go:3-56`.
- Cached entries are returned directly through a shallow response copy in `provider/cache/semantic_cache.go:176-199`.

**Impact**

An engine fallback can serve a response cached by a different deployment under the same model key. A tenant or credential context can observe another context's response if the cache is shared. Mutating opaque tool arguments, route metadata, or provider state can race with another consumer. Enabling caching with a zero config silently does nothing, making the documented behavior misleading.

**Recommendation**

Define a canonical request identity containing provider/deployment, tenant, model, all generation-affecting options, tool declarations/choice, and normalized message parts. Make cache scope explicit and never share across tenants unless deliberately namespaced. Use a complete immutable response snapshot or structured clone for cache entries. Add collision and concurrent-mutation tests.

### F-10 — Medium — Coalescing couples unrelated caller lifetimes and leaks state

**Classification:** Confirmed.

**Evidence**

- The first caller's context becomes the shared request context in `provider/resilience/coalesce.go:129-151`.
- A creator-context cancellation cleanup goroutine returns without deleting the map entry in `provider/resilience/coalesce.go:153-167`.
- Waiters are incremented but never decremented in `provider/resilience/coalesce.go:113-126,176-189`.
- The same response pointer is returned to every waiter in `provider/resilience/coalesce.go:125-127,176-187`.
- The key includes only provider, model, messages, temperature, and max tokens in `provider/resilience/coalesce.go:16-48`; system/tools and other request-affecting options are not part of the key.

**Impact**

Cancelling the creator cancels work for otherwise live waiters. Completed entries can remain forever when the creator context is already canceled, and the waiter counter eventually rejects new work even when no callers are waiting. Shared mutable response/tool maps can be changed by one consumer. Requests with different system or tool settings can be incorrectly coalesced.

**Recommendation**

Use a request-owned context independent of any waiter, cancel it when the final waiter leaves, decrement waiter state on every exit, and remove completed entries deterministically. Include a complete canonical request identity and return deep copies to waiters.

### F-11 — Medium — Provider-specific replay state is declared but not implemented

**Classification:** Confirmed contract gap.

**Evidence**

- The host contract explicitly defines `ProviderBlocks` for Anthropic thinking signatures, redacted thinking, and OpenAI reasoning items in `llm/types.go:49-75,248-287`.
- A repository search found definitions but no production adapter assignment or replay consumer.
- Anthropic response parsing extracts thinking text but skips redacted blocks and does not preserve signatures/data in `provider/adapters/anthropic.go:233-300`.
- Anthropic stream parsing records block type only to route thinking deltas and does not emit provider-block events in `provider/core/stream.go:165-210`.
- Engine normalization has no provider-block branch in `engine/stream.go:124-165`.

**Impact**

A second turn cannot replay required signed reasoning state. The first turn may appear successful, while the follow-up is rejected or produces a different answer. Redacted thinking data is silently discarded, contrary to the stated replay contract.

**Recommendation**

Make provider blocks a required part of the adapter response/stream contract. Preserve the exact wire block and provider, emit a normalized provider-block event, persist it with the assistant node, and replay only matching protocol blocks. Treat unsupported replay as an explicit warning or error rather than silently dropping it.

### F-12 — Medium — Credential migration can delete secrets after partial failure

**Classification:** Confirmed.

**Evidence**

- `migrateEnvFileAt` reads all key/value pairs but migrates only recognized discovery keys in `credentials/migrate.go:50-81`.
- It removes the file when `len(secrets) > 0`, even if `migrated == 0` or some keychain writes failed, in `credentials/migrate.go:82-85`.
- Migration marker creation ignores filesystem errors in `credentials/migrate.go:22-24,31-47`.

**Impact**

An env file containing an unrecognized secret, a partially failing keychain write, or a key not present in the current discovery registry can be deleted with the secret still only in the file, or partially migrated into the keychain. Users can lose credentials during an upgrade.

**Recommendation**

Treat migration as a transaction. First enumerate and validate every supported key, write all destinations, verify writes, and only then remove or atomically rename the source file. Preserve unrecognized entries in a permission-restricted backup and return an actionable error on any failure. Treat marker creation as part of migration success.

### F-13 — Medium — Credential isolation and endpoint policy are too implicit for multi-tenant use

**Classification:** Confirmed code paths; highest impact when configuration is tenant-controlled.

**Evidence**

- `ServiceName` is mutable process-global state without synchronization in `credentials/store.go:11-25`; default store replacement is protected separately.
- `CombinedStore` keeps decrypted secrets in a process cache for two seconds in `credentials/combined.go:18-34,57-115`.
- Compatibility configuration writes provider secrets into process environment variables in `config/provider_env.go:315-334,651-670`.
- Custom gateway URL validation checks syntax but permits HTTP, arbitrary hosts, and private addresses in `engine/host_runtime.go:46-90`; the dynamic-provider helper has the same limitation in `provider/dynamic.go:54-58`.
- Probe HTTP follows redirects by default in `internal/probehttp/probehttp.go:49-79`.
- OIDC uses `http.DefaultClient` when no client is injected in `credentials/oidc.go:49-55,83-97`.

**Impact**

Process-global environment and service settings can leak across tenants or hosts. A custom endpoint controlled by a less-trusted actor can receive API keys or reach internal services. Redirects and DNS changes can bypass an initially safe URL check. Default OIDC requests have no client-level timeout when the caller supplies no deadline.

**Recommendation**

Use explicit per-engine credential namespaces and injected stores in host paths. Prohibit process-environment fallback in strict host mode. Validate URL schemes, resolved IPs, ports, redirects, and DNS rebinding at connection time; require explicit opt-in for private/local gateways. Use a bounded client and trusted endpoint allowlist for OIDC/probe requests. Treat cached plaintext secrets as a deliberate, configurable threat-model tradeoff.

### F-14 — Medium — Storage permissions and secret-at-rest handling are incomplete

**Classification:** Confirmed; impact depends on host filesystem and key-storage policy.

**Evidence**

- The budget database stores provider API keys in `virtual_key_secrets` and documents plaintext storage in `storage/budgets.go:44-68,83-87`.
- Permission errors for the budget database and WAL/SHM sidecars are ignored in `storage/budgets.go:58-68`.
- The conversation SQLite store protects the main database but does not protect WAL/SHM sidecars in `storage/sqlite.go:48-66`.
- Recorder cassettes persist request messages and response/tool data, and request hashes intentionally omit images, temperature, and other varying options in `provider/observability/recorder.go:99-140,279-340` and `provider/observability/cassette.go:26-41,92-121`.

**Impact**

A permissive umask or ignored chmod failure can expose database sidecars, provider keys, prompts, or tool arguments. Shared replay cassettes can retain sensitive user data and can match a different request because the hash omits important fields.

**Recommendation**

Prefer credential references or an encrypted/keychain-backed store over plaintext provider keys. Treat chmod failures as fatal, secure all SQLite sidecars, and test them under a permissive umask. Define cassette redaction, retention, encryption, and complete request identity before enabling recording outside tests.

### F-15 — Medium — Health checks can report false healthy states and race during lifecycle transitions

**Classification:** Confirmed.

**Evidence**

- Anthropic `Ping` returns nil for every status except 401 in `provider/adapters/anthropic.go:640-656`; Bedrock does the same for 401/403 in `provider/adapters/bedrock.go:262-281`. Other adapters follow similar patterns.
- `NewHealthChecker` replaces the entire supplied config with defaults when only `Interval` is zero in `internal/health/healthcheck.go:100-110`; partially specified configs otherwise retain zero timeout/thresholds.
- `Check` returns a result without persisting it in `internal/health/healthcheck.go:143-164`; only the background loop calls `updateStatus`.
- Failure counts are read and then incremented separately under different locks in `internal/health/healthcheck.go:262-329`.
- `Stop` clears `cancel` before waiting for `done` at `internal/health/healthcheck.go:181-218`; a concurrent `Start` can therefore begin a new loop while the old loop is still shutting down, allowing overlapping health checks.

**Impact**

A provider returning 500, 404, or rate-limit responses can be marked healthy. Immediate checks and dashboard results can disagree. Concurrent checks can lose failure transitions. A start/stop race can run overlapping health loops and duplicate pings or distort failure state.

**Recommendation**

Normalize `Ping` to treat every non-success status as an error, validate each config field independently, persist immediate checks, and update state atomically. Capture the per-run done channel locally, serialize Start/Stop transitions, and add jitter to periodic checks.

### F-16 — Medium — OpenAI-compatible proxy is a lossy subset and reports failures as normal completion

**Classification:** Confirmed compatibility limitation.

**Evidence**

- `openAIChatMessage.Content` is a string, so standard array/object multimodal content cannot be represented in `internal/api/openai_proxy.go:22-25`.
- `n`, `top_p`, stop, presence/frequency penalties, and other fields are explicitly accepted but ignored in `internal/api/openai_proxy.go:28-44`.
- `splitOpenAIMessages` flattens roles into transcript text in `internal/api/openai_proxy.go:300-331`.
- On a conversation error, the proxy emits an error data event, then still emits a finish chunk and `[DONE]` in `internal/api/openai_proxy.go:238-258`.
- Tool calls are accepted but the underlying conversation path does not surface them; see F-02.

**Impact**

Clients can receive successful-looking responses with missing options, malformed multimodal input, or no tool calls. A failed stream is easy for OpenAI-compatible clients to interpret as a completed generation.

**Recommendation**

Either explicitly document the supported subset and reject unsupported combinations, or add canonical conversion for multimodal content, tool-call events/results, accepted generation options, and a terminal error protocol that does not emit a normal finish after failure. Add contract tests against representative OpenAI requests.

### F-17 — Medium/Low — Batch and live-catalog auxiliary paths have cancellation and resource-bound gaps

**Classification:** Confirmed in opt-in/auxiliary paths.

**Evidence**

- Batch submit reuses one request after `http.Client.Do` consumes its body and sleeps with `time.Sleep` in `provider/batch/batch.go:97-121`.
- `WaitUntilDone` can use a Retry-After delay beyond the overall timeout and uses `time.After`; transient responses are not drained/closed before the next poll in `provider/batch/batch_async.go:87-132`.
- Poll result JSONL is bounded to 4 MiB per line but the whole result slice is retained in memory in `provider/batch/batch_async.go:147-183`.
- Live fetchers use a 30-second HTTP client, but `FetchFunc` has no context parameter and `FetchLiveProviderCatalog` loops sequentially in `catalog/live/fetchers.go:20-36,52-100` and `catalog/live_enrich.go:14-101`; a refresh can therefore accumulate many provider timeouts.
- Gemini model discovery places the API key in a query string in `catalog/live/fetchers_providers.go:464-478`.
- Remote catalog URLs are accepted from explicit input/environment and fetched without signature/content trust validation in `catalog/v1.go:669-720`.
- `MemoryBackend` has no maximum entry count or byte bound in `internal/cache/backend.go:43-87`; `CacheWarmer.Stop` can race before its loop captures the stop channel in `internal/cache/cache_warmer.go:91-145`.

**Impact**

Batch callers can exceed their context deadline, leak connections, or retry an unusable request. Catalog refresh can block for many provider timeouts and cannot be canceled through its public fetch API. API keys can appear in proxy logs or browser history. Shared memory caches can grow until process pressure, and warmer shutdown can leave a goroutine running.

**Recommendation**

Clone request bodies for every retry, use context-aware timers, cap every delay by the remaining deadline, close/drain error bodies, and bound result memory. Make live/catalog fetch functions context-aware and concurrent with a global deadline. Use headers rather than query secrets, authenticate/pin remote catalogs, bound cache bytes/entries, and make stop/start lifecycle ownership explicit.

### F-18 — Low/Medium — Telemetry and audit data are not consistently lifecycle-safe or convention-aligned

**Classification:** Confirmed instrumentation gaps; exporter absence depends on host integration.

**Evidence**

- Provider tracing uses custom keys such as `provider.name` and `usage.prompt_tokens` in `provider/observability/tracing.go:33-67,70-130`, while the repository separately defines `gen_ai.*` keys in `internal/observability/genai_semconv.go:21-58`; no production use of those canonical keys was found.
- `conversation.Prompt` ends its span immediately after returning the event channel in `conversation/engine.go:166-175`, before asynchronous work completes.
- Recorder request data is stored in `provider/observability/recorder.go:113-140,320-340`; the default redactor is nil.
- Callback hooks launch one goroutine per callback/event in `provider/observability/callbacks.go:117-177,194-219`.
- Audit hashes are unsalted SHA-256 values in `internal/observability/audit.go:93-100`; the same prompt can therefore be correlated across tenants/logs.
- No OTel SDK/exporter setup was found in the audited repository paths; API tracing relies on the global provider, so a host must install one.

**Impact**

Spans can end early, wrapper request IDs can be lost, callback traffic can amplify memory/CPU, and telemetry attributes may not aggregate with the shared GrayCodeAI conventions. Raw cassette data and unsalted hashes can create privacy or correlation risk.

**Recommendation**

Use the canonical `gen_ai.*` and `cost.usd` attributes, bind span completion to stream closure, preserve request IDs, bound asynchronous callback delivery, and make redaction mandatory for recorders. Use tenant-scoped keyed hashes or omit content hashes where correlation is unnecessary. Document exporter wiring as a host responsibility and test it with an in-memory exporter.

## Confirmed Good Controls

- Native JSON request decoding uses a body limit and rejects unknown fields in `internal/httputil/httputil.go:33-54`.
- Provider JSON response parsing generally caps error bodies, and live catalog responses are bounded.
- Control-plane manifests are copied before publication, revision conflicts are rejected, and remote peers require HTTPS/signature keys; peer redirects are disabled in `router/controlplane/snapshot.go:68-140,143-204` and `router/controlplane/peers.go:74-110`.
- Control-plane route validation caps retries at 32 in `router/controlplane/snapshot.go:159-203`. Direct `NewDeploymentRouter` callers do not receive equivalent validation, which is why that path remains a risk.
- `doWithMimoAuthRetry` and shared probe code use context-aware requests and bounded clients in several paths.
- Provider-specific API keys are not serialized into the normal sanitized provider state; setup migrates them into the injected store in `engine/state.go:23-83`.
- Race-enabled tests and static checks pass, but the test suite does not cover the lifecycle and adversarial cases listed below.

## Prioritized Remediation Plan

### P0 — Protect correctness and spend

1. Add a single stream terminal/cancellation coordinator and require explicit terminal state.
2. Fix conversation accumulation for tool calls, provider blocks, thinking, and final usage; enforce a hard continuation limit.
3. Make credential migration transactional and never delete an unverified source file.
4. Replace budget check/record with an atomic reservation/finalization protocol and idempotency key.
5. Add admission/deadline limits before exposing the API to untrusted callers.
6. Make API construction fail closed for missing dependencies, auth, TLS mode, and tenant scope.

### P1 — Make routing and middleware policy-correct

1. Cache and version runtime snapshots; preserve limiter, breaker, and cache state.
2. Propagate fallback and capability requirements through deployment routing; preserve request identity and actual route attribution.
3. Canonicalize cache and coalescer keys and deep-copy returned responses.
4. Normalize provider replay state and health/Ping semantics.
5. Secure custom endpoints, credential namespaces, and all secret-at-rest paths.

### P2 — Improve operational edges

1. Complete telemetry lifecycle and shared OpenTelemetry conventions.
2. Make batch/catalog/cache warmer operations context-aware and bounded.
3. Add compatibility tests for the OpenAI proxy and clarify unsupported options.

## Test Gaps

The following tests are especially important before production use:

- Credential migration with unrecognized keys, partial keychain failure, existing values, and unwritable source/marker files.
- Conversation streaming with Anthropic input/output usage events, tool calls, warning events, provider blocks, no-usage `max_tokens`, and hard continuation limits.
- Stream wrapper `Close` cancellation and goroutine/body cleanup for guardrails, tracing, cache, budget, usage-limit, and recorder wrappers.
- EOF, truncated SSE, scanner errors, missing terminal events, and provider cancellation normalized consistently across adapters.
- Concurrent budget reservations, duplicate finalization, request-ID idempotency, failed ledger writes, and multi-event/cumulative usage.
- Router fallback with `AllowFallback=false`, missing tool capability, non-transient failure, unbounded pre-output events, and actual route/attempt attribution.
- Cache collisions across provider/deployment/tenant, all generation-affecting options, and concurrent mutation of cached tool arguments.
- Coalescer creator cancellation, waiter decrement, TTL cleanup, response isolation, and complete request-key equality.
- API non-loopback plain HTTP, `ServeHTTP` mounting without listener validation, unknown virtual keys, dependency readiness, tenant isolation, and OpenAI multimodal/tool/error streams.
- URL/redirect/private-address policy for custom gateways, probes, remote catalogs, and OIDC endpoints.
- Provider `Ping` returning 404/429/5xx, health config validation, concurrent `Check`, and Start/Stop/Start transitions.
- Batch cancellation, body reuse, Retry-After beyond deadline, response-body closure, and result-size bounds.
- OTel span completion and canonical attributes with an in-memory exporter, bounded callbacks, and recorder redaction.

## Final Assessment

Flux has a strong provider abstraction and useful defensive controls, but its current reliability guarantees are mostly local to individual provider calls. Production readiness depends on making stream state, routing identity, budget accounting, credential migration, and admission control shared primitives rather than independent wrapper behavior. The highest-value next change is a small, versioned request-runtime/stream-terminal core used by engine, conversation, router, budget, and observability paths; it would address several findings without requiring a platform-wide rewrite.
