# Audit remediation plan

Source: the 2026-09-20 full-code audit (14 subsystem reviews, each adversarially
verified: 194 verdicts, 135 confirmed, 59 partially confirmed, 0 refuted) plus
7 research reports (Go competitors, gateways, routing papers, cache/resilience
papers, provider API currency, catalog/pricing, OTel).

Branch: `fix/audit-remediation`, stacked on `refactor/feature-oriented-monorepo`
(unmerged commit `1206534`). The fixes depend on that commit's `client/` →
`provider/` rename, so branching from `main` was not possible.

## Ground rules

1. Every fix ships with a test that **fails on the old code and passes on the new**.
   Where the audit executed a repro, that repro becomes the regression test.
2. Tests for wire formats use fixtures copied from **vendor documentation**, with
   the source URL in a comment. They are labelled `source: vendor-docs`. They are
   **not** recorded traffic: no provider API keys are available to this work, and
   no fixture may be described as recorded unless it was.
3. No `Co-authored-by` trailers (AGENTS.md; githook strips them).
4. Conventional Commits. No push, no PR from the implementation agents.
5. Do not touch the AGENTS.md "do not touch" list: `Provider` method set,
   `FluxMessage`/`FluxResponse`/`ChatOptions` field **names**, `FluxError`,
   `FluxConfig`. Additive fields are allowed and already landed (below).
6. Rho must keep building where it built at baseline. See "Cross-repo gate".

## Contracts landed in Wave 0 (binding for all work packages)

| Addition | Where | Purpose |
|---|---|---|
| `ToolCall.RawArguments`, `ToolCall.ProviderMetadata` | `tools/tool.go` | exact provider JSON; opaque per-call state (Gemini `thoughtSignature`) |
| `ProviderBlock`, `FluxMessage.ProviderBlocks`, `FluxResponse.ProviderBlocks` | `llm/types.go` | round-trip signed thinking, `redacted_thinking`, OpenAI reasoning items |
| `FluxStreamEvent.ProviderBlock`, `engine.EventProviderBlock` | `llm`, `engine` | deliver a completed opaque block on a stream |
| `CallWarning`, `FluxResponse.Warnings` | `llm/types.go` | report ignored/adjusted settings |
| `StreamErrorInfo`, `FluxStreamEvent.ErrorInfo`, `ErrKind*` | `llm/types.go` | typed stream errors instead of stringified ones |
| `ResolvedRoute.DeploymentID`, `.Attempts` | `llm/types.go` | which deployment actually served the request |
| `FluxUsage` semantics documented | `llm/types.go` | see decision D1 |

### Binding decisions

- **D1 Usage.** `PromptTokens` is the **total** input including cache reads and
  cache creation for every provider. `CacheReadTokens` and `CacheCreationTokens`
  are subsets. Anthropic adapters add cached tokens into `PromptTokens`.
- **D2 Provider blocks.** An adapter replays only blocks whose `Provider` matches
  its own protocol and silently skips the rest. Hosts never edit a block.
- **D3 Finish reasons.** Not normalised (Rho may compare raw values). Consumers
  that need "hit the limit" accept both `max_tokens` and `length`.
- **D4 Tool arguments.** `Arguments` stays. When the provider's JSON is malformed,
  `Arguments` is nil, `RawArguments` holds the bytes, and the legacy
  `Arguments["_raw"]` key is still set for one release, documented as deprecated.
- **D5 Error kinds.** Adapters/router set `ErrorInfo` on every stream `error`
  event. Engine classification prefers `ErrorInfo.Kind` over string matching.
- **D6 Circuit breaker.** Only transport errors, 5xx and 529 count as failures.
  Never `context.Canceled`/caller `DeadlineExceeded`, never 4xx. `429` and
  `Retry-After` set a per-deployment cooldown, they do not count as breaker
  failures. `402` advances to a sibling deployment first.
- **D7 Explicit routing is exclusive.** A model with an explicit routing policy is
  served only by the deployments named in it. Automatic fallback stages apply only
  when there is no explicit policy, or when the policy sets
  `AllowAutomaticFallback: true`.
- **D8 Deletion criterion.** A package or file is deleted only when it has zero
  non-test importers/callers in **both** `flux` and `../rho` (checked by grep and
  `go list`, including test imports of Rho). Its tests are deleted with it.
