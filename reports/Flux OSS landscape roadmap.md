# Build Flux around trustworthy provider interoperability

**Recommendation:** keep Flux a narrow, embeddable Go provider runtime and make correctness, lossless protocol behavior, and evidence-backed conformance its differentiator—not UI, tenancy, agent orchestration, or model execution. The repository has a credible foundation, but several public promises are ahead of the composed runtime: stateful middleware is rebuilt per request, provider and route state is lost, budgets and migration are not transaction-safe, and the advertised first-run path is not independently reliable. Against the selected 20-project OSS landscape, Bifrost, AxonHub, and GoModel are the closest implementation peers; LiteLLM, Agent Router, Portkey, and the official provider SDKs are the strongest semantic donors. The next sequence is therefore: **make current contracts true, unify provider construction, prove conformance, complete OpenAI and Anthropic semantics, then publish a narrow `v0.1.0`**. Model serving, agent loops, and general gateway control planes remain external concerns, while `v1.0` waits for a compatibility baseline, reproducible releases, security ownership, and external-consumer evidence.

**Research cutoff:** 2026-09-24. **Flux snapshot:** `f9037a82e403001d50868e4100bcfa19d8a387af` on `chore/oss-competitive-roadmap`. **Evidence convention:** “verified” means confirmed in the cited source or recorded local check; “interpretation” means an engineering judgment; “assumption” means a condition that must be validated before implementation.

## A narrow thesis beats feature-count parity

Flux already has the right product boundary: hosts import `engine`, `llm`, `graph`, and `tools`, while Flux owns credentials, catalog resolution, provider transports, normalized streams, retry/fallback, usage, and provider telemetry (`README.md:31-65`; `docs/architecture/HOST-ENGINE-BOUNDARY.md:6-36`). Hosts own UX, sessions, permissions, tool execution, and agent loops. That boundary is strategically valuable because the closest peers usually bundle one of the rejected layers: a web console and tenancy, a Kubernetes control plane, a language SDK ecosystem, or model execution. The competitive product is consequently not “another LiteLLM.” It is:

> **the narrow, auditable Go runtime that makes heterogeneous hosted and self-hosted LLM endpoints behave like one explicit, lossless, conformance-tested contract, without requiring a gateway service, database, control plane, agent runtime, or model server.**

