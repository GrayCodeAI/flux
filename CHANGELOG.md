# Changelog

All notable changes to graycode-router are documented here.  
Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) · Versioning: [SemVer](https://semver.org/)

---

## [0.0.1] — 2026-09-04

### Changed — Renamed eyrie to graycode-router
- **Module is now `github.com/GrayCodeAI/graycode-router`.** All `Eyrie*`
  types are `GraycodeRouter*`, `EYRIE_*` env vars are `GRAYCODE_ROUTER_*`,
  and config lives under `~/.graycode-router`. Breaking rename with no
  behavior change; prior history is preserved below.

## [Unreleased]

### Changed — Shared MiMo auth-retry helper (2026-08-16)
- **Deduplicated `doRequestWithMimoAuthRetry`** between the OpenAI and
  Anthropic adapters into one `doWithMimoAuthRetry` helper (client/adapters,
  next to `mimoAuthHeaders`); the two adapters now differ only in the Bearer
  headers they apply to the 401 retry. No behavior change.

### Fixed — Gemini stream request IDs (2026-08-16)
- **Gemini `StreamChat` now propagates the provider request ID.** The client
  captured `X-Goog-Request-Id` from the response headers but passed an empty
  string to the stream result, so hosts lost the correlation ID on
  successful streams (it was only preserved on errors). Both the shared
  parser path and the legacy opt-out parser now carry it.

### Fixed — Non-fatal stream diagnostics no longer fail the stream (2026-08-16)
- **Stream health diagnostics are now warnings, not terminal errors.**
  `client/core`'s OpenAI stream processor emits end-of-stream diagnostics
  (reasoning-only responses, empty responses) as error-type events followed
  by the terminal `done` — but the engine mapped *every* error event to
  `provider_unavailable`, stopped forwarding, and set `Err()` even though
  content had been delivered. Diagnostic events are now marked non-fatal via
  the existing `GraycodeRouterStreamEvent.Warning` field (additive); the engine
  forwards them as `warning` events and still delivers the final
  `done`/usage event with `Err()` unset. Genuinely fatal stream errors keep
  the previous behavior. The deprecated client continuation helper and the
  tracing middleware treat warning-marked events the same way.

### Fixed — Concentrate adapter robustness (2026-08-16)
- **Concentrate Responses client now uses the shared pooled HTTP client**
  (`core.NewPooledHTTPClient(core.DefaultTimeout)`) instead of a private
  `&http.Client{Timeout: 120s}` literal — long streams are no longer cut off
  at 2 minutes and connections reuse the process-wide transport pool like
  every other adapter.
- **Concentrate requests are retried via `core.DoWithRetry`** (chat and
  stream paths) on 429/500/502/503/529 with backoff and `Retry-After`
  support; `SetRetry` previously discarded the config with a comment claiming
  the HTTP client handled retries (it never does).
- **Concentrate errors are structured `*core.GraycodeRouterError`s** built by
  `core.ParseProviderError`/`core.FormatAPIError` (8KB bounded read,
  provider/op/status/request-ID preserved), so `IsRetriable()`/`IsAuthError()`
  and the engine's error classification work; the captured `X-Request-Id` is
  also propagated to stream results.
- **`normalizeToolParams` no longer mutates the caller's tool schema map** —
  a shallow copy gets `additionalProperties:false` injected for strict mode.

### Added — Round 3 ecosystem improvements (2026-06-06)
- **Reasoning controls** — `reasoning_effort` and Anthropic extended-thinking
  `thinking_budget_tokens` passthrough on `ChatOptions` (omitted when unset).
- **GitHub OIDC keyless CI auth** — mints a short-lived OIDC token in GitHub
  Actions and exchanges it for AWS Bedrock (STS `AssumeRoleWithWebIdentity`) or
  GCP Vertex (Workload Identity Federation) credentials, no stored secrets.
- **OpenAI-compatible proxy** — `POST /v1/chat/completions` endpoint so existing
  OpenAI SDK clients can talk to graycode-router unchanged.
- **Named load-balancing strategies** — `simple-shuffle`, `least-busy`,
  `latency-based`, `cost-based`, and `usage-based` alongside the default
  weighted router.
- **Pluggable cache backend** — distributed `CacheBackend` interface
  (in-memory default, RESP/Redis-capable, stdlib-only).
- **Audit log sink** — pluggable `AuditSink` interface with a no-op default and
  a JSONL file sink recording privacy-preserving call metadata.
- **Model role slots** — named `primary` / `weak` / `editor` slots with
  fallback to primary, plus an LLM summarizing condenser for long histories.
- **`/rerank` endpoint** — provider-backed reranking with a lexical fallback.
- **`/ready` readiness probe** — alongside the existing `/health` check.
- **gRPC skeleton** — dependency-free gRPC API skeleton behind the `grpc`
  build tag, ready to wire when generated stubs are available.

### Changed
- **Version re-baselined to `0.1.0`** in `graycode-router.go` (`const Version`) and
  `client/client.go` (`var Version`, used in the `User-Agent` header).

### Added — Round 2 ecosystem improvements (2026-06-01)
- **`internal/shrink`** package: tool-description shrink for LLM tool
  definitions before they are sent to the provider.
  Compresses `[]types.Tool` descriptions before they are sent to
  the provider. Stages:
  1. Auto-clarity safety check (security/destructive keywords pass
     through verbatim)
  2. Dictionary substitution (long phrases → short equivalents)
  3. Drop-list (articles, filler, second-person pronouns)
  4. 200-char length cap
  5. Whitespace collapse
  Reports aggregate `BytesSaved` and `PercentOff` across all tools
  in the slice. Safe to call concurrently.
  Aligns graycode-router with the rest of the hawk-eco ecosystem (`hawk`, `shrike`,
  `harrier`, `kestrel`, `merlin`).

### Added
- Output guardrails framework (PII, secrets, injection, harmful content)
- Request coalescing for identical concurrent LLM requests
- Lifecycle callback hooks (8 events)
- Structured output validation with retry-on-failure

### Added — Production Hardening (top-50 OSS parity)
- Same-style hardening pass already on this branch:
  strict `golangci-lint` v2 config, unchecked-error fixes across
  `observability.go`, `sdk/go/client.go`, `storage/dag.go`,
  `storage/sqlite.go`, dead-code removal, and gofmt cleanup of the
  residual blank-line drift in `client/client.go`.
- `CONTRIBUTING.md` — development setup, branch flow, conventional
  commits, test/lint requirements.
- `CODE_OF_CONDUCT.md` — Contributor Covenant 2.1.
- `.gitattributes` — LF line-ending normalization, binary detection,
  GitHub linguist hints.
- `.github/PULL_REQUEST_TEMPLATE.md` — Summary / Changes / Testing /
  Checklist.
- `.github/ISSUE_TEMPLATE/bug_report.yml` — structured bug report.
- `.github/ISSUE_TEMPLATE/feature_request.yml` — feature request with
  developer fit checks.
- `.github/ISSUE_TEMPLATE/config.yml` — routes security reports to
  GitHub Security Advisories, questions to Discussions, blocks blank
  issues.

### Fixed (2026-05-16)
- Flaky test `TestOpenAIStreamChat_MultipleToolCalls` order assertion ([f5e7f3b](https://github.com/GrayCodeAI/graycode-router/commit/f5e7f3b21d092886971b092027a8184c6fd8a385))
- gofumpt formatting ([7de54e3](https://github.com/GrayCodeAI/graycode-router/commit/7de54e352170e456bbfe9bfee9b79e244210db72))
- gofumpt formatting + `go mod tidy` ([8bd58dc](https://github.com/GrayCodeAI/graycode-router/commit/8bd58dcea346eb9df8d13d9020d936df2ceba1b5))
- Remaining errcheck issues ([a0a746a](https://github.com/GrayCodeAI/graycode-router/commit/a0a746aa95cd9acdf5d29ef836985e13ab864ba1))
- Resolve all lint errors (bodyclose, errcheck, gocritic, noctx, unlambda) ([566980f](https://github.com/GrayCodeAI/graycode-router/commit/566980f39bdd7ebba072ce4c17bc19bc0f3cf2a0))
- Syntax errors in test files ([51809bc](https://github.com/GrayCodeAI/graycode-router/commit/51809bcb181a27eb344a1493501f59c593b3065b))
- Upgrade Go from 1.26.1 to 1.26.3 to patch stdlib vulnerabilities ([f4a6594](https://github.com/GrayCodeAI/graycode-router/commit/f4a65944a6f777947071c9b870a6f2c4f74891be))

### Tests (2026-05-16)
- Fix flaky `TestBackoffDelay` by accounting for jitter ([5fa573f](https://github.com/GrayCodeAI/graycode-router/commit/5fa573fe3ec8074a84bf81d10ea59837bf40b7bc))

## [0.1.0] — 2026-05-12

> Initial release.

### Added

**Clients**
- `Provider` interface — `Chat`, `StreamChat`, `Ping`, `Name` — composable and mockable
- `AnthropicClient` — Anthropic Messages API with full content block support
- `OpenAIClient` — OpenAI and all OpenAI-compatible providers
- `MockProvider` — testing without API keys (echo / fixed / tool_use / error / max_tokens modes)
- `GraycodeRouterClient` — thread-safe universal client with cached provider instances

**Streaming**
- SSE parser for Anthropic and OpenAI formats
- Tool call streaming — `input_json_delta` accumulation (Anthropic), index-based accumulation (OpenAI)
- Thinking block streaming (`thinking_delta`)
- `StreamResult` with `Close()` — no goroutine leaks
- Buffered channels (64) with goroutine-owned close

**Reliability**
- Retry — exponential backoff, full jitter, `Retry-After` header support
- `ChatWithContinuation` — auto-retry on `max_tokens` (up to 3 continuations)
- `WithRateLimit` — token bucket rate limiter decorator
- Fallback provider chains with automatic failover on retriable errors
- Provider health checking and success/failure stat tracking

**Caching**
- `AddCacheBreakpoints` — Anthropic `cache_control` on system prompt and conversation prefix
- Semantic caching for repeated queries
- Cache analytics (hit/miss stats)

**Catalog**
- Embedded model catalog — pricing, context windows, max output for 8 providers
- Live fetch from OpenRouter and CanopyWave
- Model tier resolution — opus / sonnet / haiku → concrete model IDs
- Model name canonicalization and marketing display names
- Deprecation warnings per provider

**Config**
- Provider detection from env vars (priority order)
- `~/.hawk/provider.json` config file I/O
- `ApplyProviderConfigToEnv` — applies config to `os.Environ`
- OpenAI-compatible runtime resolution
- Provider profile management

**Advanced**
- Call metrics collector for provider usage tracking
- Role merge utility for message optimization
- Dynamic provider registration with registry freezing
- Weighted provider selection for load balancing
- Batch API support (Anthropic Message Batches)
- Cost estimator for pre-call cost prediction
- Embedding support

**Quality**
- `User-Agent: graycode-router/0.1.0` on all HTTP requests
- Request ID captured from response headers
- 4 KB error body cap — prevents OOM on large error responses
- `"graycode-router: "` prefix on all errors with `%w` wrapping
- Zero external dependencies · Go 1.26