- **D9 Guardrails, moderation, budgets, virtual keys, response caching, adaptive
  rate limiting, request coalescing, condenser, role router** are product
  semantics or unreachable inert code. They are deleted, not repaired.
- **D10 Catalog trust.** Remote/cache catalog decoding is tolerant of unknown
  fields. Production discovery starts from `BootstrapCatalog()`, never fixtures.
  A price of 0/0 means **unknown** unless the source marks the model free.
- **D11 Embedded catalog.** A snapshot is embedded. The engine uses it only when
  `Options.FallbackToEmbeddedCatalog` is true (default false), so Rho's existing
  `ErrorCatalogUnavailable` → refresh flow is unchanged.
- **D12 `flux/graph`.** Rho carries a diverged vendored copy. Flux cannot delete
  it without breaking Rho's import; that is a Rho-side PR, recorded in the report.

## Work packages

Waves run in order. Inside a wave, packages own **disjoint files** and run in
isolated git worktrees; branches are merged and gated between waves.

### Wave 1: leaf packages (parallel)

**WP1 transport** — `provider/core/{transport,retry,constants,provider_errors,response_health}.go`,
`internal/probehttp/**`, `internal/httputil/**`
- `CheckRedirect`: strip `x-api-key`, `api-key`, `x-goog-api-key`, `Authorization`,
  and any header the client set for auth on a cross-host redirect. Shared helper
  used by `NewPooledHTTPClient` and `probehttp`.
- `Proxy: http.ProxyFromEnvironment`, `ForceAttemptHTTP2`, `ResponseHeaderTimeout`;
  provide `core.WithoutClientTimeout(*http.Client)` for streams so `Client.Timeout`
  no longer caps stream bodies.
- Retry: on exhaustion return a typed `*FluxError` built from the last response
  (status, request id, `Retry-After`, redacted body); parse `Retry-After-Ms`,
  float seconds, `x-should-retry`; retry 408/409/504; never clamp `Retry-After`
  downward (surface it instead); drain bodies; clear stale `lastResp`;
  fix `CloseIdleConnections` reading `sharedTransport` outside the `Once`.
- Reconcile `constants.go` with the values actually used; delete the unused ones.

**WP2 stream** — `provider/core/{stream,repeat_detector,merge,stream_merger,sanitize,errors,options,core,copy,structured,embedding,image,audio}.go`
(not `guardrails.go`, `stream_guardrails.go`: deleted in Wave 3)
- Anthropic: read `delta.thinking`; capture `signature_delta`; emit a
  `provider_block` event per completed thinking/redacted block; parse
  `message_start`/`message_delta` usage including cache tokens; merge into one
  usage per D1; tolerate empty tool-input JSON; tool-call JSON errors become a
  `tool_call` with `RawArguments` (D4) instead of a fatal error.
- OpenAI: do **not** return at `finish_reason`; drain to `[DONE]`, then emit
  usage, then `done`. Parse `cached_tokens`, `reasoning_tokens`. Accumulate tool
  calls robustly when `index` is absent or an id repeats. Capture
  `extra_content` on tool calls into `ToolCall.ProviderMetadata`.
- `RepeatDetector` off by default; flush `thinkSplitter` at end; SSE parser: BOM,
  spec-correct multi-line `data:`; per-event idle watchdog that emits a typed
  transient error; `core.Go` helper that recovers panics in goroutines and
  reports them as an error event; use it at every `go func` in the package.

**WP3 catalog** — `catalog/**` (including `capabilities`, `live`, `discover`,
`registry`, `opencodego`, `xiaomi`, `zai`, `concentrate`, `opengateway`)
- Tolerant remote/cache decode; surface remote-parse failure in `RefreshResult`.
- Production discovery from `BootstrapCatalog()`; replace only deployments that
  returned live models. Fixtures stay where they are until Wave 4.
- 0/0 → `PricingUnknown` (D10); carry cache read/write, reasoning and tier prices
  into `RatesPer1M`; expose `catalog.CostUSD(offering, usage)` using D1.
- OpenRouter enrichment: join on `canonical_slug`, only for first-party
  Anthropic/OpenAI/Google/xAI, one download per refresh, never overwrite
  provider-reported context or output limits.
- Gemini key in `x-goog-api-key` header; scrub URLs from returned errors;
  Anthropic `limit=1000`; thread `ctx` through every fetcher; bounded concurrency.