The research produced two related but distinct views that must not be conflated. `selection_methodology.md` defines a reproducible **evidence-ranked** portfolio using public source, an OSI-approved core, activity within 180 days, Flux-specific evidence, and the score `6F + 4C + 3I + 3M + 2G + O + E`. It caps the evidence set at eight direct peers, six serving runtimes, four provider SDKs, and two adjacent donors. `gap_mapping.md` then reconciles that portfolio with the runtime and SDK findings into the **decision portfolio** below; it explicitly supersedes the scoped lists where they conflict. GitHub counts, release labels, and maintenance signals are 2026-09-24 snapshots, not quality scores, and upstream documentation establishes intent rather than field-by-field wire conformance. ([GitHub repository API](https://docs.github.com/en/rest/repos/repos#get-a-repository))

| Decision rank | Project | Portfolio role | Evidence-ranked position | Primary Flux use | Decision |
|---:|---|---|---|---|---|
| 1 | [Bifrost](https://github.com/maximhq/bifrost) | Direct peer; closest Go runtime | 1 (97.0) | Provider abstraction, operation matrices, raw-response inspection, narrow middleware | **Borrow and build evidence surfaces** |
| 2 | [LiteLLM](https://github.com/BerriAI/litellm) | Direct peer; feature benchmark | 2 (95.0) | Per-attempt policy revalidation, route/attempt semantics, spend, privacy-safe OTel | **Borrow semantics; reject platform shape** |
| 3 | [AxonHub](https://github.com/looplj/axonhub) | Direct peer; Go transformer pipeline | 4 (91.5) | Inbound dialect → canonical request → provider → reverse stream/error transforms | **Borrow architecture** |
| 4 | [GoModel](https://github.com/ENTERPILOT/GoModel) | Direct peer; compact Go runtime | 5 (90.0) | Policy composition, aliases, budgets, usage, migration ergonomics | **Borrow selectively** |
| 5 | [Agent Router](https://github.com/theagentrouter/agent-router) | Direct peer; reliability/extension donor | 7 (84.5) | Stream idle deadlines, per-request credentials, provider extensions | **Borrow reliability semantics** |
| 6 | [Portkey Gateway](https://github.com/Portkey-AI/gateway) | Direct peer; maintenance-watch benchmark | 10 (81.5) | Request-attached policy, sticky routing, conditional fallback | **Borrow policy shape only** |
| 7 | [OmniRoute](https://github.com/diegosouzapw/OmniRoute) | Direct peer; high-velocity gateway donor | 3 (94.0) | Provider fallback, aliases, native passthrough, migration diagnostics | **Borrow selectively** |
| 8 | [Manifest (`mnfst/llm-gateway`)](https://github.com/mnfst/llm-gateway) | Direct peer; routing-focused | 8 (83.5) | Route hints, model tiers, affinity, cost-aware selection | **Defer until baselines exist** |
| 9 | [LocalAI](https://github.com/mudler/LocalAI) | Serving runtime; local profile target | 6 (85.5) | Backend-neutral discovery and profile metadata | **Adopt as conformance target** |
| 10 | [Ollama](https://github.com/ollama/ollama) | Serving runtime; local lifecycle profile | 9 (82.0) | Native tags/show/keep-alive and structured-output profile | **Build read-only profile support** |
| 11 | [llama.cpp](https://github.com/ggml-org/llama.cpp) | Serving runtime; portable profile | 15 (69.0) | Native props/model/slot evidence and constrained JSON | **Adopt as endpoint profile** |
| 12 | [vLLM](https://github.com/vllm-project/vllm) | Serving runtime; conformance target | 12 (74.0) | Version-sensitive OpenAI/Anthropic surfaces and serving topology | **Adopt as conformance target** |
| 13 | [SGLang](https://github.com/sgl-project/sglang) | Serving runtime; gateway-boundary donor | 11 (74.5) | Cache/load signals, gateway lifecycle, plane separation | **Adopt as profile and signal donor** |
| 14 | [any-llm-go](https://github.com/mozilla-ai/any-llm-go) | Go optional-interface donor | Secondary scoped donor | Optional operation ports and explicit unsupported states | **Borrow interface shape; validate before dependency use** |
| 15 | [Vercel AI SDK](https://github.com/vercel/ai) | Model-port and middleware donor | Secondary framework donor | Versioned model port, provider metadata, deterministic middleware | **Borrow semantics; reject UI and agent layers** |
| 16 | [OpenAI Python SDK](https://github.com/openai/openai-python) | OpenAI protocol donor | 16 (66.0) | Chat/Responses wire shapes, typed SSE, tool events | **Adopt pinned fixtures** |
| 17 | [Anthropic Python SDK](https://github.com/anthropics/anthropic-sdk-python) | Anthropic protocol/replay donor | 18 (62.0) | Typed Messages events, accumulation, signed/redacted replay | **Adopt pinned fixtures** |
| 18 | [Google GenAI Python](https://github.com/googleapis/python-genai) | Gemini/Vertex protocol donor | 19 (61.5) | Multimodal parts, tools, safety, streaming | **Adopt pinned fixtures** |
| 19 | [OpenTelemetry GenAI conventions](https://github.com/open-telemetry/semantic-conventions-genai) | Interoperability standard | 20 (58.0) | Portable GenAI spans, usage, finish reasons, privacy | **Adopt as standard** |
| 20 | [Gateway API Inference Extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension) | Deployment interoperability donor | Secondary scoped donor | Backend-neutral model/pool identity, endpoint-picker and capability signals | **Borrow concepts through optional adapters** |

This reconciliation retains all eight direct peers, narrows the serving group to five high-value profiles, and replaces three evidence-ranked entries with donors that close more direct contract or deployment gaps. Specifically, `go-openai` (evidence rank 13), Langfuse (14), and TensorRT-LLM (17) remain valuable secondary donors but leave the decision twenty for any-llm-go, Vercel AI SDK, and Gateway API Inference Extension. “Decision rank” expresses Flux implementation priority, not project quality, popularity, or performance.

| Layer | Representative projects | What Flux should learn | What Flux must not import |
|---|---|---|---|
| Data plane | Bifrost, AxonHub, GoModel, Agent Router, Portkey, OmniRoute, Manifest | Transform isolation, per-attempt policy, deadline/recovery, declarative routing | UI, tenancy, MCP hosting, cluster management |
| Serving target | LocalAI, Ollama, llama.cpp, vLLM, SGLang | Runtime/version evidence, endpoint profiles, load/capability signals | Kernels, weights, batching, KV cache, GPU scheduling |
| Contract and port | any-llm-go, Vercel AI SDK, OpenAI, Anthropic, Google GenAI | Optional interfaces, deterministic middleware, typed events, partial JSON, provider replay | SDK objects in `engine`, provider-specific host semantics |
| Evidence and deployment | OpenTelemetry GenAI, Gateway API Inference Extension | Portable telemetry and backend-neutral routing signals | Embedded evaluation UI, Kubernetes types in stable `engine` DTOs |

A wider secondary donor set remains useful: go-openai, Langfuse, TensorRT-LLM, New API’s independent `relaykit` boundary, APISIX and Higress lifecycle ordering, Helicone’s P2C/PeakEWMA strategy, LLM Gateway’s migration/doctor UX, BAML schema descriptors, Pydantic AI test models, and OpenAI Agents stream/usage rules. Each pattern should enter Flux only when it reduces a verified contract or operational gap.

## Flux has a strong core whose promises outrun composition

The source has more than leaf scaffolding. Canonical DTOs are centralized in `llm`, and `provider/core` aliases them rather than maintaining a second wire model (`llm/types.go:49-75`; `provider/core/core.go:35-97`). The lower-level `core.Provider` remains only `Chat`, `StreamChat`, `Ping`, and `Name` (`provider/core/core.go:21-33`). Streaming uses a bounded SSE parser and semantic events, routing has real weighted, least-busy, latency, cost, and usage strategies, and signed route manifests validate revisions, signatures, redirects, and last-good state. The test suite is unusually broad, and the recorded audit passed race tests, vet, lint, formatting, and `govulncheck`; CI additionally covers fuzzing, secrets, dead code, duplication, Markdown, and cross-platform builds (`.github/workflows/ci.yml:34-300`).

The problem is composition. The normal engine path reloads catalog/config and reconstructs adapters, rate limiters, caches, and deployment routers on every request. Selection and execution can therefore observe different state, and the advertised circuit, rate-window, and cache state does not survive a call. This is not a performance footnote: it makes existing reliability features semantically different from their descriptions.

| Area | Verified strength | Current gap or risk | Evidence |
|---|---|---|---|
| Host boundary | Compile-checked `engine` contract and narrow low-level provider port | Public host `llm.Provider` still bundles several maintenance facets; `runtime` docs call an older entry point “recommended” | `docs/architecture/HOST-ENGINE-BOUNDARY.md:33-36`; `llm/provider.go:16-24`; `runtime/runtime.go:1-22` |
| Canonical DTOs | Multimodal input, tools, reasoning, usage, route, warnings, and provider blocks are modeled | Several fields have no producer or are dropped before the host | `llm/types.go:29-47`; `llm/types.go:49-75`; `llm/types.go:219-307` |
| Provider coverage | 28 registry IDs across Anthropic Messages, Gemini, OpenAI Responses, and OpenAI Chat families | Direct and deployment construction can select different protocols; 28 IDs are not 28 independent conformance levels | `catalog/registry/providers.go:16-341`; `provider/provider_registry.go:74-179`; `setup/deployment.go:183-383` |
| Streaming | Bounded parser, semantic deltas, cancellation-aware provider requests | EOF can look successful; tracing and guardrail wrappers drop the source `Close`; split usage and provider state are lost | `provider/core/stream.go:31-89`; `engine/stream.go:82-165`; `provider/observability/tracing.go:91-135`; `provider/resilience/guardrails.go:83-118` |
| Routing | Deployment validation, fallback, breaker filtering, live atomic snapshots | Fallback ignores caller intent, tools can be stripped, actual deployment/attempt is not returned | `router/deployment_router.go:134-247`; `router/deployment_router.go:320-389`; `router/deployment_router.go:554-576` |
| Resilience | Retry, continuation, rate limits, cache, guardrails, health, tracing | Stateful wrappers are rebuilt; deadline classes and admission limits are absent | `engine/engine.go:303-327`; `provider/core/transport.go:10-11`; `provider/core/transport.go:56-63` |
| Budget/cost | Check, usage records, and analytics exist | Check and record are separate; no reservation, operation ID, or idempotent finalization | `provider/observability/budget_provider.go:73-142`; `storage/budgets.go:157-209` |
| Credential safety | Injected stores, sanitized state, atomic config writes | Migration can remove plaintext after partial failure; endpoint and secret-at-rest policy remain weak | `credentials/migrate.go:31-85`; `storage/budgets.go:44-87` |
| Observability | Public OTel wrapper and metrics exist | No normal engine construction path wires the wrapper; attributes are custom/non-current, and the conversation span ends before asynchronous stream work | `provider/observability/tracing.go:12-136`; `internal/observability/genai_semconv.go:21-58`; `conversation/engine.go:166-175` |
| Delivery | Internal HTTP, optional gRPC, and three SDK drafts exist | These are not coherent public products; OpenAPI and SDKs are incomplete and outside root gates | `internal/api/server.go:101-121`; `api/openapi.yaml:118-340`; `internal/sdk/go/go.mod:1-3` |
| Release | `VERSION` is `0.0.1`; the research snapshot verifies a signed tag; CI has strong static checks | No release PR, API diff, module-proxy gate, SBOM/provenance gate, or repeatable `v0.1` policy | `VERSION:1`; `.github/workflows/release.yml:1-31`; [recorded `v0.0.1` release](https://github.com/GrayCodeAI/flux/releases/tag/v0.0.1) |

The highest-risk behavior follows directly from the source. `engine.toClientOptions` does not map canonical `ThinkingEnabled`, although adapters consume it (`engine/convert.go:15-34`). Anthropic response parsing skips redacted thinking and the message builder has no provider-block replay branch (`provider/adapters/anthropic.go:233-300`; `provider/adapters/anthropic.go:303-429`). The engine normalizer does not copy provider blocks (`engine/stream.go:124-165`). Conversation persistence reads usage only on `done`, while real Anthropic and OpenAI processors can emit usage earlier or separately (`conversation/engine.go:266-291`; `provider/core/stream.go:236-281`; `provider/core/stream.go:502-512`). The stable engine continuation path has a default continuation and total-token cap, but still synthesizes `done` when a source closes without a terminal event; the separate conversation path has no hard continuation-call bound (`engine/continuation.go:15-23`; `engine/continuation.go:64-70`; `conversation/engine.go:305-313`).

Routing can silently change semantics. Automatic fallback follows explicit selection, and deployments lacking requested tools remain eligible while `optsForOffering` removes unsupported tools (`router/deployment_router.go:320-389`; `router/deployment_router.go:554-576`). The response route is overwritten with the initially selected route rather than the successful fallback deployment (`engine/convert.go:88-96`; `llm/types.go:235-246`). This undermines billing, audit, debugging, and any future reliability policy that depends on knowing which provider served the request.

Operational controls are similarly incomplete. Budget checks and later records are not one transaction, and stream usage events are independently recorded while errors are ignored (`provider/observability/budget_provider.go:73-142`). The response cache omits response-affecting fields, is not semantically similar despite its name, and returns copies that still alias provider-block and warning slices (`provider/cache/semantic_cache.go:52-138`; `provider/cache/semantic_cache.go:176-199`; `provider/cache/semantic_cache.go:296-339`; `provider/core/copy.go:3-56`). The coalescer ties shared work to the first caller and does not decrement waiter state on every exit (`provider/resilience/coalesce.go:16-48`; `provider/resilience/coalesce.go:113-189`).

The onboarding and delivery story compounds those runtime gaps. A normal engine call requires a prepared catalog cache and otherwise points users to a Rho command (`engine/state.go:86-103`; `catalog/v1.go:615-644`). The strict decoder rejected an additive field observed in the then-current [default catalog](https://langdag.com/model-catalog/v1/catalog.json) on 2026-09-24 (`catalog/v1.go:444-460`). All examples use lower-level `provider` packages rather than the accepted host facade; the streaming example claims auto-continuation but calls the non-continuing method; and the multi-provider example performs manual fallback (`examples/basic/main.go:8-20`; `examples/streaming/main.go:1-33`; `examples/multi-provider/main.go:31-44`). OpenAPI documents 11 paths while the internal server registers 15, omitting `/ready`, `/rerank`, and `/v1/chat/completions` (`api/openapi.yaml:118-340`; `internal/api/server.go:101-121`). The Go SDK is a nested module outside root `go test ./...`, while the Python and TypeScript clients live under `internal` (`internal/sdk/go/go.mod:1-3`; `go.mod:1-41`). The Python delete method expects JSON, matching the current server but not OpenAPI’s declared `204` response (`internal/sdk/python/flux.py:68-75`; `internal/api/server.go:266-273`; `api/openapi.yaml:223-237`).

The honest baseline is therefore **strong primitives, partial end-to-end behavior**. A provider-count-oriented roadmap would hide exactly the defects adopters are most likely to hit. Flux should first reduce semantic edges: options, stream termination, usage, provider replay, fallback requirements, request identity, actual route, and cancellation.

## Correctness and trust should unlock the roadmap

### One runtime snapshot and one stream state machine

The first architectural correction is to make an immutable runtime snapshot the unit of selection and execution. It should contain the compiled catalog, provider configuration, resolved adapters, router, breaker, limiter, cache, and immutable secret references. `Engine` should load once at construction and swap snapshots atomically only after explicit catalog, config, or credential invalidation. Selection and execution must use the same generation, while credential rotation should replace a secret reference without rebuilding unrelated state. This directly fixes the current per-request reconstruction in `engine/engine.go:303-327` and `setup/deployment.go:94-110`.

The second correction is a shared stream coordinator under `engine` and `provider/core`. Every adapter, wrapper, and facade should produce exactly one terminal state: `done`, `error`, or `cancelled`. A transport EOF without a provider terminal event is truncation, not success. Every wrapper must preserve the upstream `Close` callback, use a request-owned child context, stop forwarding on slow-consumer cancellation, and close/drain the body. Split usage should be accumulated into one final aggregate; provider request IDs, finish reasons, route attempts, warnings, and replay blocks should survive that final event. This is the smallest shared fix for several independent high-severity findings.

Agent Router’s stream idle timeout and failover behavior demonstrates why Flux should separate response-header, first-event, first-visible-output, inter-event idle, and total-deadline clocks. ([Agent Router v1.1 notes](https://theagentrouter.ai/release-notes/v1.1)) Flux should add bounded admission separately from deadlines: configurable global, provider, tenant, and queue limits; deterministic queue-full errors; cancellation while waiting; and no provider call for a request rejected at admission. The current ten-minute end-to-end HTTP timeout is too coarse to protect memory, connections, or spend under stalled streams (`provider/core/transport.go:10-63`).

### Provider-native state must be explicit, not best effort

The canonical DTO should represent two kinds of state deliberately: normalized content that hosts can use and opaque provider blocks required for replay. Anthropic signed/redacted thinking, OpenAI reasoning items, and Gemini thought state should remain namespaced, size-bounded, redacted by policy, and protocol-filtered on the next turn. The official Anthropic SDK’s event accumulation and replay helpers are the primary fixture source, not a runtime dependency. ([Anthropic stream helpers](https://github.com/anthropics/anthropic-sdk-python/blob/main/helpers.md))

When a provider does not support a requested field, Flux should transform it only when the transformation is safe and emit a structured warning. If a required capability cannot be preserved, the route should be rejected or the call should fail explicitly. Tools must never be removed merely to make a fallback provider eligible. A universal DTO cannot faithfully represent every future provider extension, so raw passthrough is valuable only as a bounded, versioned, redacted escape hatch after canonical OpenAI and Anthropic behavior is correct.

### Conformance replaces provider-count confidence

Flux already has a `verify` package, but it is not wired into adapters or CI. The suite should become the authoritative contract for auth, endpoint construction, blocking and streaming requests, tools, schema output, reasoning/replay, usage, errors, cancellation, close semantics, and unknown additive fields. Each case must return `PASS`, `UNSUPPORTED`, `FAIL`, `TIMEOUT`, or `NOT_TESTED`; unsupported is a valid result only when the capability matrix says so and the route policy rejects semantic loss.

| Tier | Scope | Deterministic gate | Live evidence |
|---|---|---|---|
| A | Anthropic, OpenAI, Gemini, Azure, Bedrock, Vertex | Pinned official-SDK fixtures plus Flux contract tests | Credential-gated nightly/weekly probes with redacted cassettes |
| B | OpenAI-compatible gateways and specialized adapters | Provider/profile-specific wire fixtures, errors, usage, cancellation | Periodic, budget-capped smoke and last-verified metadata |
| C | User-supplied custom OpenAI endpoints | Generic wire subset only | Host-owned; Flux reports unsupported fields rather than claiming parity |
| Local | Ollama, llama.cpp, vLLM, SGLang, LocalAI | Pinned profile fixtures and discovery parsing | Version-pinned endpoint probes; never provider API calls |

Operation support should be recorded as an `operation × provider/profile × model × runtime/version × configuration` tuple with source, confidence, observation time, and expiry. A generic “OpenAI compatible” or non-local provider boolean is not sufficient. LocalAI’s discovery document, llama.cpp’s `/props`, Ollama’s tags/show endpoints, and KTransformers’ qualified support matrix all demonstrate why support evidence must preserve model, runtime, hardware, and configuration qualifiers. ([LocalAI discovery](https://localai.io/docs/features/api-discovery/index.html); [llama.cpp server](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md); [Ollama API](https://github.com/ollama/ollama/blob/main/docs/api.md); [KTransformers matrix](https://ktransformers.net/en/docs/support-matrix))

### Security must fail closed at host boundaries

The embedded API is internal, so it should not become a hidden public gateway by accident. `NewServer` does not validate dependencies or safe transport posture, `ServeHTTP` bypasses the listener’s bind check, and plain HTTP is accepted on a non-loopback address when an API key exists (`internal/api/server.go:50-85`; `internal/httputil/httputil.go:88-103`). Flux should require a validated constructor, refuse non-loopback plain HTTP unless an explicit trusted-proxy/TLS mode is configured, attach authorization and tenant scope to every sensitive route, and make readiness test real provider/configuration readiness.

Credential migration must be transactional: enumerate all recognized values, write and verify every destination, preserve unknown or failed values in a permission-restricted source/backup, and write a success marker only after complete success. SQLite main, WAL, and SHM files must be hardened under a permissive umask. Provider keys should be stored as references or protected secrets rather than plaintext budget rows. Custom endpoint validation must cover scheme, redirect, resolved address, DNS rebinding, port, and private/local-network policy at connection time, with explicit operator opt-in for local endpoints.

Budgets need reserve → execute → idempotently finalize semantics. A reservation must atomically prevent concurrent overspend; finalization must use a Flux operation ID, aggregate split usage once, reconcile cancellation and partial cost, and surface failed ledger writes as retryable operational events. Cache and coalescer identity must include tenant, provider, deployment, every response-affecting message part, tool/schema definition and choice, response format, reasoning controls, stop conditions, sampling controls, and provider options. Returned metadata must be deeply isolated.

OpenTelemetry GenAI conventions should be adopted through the OTel API, with content capture off by default and compatibility aliases only during a documented migration. Flux should use an in-memory exporter test to prove provider/model identity, operation, tokens, finish reason, route attempts, error class, and actual stream termination. ([OpenTelemetry GenAI spans](https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/gen-ai-spans.md))

## Turn the landscape into explicit build decisions

### Build, adopt, borrow, defer, and reject

| Decision | Flux work or standard | Primary donors | Reason |
|---|---|---|---|
| **Build** | Shared stream terminal/cancellation coordinator | Agent Router, Bifrost, OpenAI/Anthropic SDKs | Fixes cross-layer correctness rather than one adapter |
| **Build** | Lossless canonical state and warnings | Anthropic, OpenAI, Google GenAI, Vercel AI SDK | Makes declared DTO fields real across two turns |
| **Build** | Immutable runtime snapshot and atomic refresh | Bifrost, GoModel, LiteLLM | Makes stateful resilience true and selection deterministic |
| **Build** | Fresh bootstrap and catalog trust pipeline | Flux’s signed-manifest controls | Removes Rho dependency and mutable-source startup failure |
| **Build** | One provider descriptor/factory and 28-ID parity tests | AxonHub, New API `relaykit`, any-llm-go | Eliminates entry-point protocol divergence |
| **Build** | Conformance v1, capability evidence, endpoint profiles | Bifrost matrix, LocalAI discovery, official SDKs | Replaces nominal provider count with reproducible evidence |
| **Build** | Atomic budgets, exact cache identity, safe migration | LiteLLM spend semantics, secure state patterns | Protects money, secrets, and tenant isolation |
| **Adopt** | OpenTelemetry GenAI semantics and privacy defaults | OpenTelemetry, LiteLLM OTel v2 | Portable, current, and less API surface than a custom sink |
| **Adopt** | Official-SDK pinned wire fixtures | OpenAI, Anthropic, Google | Faster and more trustworthy than handwritten assumptions |
| **Adopt** | Go API diff, module-proxy smoke, SBOM/provenance/attestation | Go and GitHub release tooling | Creates a reproducible supply-chain baseline |
| **Adopt** | AIPerf-style methodology and machine-readable results | AIPerf | Measures client overhead without fake GPU/provider ranking |
| **Borrow** | Transformer pipeline layering | AxonHub | Isolates inbound, canonical, outbound, and reverse transforms |
| **Borrow** | Per-attempt auth/budget/capability revalidation | LiteLLM | Prevents fallback policy bypass |
| **Borrow** | Raw-response/debug inspection with redaction | Bifrost | Exposes translation loss without leaking raw secrets |
| **Borrow** | Request-attached policy and sticky affinity | Portkey | Makes route intent explicit after deterministic baselines exist |
| **Borrow** | Idle timeout, per-request credentials, provider extensions | Agent Router | Improves stream recovery and compatibility |
| **Borrow** | Deterministic `transformParams`/wrap generate/wrap stream seam | Vercel AI SDK | Adds extension without agent/plugin sprawl |
| **Borrow** | Optional operation interfaces and explicit unsupported states | any-llm-go | Keeps `core.Provider` narrow |
| **Borrow** | Schema descriptor plus validation/repair metadata | BAML, Pydantic AI | Makes structured output explicit without owning a prompt lab |
| **Borrow** | Backend-neutral endpoint signals | Gateway API Inference Extension | Adds pool/load/capability hints without Kubernetes DTOs |
| **Defer** | Default semantic cache and semantic routing | LiteLLM, Bifrost, Helicone | Exact identity and cost/latency baselines are not yet trustworthy |
| **Defer** | P2C/PeakEWMA and sticky affinity | Helicone, Portkey | Useful but lower priority until persistent snapshots and tenant scope exist |
| **Defer** | Public model-call middleware and custom selectors | Vercel AI SDK, current Flux seams | Lifecycle defects would be multiplied by extension points |
| **Defer** | Responses/raw passthrough and more endpoint breadth | OpenAI SDK, LiteLLM, Agent Router | Add translation paths before canonical losslessness is proven |
| **Defer** | Public HTTP, gRPC, and SDK extraction | Portkey deployment portability | Valuable only as separately versioned, tested modules |
| **Defer** | Evaluation/guardrail ports and model administration | Langfuse, DSPy, Ollama, Triton | Host or admin concerns; Flux should export evidence, not products |
| **Defer** | Wasm or unrestricted plugins | Higress, APISIX | Sandbox, ABI, memory, signing, and credential risks exceed current need |
| **Reject from core** | UI, agent runtime, tools execution, RAG, memory, sessions | Dify, Open WebUI, LangGraph, Pydantic AI | Direct host duplication and dependency-direction violation |
| **Reject from core** | Tenancy, virtual organizations, billing, admin control plane | LiteLLM, Kong, New API | Platform concerns and mandatory state/operations |
| **Reject from core** | Model execution, weights, batching, KV cache, GPU scheduling | vLLM, SGLang, llama.cpp, TensorRT-LLM | Wrong architectural layer |
| **Reject from core** | General API-gateway plugin ecosystem or MCP process host | Kong, APISIX, Higress, Bifrost | Broad attack surface and product identity drift |
| **Reject from core** | Mandatory PostgreSQL, Redis, Kubernetes, or distributed cache | LiteLLM, Agent Router, KServe | Defeats embeddability and host neutrality |

Bifrost’s 2026 MCP registration vulnerability is a useful boundary lesson: merging management and execution planes expands the consequences of an unauthenticated control surface. ([CVE analysis](https://research.jfrog.com/vulnerabilities/bifrost-is-vulnerable-to-unauthenticated-remote-code-execution-via-mcp-stdio-client-registration-cve-2026-90898/)) Flux should not host command-spawning MCP, unrestricted process plugins, or an ambient management listener merely to match feature lists.

### Provider and protocol gaps should be closed by importance

| Provider/protocol concern | Current Flux state | Required direction |
|---|---|---|
| Provider identity and factory | 28 IDs, four declared families, but multiple constructor switches | One descriptor, canonical aliases, one resolver, parity test for every ID |
| Gemini | Native client exists, but direct `gemini` does not select it; Vertex paths diverge | Make `ProviderSpec` and factory authoritative; test native and compatible profiles |
| Vertex | Direct path is Anthropic-on-Vertex while deployment path is native Gemini | Declare distinct profiles explicitly; never select by entry point |
| Concentrate | Deployment uses Responses while direct path uses generic OpenAI | Use the declared Responses factory everywhere or declare two provider IDs |
| Ollama | Thin OpenAI wrapper | Add read-only native tags/show/profile metadata; no model pull/load in `engine` |
| OpenAI Chat | Text/tool builder exists; system/developer and JSON-schema descriptor need conformance | Typed translator with pinned official fixtures and no silently ignored accepted fields |
| Anthropic Messages | Native adapter exists; signed/redacted replay and public facade do not | Preserve blocks, events, usage split, finish reasons, and multi-turn replay |
| Gemini/Vertex | Native multimodal/tool support exists; thinking and URL semantics are partial | Pin current API fixtures and preserve provider-native state |
| Embeddings/batch/rerank/media/moderation | Separate leaf interfaces/clients | Keep optional ports; add independent conformance only when demand warrants it |
| Provider count | Nominal gateway IDs | Publish operation/profile/model/version support tiers and last-verified dates |

Protocol work should use pinned official fixtures and opt-in live probes. Current documentation is evidence of intended semantics, but no live provider credentials were available in the audit. The OpenAI JSON-schema descriptor, Gemini image URL handling, current provider SDK shapes, and release boundaries must therefore be validated before changing wire behavior.

### API and OSS maturity need one truth

| Surface | Current state | Recommendation |
|---|---|---|
| `engine` | Accepted Go host facade with `Generate`/`Stream` | Stabilize the existing minimal generation path before adding breadth |
| Root `flux` | Not yet the primary API | Consider a small root `New/Generate/Stream/Close` facade for `v0.1`, preserving `engine` during migration |
| `llm.Provider` | Broad composition facet | Separate generation from optional controller/maintenance interfaces |
| Internal HTTP | Useful local component; weak secure defaults and lossy conversation path | Keep internal or extract only after typed translation and fail-closed construction |
| OpenAPI | Hand-maintained, incomplete relative to routes | Validate against a public handler or remove from the core promise |
| gRPC | Tagged, skeletal, optional | Keep experimental; do not market until generated clients/protobuf and compatibility exist |
| Go SDK | Nested module, excluded from root CI | Extract as independent module only if delivery is a validated use case |
| Python/TypeScript SDKs | Internal stubs outside root CI | Remove from stable claims or publish tested, separately versioned modules |
| Examples | Lower-level and partly mislabeled | Rewrite against stable API and compile/run them as external-package tests |
| Catalog publication | Remote mutable artifact outside repository governance | Add in-repo generation, schema compatibility, publisher identity, digest/signature, last-good cache |
| Release | Tag-triggered GitHub release notes | Add release PR, API diff, clean consumer, module proxy, SBOM/provenance, verified tag |
| Security operations | CI scanning exists; repository alerts/push protection were disabled in the audit snapshot ([repository security snapshot](https://api.github.com/repos/GrayCodeAI/flux)) | Enable native alerts/secret push protection and own support/security policy in Flux |

The repository should publish a stability inventory: **stable** for the minimal host generation contract, **advanced** for deliberate extension interfaces, and **experimental/internal** for delivery, distributed routing, and broad package surfaces. No `v1.0` claim should precede an import census, `apidiff` baseline, migration guide, support window, and external-consumer test.

## Sequence hardening into a credible release

The backlog, phase gates, versioning targets, and 90-day sequence below are **proposed acceptance criteria and engineering recommendations**, not implemented capabilities, dated commitments, or validated demand estimates.

### Prioritized backlog

| Priority | Workstream | Minimum acceptance evidence |
|---|---|---|
| P0 | Central stream terminal/cancellation coordinator | Exactly one terminal event; truncated EOF fails; every `Close` cancels source/body; race tests show no forwarding leak |
| P0 | Lossless canonical options, stream state, provider replay, route, and warnings | Every documented option has conversion tests; signed/redacted/provider state round-trips over two turns |
| P0 | Immutable runtime snapshot and explicit refresh | One state load per generation; limiter/cache/breaker survive calls; selection and execution share a snapshot |
| P0 | Fresh-state bootstrap and catalog trust | Clean temporary home runs mock-backed `New → Generate/Stream`; current artifact validates; last-good recovery works |
| P0 | One provider descriptor and factory | All 28 IDs resolve identically through direct/deployment paths; Gemini, Vertex, and Concentrate are regressions |
| P0 | Transactional credential migration and secret/storage hardening | Failed or partial writes never remove source; sidecar modes and endpoint policy are tested |
| P0 | Atomic budget reserve/finalize | Concurrent calls cannot overspend; duplicate operation IDs are idempotent; cancellation reconciliation is durable |
| P0 | Canonical cache/coalescer identity | Every response-affecting field and scope changes identity; returned state is isolated; “exact” is named truthfully |
| P0 | Admission, lifecycle deadlines, fallback policy, actual route attribution | Bounded queue; distinct clocks; fallback opt-out; required tools reject rather than strip; actual attempt returned |
| P0 | Shared conformance v1 | Every built-in adapter runs shared request/stream/error/usage/cancel cases or explicit unsupported rows |
| P0 | Truthful `v0.1` release and docs | Clean external module consumes the candidate; examples run; provider tables generate; API diff and release gates pass |
| P1 | Full OpenAI Chat Completions translator | Multimodal messages, tools/results, schema, options, usage, stream options, and errors pass official fixtures |
| P1 | Anthropic Messages facade and replay | Thinking, tools, split usage, stop reasons, errors, and two-turn replay pass official fixtures |
| P1 | Operation/profile/model capability evidence | Support is qualified by runtime/version/config with source, confidence, freshness, and expiry |
| P1 | Read-only endpoint profiles | Ollama, llama.cpp, vLLM, SGLang, and LocalAI metadata imports are bounded, redacted, and versioned |
| P1 | Current privacy-safe OTel GenAI contract | In-memory exporter verifies attributes, stream completion, and content-off defaults |
| P1 | Performance, soak, and live-provider evidence | Versioned local results and nightly `PASS/UNSUPPORTED/FAIL/TIMEOUT/NOT_TESTED` reports |
| P1 | Public selector and narrow model-call middleware | Deterministic order; generate/stream wrappers preserve terminal, cancel, usage, and route state |
| P1 | Signed-manifest production proof | TLS, separate process, peer outage, stale revision, restart, persisted last-good, and drain; otherwise remain experimental |
| P2 | Optional operation ports | Embeddings, batch, rerank, media, and moderation have independent interfaces/suites; `core.Provider` stays four methods |
| P2 | Migration/compatibility diagnostics | Report effective route and every transformed, dropped, unsupported, or required-host field |
| P2 | Responses/raw passthrough | Explicit, versioned, size-bounded, redacted; canonical mode warns on every lossy conversion |
| P2 | Genuinely adoptable delivery modules | Any retained HTTP/gRPC/SDK is public, independently versioned, route/schema tested, and consumer-tested |
| P3 | Evaluation and semantic-guardrail ports | Privacy-safe fixture replay and typed callbacks; no dataset, optimizer, policy catalog, or UI |
| P3 | Separate model-server administration | Explicit admin credentials and operations; never imported by stable `engine` |
| P3 | Wasm/edge adapter only after a proven gap | Documented use case, ABI, sandbox, limits, signing, rollback, and no ambient credentials |

### Phased delivery gates

| Phase | Objective | Exit gate |
|---|---|---|
| 1. Hardening and truth | Make current claims true and establish one canonical contract | Fresh quickstart; P0 race tests; 28-ID factory parity; conformance v1; docs/API/release truth; clean candidate consumer |
| 2. Protocol completeness | Make OpenAI Chat and Anthropic Messages trustworthy | Official-SDK fixtures pass; no flattened history or silently ignored accepted fields; replay and unknown-event semantics explicit |
| 3. Production reliability | Prove behavior under load, failure, cancellation, and restart | Admission/deadlines, budget/cache concurrency, route/health semantics, benchmarks, soak, and signed-manifest proof or experimental label |
| 4. Interoperability | Support hosted and self-hosted profiles with portable evidence | Versioned capability reports, local endpoint profiles, OTel conformance, optional backend-neutral signals, migration diagnostics |
| 5. Ecosystem and adoption | Publish one coherent library and validate external demand | Truthful `v0.1`, stability labels, automated release/security gates, extracted optional modules, at least one independent Go host |

### First 90 days: sequence, not a staffing promise

The repository evidence does not establish maintainer capacity, so this is an outcome-oriented 90-day sequence rather than a delivery commitment. Scope should be reduced before dates move.

| Window | Primary outcome | Exit evidence |
|---|---|---|
| Days 0-30 | Reproduce and freeze the P0 contract: stream terminal model, option/state matrix, fresh bootstrap, factory table, migration failure cases, budget concurrency tests | Failing tests become explicit acceptance gates; no new provider or endpoint breadth enters the branch |
| Days 31-60 | Implement runtime snapshot, stream coordinator, provider blocks/usage aggregation, fallback policy/actual route, cache identity, budgets, and conformance v1 | Two-request state tests, blocked-provider cancellation, concurrent budget/cache tests, and all built-in adapter fixture results pass under race |
| Days 61-90 | Complete OpenAI translator, Anthropic replay, current OTel semantics, truthful docs/examples/OpenAPI scope, release automation, and clean consumer test | Candidate can be installed from a clean external module; API diff approved; release/security evidence generated; unresolved features explicitly experimental |

If the P0 exit criteria are not met by day 90, the correct action is to delay `v0.1.0`, not pull P1 work into the release. A narrow pre-1.0 release with explicit limitations is more valuable than a feature-rich release whose claims are not reproducible.

### Versioning and `v1.0` gates

`v0.1.0` should be a **library-foundation release**: fresh onboarding, stable core generation contract, persistent runtime state, canonical stream behavior, verified provider construction, and truthful package/API scope. It should not imply a complete gateway, SDK suite, distributed control plane, or full endpoint catalog.

`v0.2.x` should add evidence-backed extension: provider plugin descriptors, public selector/middleware seams where justified, Tier-A conformance, privacy-safe OTel, endpoint profiles, reproducible benchmarks, and optional separately versioned delivery modules. Security alerts, dependency automation, API diffs, release PRs, module-proxy checks, SBOM/provenance, and verified tags should be established by this stage.

`v1.0` requires at least one compatibility baseline and external-consumer history; a documented Flux-owned support/security policy; reproducible release from a clean clone; no unresolved incompatibility in the supported contract; a migration guide from Eyrie and the current pre-1.0 API; provider support tiers with last-verified dates; and a credible maintenance/succession path. The current `v0.0.1` history does not establish those conditions.

### Principal risks and guardrails

| Risk | Early warning | Guardrail |
|---|---|---|
| Universal DTO remains inherently lossy | More provider-specific fields enter the stable DTO or raw vendor objects leak into `engine` | Canonical normalized state plus bounded, namespaced replay blocks and explicit warnings |
| Correctness fixes become an uncontrolled API break | `engine.ContractVersion` changes without migration notes | Inventory public imports, add `apidiff`, publish migration guide, keep pre-1.0 scope narrow |
| Provider-count pressure resumes | New registry IDs land before factory/conformance changes | Freeze nominal additions until every registered ID has one factory and a conformance row |
| Middleware amplifies lifecycle bugs | Public hooks appear before terminal/cancel coordinator | No public middleware until wrappers have shared conformance and ownership |
| Exact semantics are mislabeled as semantic AI | Similarity or model-name heuristics become default | Ship exact cache/routing first; require measured opt-in semantic features |
| Serving scope expands into model infrastructure | Import of vLLM/SGLang/Triton internals or model pull/load appears in `engine` | Read-only profiles only; administration in a separate module |
| Catalog publication becomes a supply-chain dependency | Mutable external artifact lacks identity, digest, or compatibility test | Generator, signed/digested release artifact, tolerant additive schema, last-good cache |
| Distribution/gateway scope expands the project | HTTP/gRPC/SDKs appear in the stable promise without independent ownership | Extract and version separately or remove from core claims |
| One-maintainer capacity invalidates roadmap dates | P0 work grows or bypasses race/conformance gates | Sequence gates, reduced scope, design partners, explicit defer decisions |
| Benchmark claims distort positioning | Vendor throughput or stars become Flux quality evidence | Publish client-only methodology and distinguish upstream claims from Flux measurements |

### Non-goals and unresolved evidence

The following are explicit non-goals for Flux core: UI or playground; chat UX; agent execution; tool execution; sessions and memory; RAG; workflow/durable orchestration; tenancy, organizations, virtual teams, billing, or admin control planes; model weights, pull/load/delete, compilation, batching, KV cache, or GPU scheduling; Kubernetes CRDs; a general API gateway; MCP hosting; a mandatory database/cache; unrestricted plugins or Wasm; and a feature-count race with LiteLLM.

Several evidence gaps must remain visible. No live provider calls or independent common-harness benchmark were run, so current hosted compatibility, model availability, regional behavior, and latency remain unverified. The mutable default catalog may have changed since 2026-09-24 and must be revalidated. The official provider SDKs establish wire references, not proof that every Flux field is implemented. Production call volume, slow-consumer behavior, restart evidence, TLS/process manifest operation, partition behavior, secret-store latency, and downstream adoption are not established. Upstream release labels and component boundaries can move quickly, especially for Portkey, Bifrost, rolling llama.cpp/TensorRT tags, and newer GenAI conventions. The public API proposal, roadmap sequence, and support tiers are engineering interpretations until validated with independent Go hosts and design partners.

## Conclusion

The landscape does not show that Flux needs more breadth; it shows why a narrower promise is stronger. Flux can own the difficult seam between heterogeneous providers and host applications, but only if the same contract governs options, provider replay, streaming, usage, fallback, route attribution, and cancellation. The winning release is not a miniature gateway product—it is a small runtime whose claims survive fresh installation, concurrent calls, provider failure, and two-turn protocol replay.

The next strategic move is to make the repository’s strongest abstraction real end to end. A persisted runtime snapshot, one stream state machine, one provider factory, one conformance corpus, and one truthful release contract would convert Flux from a broad collection of provider capabilities into a trustworthy interoperability layer. Everything beyond that—agents, serving, tenancy, control planes, and product UI—should remain outside the core until external demand proves that a separate module is justified.