- Fix the `compiled_list.go` in-place sort race (clone before sorting); fix
  `mini` matching inside `gemini` (token boundaries, expensive patterns first);
  fix `registry.Register` replace leaving stale slices; deployment env-fallback
  precedence; Concentrate pricing (check status, bound reads, cache only parsed
  prices, `0600` atomic write).
- Delete: `capabilities` package, `deprecation.go` and the hard-coded model-name
  table if callers allow (D8), unused `opencodego` usage tracking, legacy
  `ModelCatalog` path. Replace `opencodego/models.go` wrong prices/windows with
  unknown rather than wrong data.
- Neutral error text: no reference to the `rho` CLI in library errors.

**WP4 credentials-config** — `credentials/**`, `config/**`, `docs/guides/CREDENTIAL-SETUP-FLOW.md`
- `MigrateEnvFileCredentials`: delete a file only after every secret in it is
  confirmed written by re-reading; preserve unmigrated lines; never write the
  done-marker on failure; return the error.
- Guard `ServiceName` with a mutex + accessor; `CombinedStore.Set("")` errors;
  `StorageReportFor` uses the write probe; distinguish "not found" from
  "backend failed" (additive API); parallelise `APIKeysMap` with a bounded pool;
  longer negative-cache TTL; OIDC calls get a client timeout and a caller context.
- Remove `DisallowUnknownFields` from the strict `provider.json` loader
  (forward compatible; keep the trailing-value check).
- Derive `~/.rho`/`~/.hawk` paths and "Rho" strings from a settable name.
- Delete: `category.go`, `user_profiles.go`, `agent_routing.go`, `runtime.go`,
  `ApplyProviderEnvToProcess`/`ApplyProviderConfigToEnv` (D8), stale hard-coded
  model IDs. Fix the credential flow doc (real path, 28 providers).

**WP5 router** — `router/**`, plus `runtime/replica.go` and its test (single-file exception)
- D6, D7, D5; forward events until `done` (usage before done is normal);
  jittered backoff between stage retries; all-breakers-open with a single
  deployment still attempts a probe; per-deployment first-output timeout
  (configurable, default 120s); session-sticky selection by rendezvous hash on
  `session.id`; report `DeploymentID`/`Attempts` on responses and `done` events;
  `LiveDeploymentRouter.Replace` and any rebuild carry breaker state by
  deployment id; an `OnAttempt` hook for engine/telemetry.
- Delete: `router.go`, `strategy.go`, `filter.go` (the `Router` and six
  strategies), `router/controlplane/**`, `runtime/replica.go`.

**WP6 deadcode** — `internal/{api,grpc,cache,observability,shrink,sdk}`, `verify`,
`codeagent`, `utils`, `constants`, `provider/batch`, `storage`, `conversation`,
`examples` (rewritten in Wave 4)
- Delete per D8; then `go mod tidy`. `storage` and `conversation` are deleted only
  if the census shows nothing reachable imports them once the above are gone.

### Wave 2: adapters and engine (parallel, after Wave 1 merged)

**WP7 openai-family** — `provider/adapters/{openai*,compat,azure,concentrate_responses,provider_registry,adapter_config,zai,longcat,opencodego,poolside,protocol_router}.go`
and the thin shims (`agnes canopywave clinepass deepseek grok groq kimi minimax
ollama opengateway openrouter stepfun mimo`) plus their tests in `provider/` and `provider/adapters/`
- Inject `opts.System` on the OpenAI wire; reject `N>1`; drop non-standard
  `is_error`; `Ping` healthy only on 2xx; `core.UserAgent()` everywhere.
- First-party OpenAI and Azure over the **Responses API** (generalise the
  Concentrate codec), replaying reasoning items through `ProviderBlocks` with
  `include:["reasoning.encrypted_content"]`; Azure via the `/openai/v1/` path,
  `max_completion_tokens`, streamed usage. Chat Completions stays for compat gateways.
- Collapse the 13 shim files into one `NewCompatClient(spec)` file, keeping the
  existing exported constructors as one-liners so `setup/` compiles; `Name()`
  returns the registry provider id; fix compat flags from vendor docs, add the
  `fireworks` row, default custom-gateway streamed usage on; delete unread flags.
- One construction table used by both `provider/` and `setup/`.
- Poolside genuinely streams. `prompt_cache_key` passthrough. Echo
  `ToolCall.ProviderMetadata` (Gemini-compat `extra_content`).

**WP8 anthropic-family** — `provider/adapters/{anthropic,anthropic_base,anthropic_cache,bedrock,vertex}.go` + tests
- One request builder; delete the forked cached builder; `cache_control` on
  system, tools and the **last** block; `ttl` option; `output_config`, `metadata`,
  `service_tier` on every path.
- Model-aware thinking: adaptive vs budget, `display`, no `temperature/top_p/top_k`
  where the model rejects them; emit `CallWarning` when a setting is dropped.
- Replay `ProviderBlocks` (thinking + signature, `redacted_thinking`) before
  `tool_use`; multiple system messages concatenate, never overwrite; tool
  `strict`, `eager_input_streaming`; stop reasons `pause_turn`, `refusal`,
  `model_context_window_exceeded` handled; malformed tool JSON never becomes an
  empty-argument call; `count_tokens` forwards thinking and tool choice.
- Usage per D1.
- **Bedrock:** decode `{"bytes":base64}` frames, surface exception frames as typed
  errors, accumulate tool arguments, map usage explicitly, SigV4 canonical URI
  double-encoded, strip body keys Bedrock rejects, add `anthropic_version`,
  `AWS_BEARER_TOKEN_BEDROCK`, credential source with refresh.
- **Vertex:** global/multi-region hosts, `model` out of the body, token source
  interface with refresh on 401.

**WP9 gemini** — `provider/adapters/{gemini,gemini_direct}.go` + tests
- Inject `opts.System`; finish only on `finishReason`/close, keep latest usage;
  `thoughtSignature` round-trip; populate `thinkingConfig`; URL images as
  `fileData`; configurable safety settings; honour named `tool_choice`;
  structured output independent of other options; bounded response reads.

**WP10 engine** — `engine/**`, `setup/**`, `runtime/**` (not `runtime/replica.go`)
- Memoise the composed transport per `Engine`, keyed by a fingerprint of
  provider config, catalog and credential state; swap via `LiveDeploymentRouter`
  so breaker state survives; load runtime state once per request; cache the
  credential environment with a short TTL (was 132 keychain reads per request).
- Map `ThinkingEnabled` (add a reflect test that every `GenerationOptions` field
  reaches `ChatOptions`); `Stream.Err()` set on cancel/timeout; classify with
  `ErrorInfo`; emit `context_exceeded`/`content_filtered`; continuation is
  opt-out, keeps thinking, preserves typed errors; emit `route_changed`/`retry`
  from the router hook or delete the constants.
- `Options.FallbackToEmbeddedCatalog` (D11); remove `EnableCaching`,
  `EnableRateLimiting`, `CacheConfig`, `RateLimitConfig`; default `MaxTokens` from
  the catalog; fill `Route.DeploymentID`; wire `core.SetVersion` so the
  User-Agent is not `flux/dev`.
- Review `setup/` (~1,400 lines, unreviewed by the audit) and
  `engine/host_control.go` `MigrateProviderSecretsContext`. Add the OIDC
  `allowAmbient` decision. Cross-process lock for `provider.json`; honour or
  delete the migration marker. Media methods classify errors.
- `runtime`: delete the exports nothing references (D8), remove `os.Setenv`,
  correct the package doc. Native compaction stops hard-coding `api.anthropic.com`.
- `operationsgraph`: drop the unsound anonymisation claim (keep the projection).

### Wave 3: cleanup and data (parallel)

**WP11 provider-root** — `provider/*.go`, `provider/{resilience,cache,observability,embeddings,media,extraction,testkit}`,
and the guardrail hooks in `provider/core` and `provider/adapters`
- Delete D9 items: `provider/cache`, `embeddings/cache.go`, `resilience/{adaptive_ratelimit,ratelimit,coalesce,condenser,guardrails,moderation,health,roles,policy,thinking_policy}.go`,
  `core/{guardrails,stream_guardrails}.go`, `WithGuardrails`, adapter guardrail
  calls, `observability/{budget_provider,usage_limit,usage_tracker,callbacks,request_logger,call_metrics,cache_analytics,cost_estimator}.go`.
- Keep and fix: `continuation.go` (append only the delta, `done` on tool-JSON cut),
  `recorder`/`cassette` (redact request messages and tool arguments too).
- `provider.Client(opts...)` stores and applies options; `ResolveDefaultModel`
  uses an explicit default and retries a failed catalog load; no credential scan
  when `cfg.Provider` is set.
- Structured output: use the adapters' native `ResponseFormat` path; delete the
  Anthropic prefill and the no-op `media.WithStructuredOutput`.

**WP14 catalog-data** — `catalog/**`, `scripts/gen-catalog/**`
- Generator: models.dev `api.json` → catalog v1 with a reviewed overlay file;
  embedded snapshot via `go:embed`; a drift script that compares against LiteLLM
  for the top models. Correct model-ID/price entries. `NOTICE` for `catalog/v1.go`
  only if a diff against langdag shows it is derived (MIT).

### Wave 4: tests, telemetry, docs (mostly sequential)

**WP12 test-relocation** — move surviving `provider/*_test.go` into their owning
packages; delete `test_compat*_test.go`; unexport wrappers that only the shim
needed (`AWSCanonicalURI`, `Sha256Hex`, …); make assertion-free security tests
real or delete them; move `catalog/testfixtures.go` behind a test-only package.

**WP13 otel** — spec-conformant `gen_ai.*` span per `Generate`/`Stream` using only
the OTel **API** (no SDK), `gen_ai.client.*` metrics incl. time-to-first-chunk,
retry/fallback span events, cost from `catalog.CostUSD`, content capture opt-in
and off. Rewrite `TracingProvider` (with a real `cancel`). Wired inside the engine.

**WP15 conformance** — vendor-doc fixtures replayed through adapter, router and
engine for Anthropic, OpenAI (Chat + Responses), Gemini, Bedrock, Azure. A record
mode (`FLUX_RECORD=1`, needs keys) so real cassettes can replace the fixtures.

**WP16 docs** — README (compiling examples, 28 providers, wire-protocol table,
honest status), AGENTS.md (regenerated file map; the old one cited 12 missing
files), `docs/ARCHITECTURE.md` (no `flux serve`), SECURITY/CONTRIBUTING false
claims, CHANGELOG, examples through `engine`.

**CI** (done by hand) — `-coverpkg=./...`, deadcode job gates on growth, an
`apidiff` report job, gosec exclusions trimmed to what is justified.

## Deferred, with reasons

| Item | Reason |
|---|---|
| Host the catalog at a GrayCodeAI URL | needs infrastructure this repo cannot create; the embedded snapshot removes the runtime dependency instead |
| Recorded provider cassettes | no API keys available; WP15 ships vendor-doc fixtures and a record mode |
| P2C / peak-EWMA balancing | only matters with ≥2 deployments for one model and needs the persisted state and telemetry that land in this plan first |
| Namespaced `ProviderOptions` replacing flat `ChatOptions` fields | a deliberate public API decision (AGENTS.md); adding an unread field would be dead surface |
| Derived default deployment weights | P3, changes default behaviour for OpenAI+Azure users |
| Delete `flux/graph` | Rho imports it; needs a Rho-side PR (D12) |
| `OTEL-CONVENTIONS.md` correction | file lives in the Rho repo; recorded for a Rho PR |
| Rho `internal/engine` unused import | pre-existing Rho build error, not flux's to fix |

## Verification protocol (run twice, by different actors)

**Pass 1: mechanical gate (per wave and at the end).** Every CI job run locally:
boundary guards, `gofumpt -l`, `goimports -l`, `go mod tidy` no diff, `go vet`,
`golangci-lint`, `go test -race -shuffle=on -count=1 -coverpkg=./...`, coverage
≥ 60% real, `govulncheck`, `gosec`, `deadcode`, four fuzz targets, six-target
cross-compile. Plus every executed audit repro rewritten as a passing test.

**Pass 2: independent adversarial review (final).** New agents that did not write
the code, one per work package: re-run the original audit repro against the final
tree, read the diff for regressions and new dead code, try to break each fix, and
check that docs match code. Plus `-count=3`, an `apidiff` against the base commit
to enumerate every breaking change, and README snippets compiled as examples.

**Cross-repo gate.** With a scratch `go.work` joining `flux` and `../rho`: the set
of Rho packages failing to build must equal the baseline (`internal/engine` only),
and `rho/internal/{provider/gateway,testaudit,config}` tests must pass.
