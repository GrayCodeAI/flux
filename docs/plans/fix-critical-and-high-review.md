# Plan: Fix Critical + High-Impact Review Findings — graycode-router

> Branch: `fix/critical-and-high-review-2026-06`
> PR: <https://github.com/GrayCodeAI/graycode-router/pull/38>
> Status: **✅ COMPLETE — all 8 items committed, 8 PRs merged into the branch.**
> Constraint: **no new go.mod / go.sum dependencies** for any item in this plan.

## Completion summary

| ID  | Severity | Title | Status | Commit |
|-----|----------|-------|--------|--------|
| C1  | critical | Pin `go.mod` to a real Go version | ✅ no-op (1.26.4 is real) | — |
| C2  | critical | Fix Vertex misrouting bug | ✅ committed | `b5c4a90` |
| C6  | critical | Fix keyring goroutine leak | ✅ committed | `e55a3c5` (merged as `6603473`) |
| C7  | critical | Remove ghost dynamic-provider auto-register (opt-in flag) | ✅ committed | `c1f2869` |
| H1  | high | Unify Gemini SSE parser | ✅ committed | `98af9e7` |
| H2  | high | Extract shared `providerRequest` builder | ✅ committed | `704162f` |
| H3  | high | Unify Anthropic response parsing (3 → 1) | ✅ committed | `cf61ca7` |
| H4  | high | Wire `GraycodeRouterError` into provider error paths | ✅ committed | `d8993ba` |

**Net diff**: 8 production-code changes + 8 test files; all tests pass with `-race`; `go vet ./...` clean; `go.mod` / `go.sum` unchanged. **No new dependencies.**

## Context

A deep code review of `graycode-router` and `hawk` (see `docs/plans/review-2026-06-summary.md`
— to be created if you want it archived here) surfaced 7 critical and 9 high
items. This plan covers **all graycode-router items** (C1, C2, C6, C7, H1, H2, H3, H4)
broken into a sequence of small, reviewable PRs.

The hawk-side companion plan lives at `../hawk/docs/plans/fix-critical-and-high-review.md`.

## Scope (graycode-router)

| ID | Severity | Title | File(s) | Effort |
|----|----------|-------|---------|--------|
| C1 | critical | Pin `go.mod` to a real Go version | `go.mod:3` | XS (1 line) |
| C2 | critical | Fix Vertex misrouting bug | `client/provider_registry.go:167-177`, `client/vertex.go` | S |
| C6 | critical | Fix keyring goroutine leak | `credentials/keyring_platform.go:22-32` | S |
| C7 | critical | Remove ghost dynamic-provider auto-register | `client/dynamic.go:62-70`, `client/provider_registry.go:107-110` | S |
| H1 | high | Unify Gemini SSE parser | `client/gemini.go:496-535` | M |
| H2 | high | Extract shared `providerRequest` builder | `client/anthropic.go`, `client/openai.go` | L |
| H3 | high | Unify Anthropic response parsing (3 → 1) | `client/{anthropic,bedrock,vertex}.go` | M |
| H4 | high | Wire `GraycodeRouterError` into provider error paths | `client/errors.go`, all `client/*.go` | M |

## Out of scope (deferred to next plan)

- H11: Provider-registry drift (12/17/19). Documentation alignment only.
- M1–M20 medium items from the review.
- L-tier quick wins.
- Anything that requires a new dependency.

## Sequencing rationale

Critical items first, in **independent** order (no PR blocks another). Then
high items in dependency order: H1 → H2 → H3 → H4.

| PR | Items | Why this order | Branching strategy |
|----|-------|----------------|--------------------|
| 1  | C1    | Trivial. Unblocks CI. | direct on branch |
| 2  | C2    | Standalone bug fix. No API change. | direct on branch |
| 3  | C6    | Resource leak. Independent. | direct on branch |
| 4  | C7    | Security; opt-in flag addition. | direct on branch |
| 5  | H1    | Bug-fix-by-unification. Independent. | direct on branch |
| 6  | H2    | Refactor. Best after H1 so one SSE test infra exists. | direct on branch |
| 7  | H3    | Refactor. Independent of H2. | direct on branch |
| 8  | H4    | Touches all providers. Last. | direct on branch |

PRs can be merged individually; the branch is just a namespace.

---

## PR 1 — Pin Go version (C1)

**What**: `go.mod:3` says `go 1.26.4` — does not exist. Pin to the latest
stable Go (verify via <https://go.dev/dl/> on the day this lands). Also bump
`GOTOOLCHAIN` policy if needed.

**Files**:
- `go.mod:3` (1 line)

**Test plan**:
- `go mod verify`
- `go build ./...`
- `go test -race ./...`
- Confirm no `toolchain` directive is needed for the chosen version.

**Risk**: low. A wrong Go version is the only failure mode; trivially
re-pinnable.

**Rollback**: revert the commit.

---

## PR 2 — Fix Vertex misrouting (C2)

**Bug**: `client/provider_registry.go:167-177` instantiates Vertex as a
`GeminiClient` (Gemini wire format + URL), even though `client/vertex.go:42`
defines `c.baseURL()` returning `publishers/anthropic/models` (Anthropic-on-Vertex).
Vertex users are silently sent to Gemini's endpoint with an Anthropic-shaped
URL — guaranteed 4xx.

**Fix**:
1. Add a `NewVertexClient` branch to `getOrCreateProvider` matching the
   `ProviderTypeVertex` case.
2. Verify the constructed client uses the `vertex.go:42` URL and
   `vertex.go:14-100` request/response paths.
3. Add a unit test that asserts the constructed client is `*VertexClient`
   and its `BaseURL()` matches the Anthropic-on-Vertex template.

**Files**:
- `client/provider_registry.go` (replace 1 switch case)
- `client/vertex.go` (likely no change; review for testability)
- `client/provider_registry_test.go` (NEW — was the biggest test gap)

**Test plan**:
- `TestGetOrCreateProvider_Vertex` asserts `*client.VertexClient`.
- Existing `vertex_test.go` (cloud_providers_test.go) covers HTTP roundtrip;
  ensure it still passes against the corrected URL.
- Run the `verify/` conformance harness against a live Vertex endpoint
  (only if credentials are present; otherwise skip).

**Risk**: low. The fix is a one-line provider switch + test. No API surface
change. Misrouting today is already broken, so no regression window.

**Rollback**: revert the commit. (Vertex users currently get a 4xx; reverting
restores that 4xx — same as before this PR.)

---

## PR 3 — Fix keyring goroutine leak (C6)

**Bug**: `credentials/keyring_platform.go:22-32` (`keyringDo`) spawns a
goroutine that calls `fn()` (the keyring call) and writes the result to a
buffered channel. If the caller's `ctx` is cancelled, the goroutine keeps
running and eventually writes to the channel. If `fn()` blocks (locked
keyring), the goroutine leaks indefinitely.

**Fix**:
1. Replace the goroutine with a select on `(ctx.Done(), doneCh)`.
2. The keyring call is wrapped in a goroutine that selects on a `done`
   channel; if `done` is closed, the goroutine exits without writing.
3. Alternative (cleaner): use `errgroup.WithContext` and `context.AfterFunc`
   (Go 1.21+).

**Files**:
- `credentials/keyring_platform.go` (rewrite `keyringDo`)

**Test plan**:
- `TestKeyringDo_ContextCancel` — call with a cancelled ctx; assert the
  function returns within 50ms and the keyring mock is not invoked.
- `TestKeyringDo_NormalReturn` — call normally; assert result is propagated.
- `TestKeyringDo_KeyringBlocks` — mock keyring that blocks forever; assert
  cancellation exits the goroutine (use a leaked-goroutine detector: a
  `runtime.NumGoroutine()` before/after).

**Risk**: low. The fix is local; other code paths already trust that
`keyringDo` respects ctx.

**Rollback**: revert. The leak is pre-existing.

---

## PR 4 — Remove ghost dynamic-provider auto-register (C7)

**Bug**: `client/dynamic.go:62-70` reads `OPENAI_API_BASE` / `OPENAI_BASE_URL`
at request time and `client/provider_registry.go:107-110` auto-registers an
unknown provider as an OpenAI-compatible client pointed at that URL. A
poisoned `OPENAI_API_BASE` (e.g., from a leaked `.envrc`) exfiltrates the
user's `OPENAI_API_KEY` header to the attacker's server.

**Fix** (two-step, opt-in safe):
1. Remove the auto-registration. `getOrCreateProvider` returns
   `ErrUnknownProvider` for unknown provider names.
2. Add a documented opt-in: `GRAYCODE_ROUTER_ALLOW_DYNAMIC_PROVIDERS=1` env var. When
   set, the existing auto-registration is allowed (for users who run
   local proxies like LiteLLM, Ollama, etc.). Default: off.
3. Log a `WARN` line the first time a dynamic provider is registered.

**Files**:
- `client/dynamic.go` (gate the registration on the env var)
- `client/provider_registry.go` (default-error branch; no auto-register)
- `docs/guides/CREDENTIAL-SETUP-FLOW.md` (document the env var)

**Test plan**:
- `TestDynamicProvider_DefaultDeny` — unknown provider returns
  `ErrUnknownProvider`.
- `TestDynamicProvider_OptIn` — with `GRAYCODE_ROUTER_ALLOW_DYNAMIC_PROVIDERS=1`,
  the existing behavior is preserved.
- `TestDynamicProvider_LogsWarning` — assert the `WARN` log line.

**Risk**: low. The new opt-in is backward-compatible for users who set
`GRAYCODE_ROUTER_ALLOW_DYNAMIC_PROVIDERS=1`. The default is safer.

**Rollback**: revert. The opt-in can be enabled in the env at any time.

---

## PR 5 — Unify Gemini SSE parser (H1)

**Bug**: `client/gemini.go:496-535` has its own bespoke SSE parser
(`streamLoop`) using a 4 KB read buffer. Every other provider uses
`client/stream.go:32-88` `parseSSEStream` with a 2 MB buffer. Bug fixes to
SSE parsing don't reach Gemini; the Gemini parser doesn't respect `ctx.Done()`
between reads.

**Fix**:
1. Extract a `processGeminiStream` modeled on `processAnthropicStream` /
   `processOpenAIStream`.
2. Replace `streamLoop` with a call to `parseSSEStream` + `processGeminiStream`.
3. Map Gemini finish reasons consistently with other providers (consider
   centralizing in `client/finish_reasons.go`).

**Files**:
- `client/gemini.go` (replace `streamLoop`; add `processGeminiStream`)
- `client/stream.go` (no change to `parseSSEStream`; ensure it handles
  Gemini's `data:` lines — verify it does)
- `client/gemini_test.go` (add streaming test with multiple events)

**Test plan**:
- `TestGemini_Streaming_ToolCall` — mock SSE server emits tool-call deltas;
  assert the assembled call.
- `TestGemini_Streaming_ContextCancel` — cancel mid-stream; assert no panic
  and the goroutine exits.
- `TestGemini_Streaming_LargeEvent` — single SSE event > 2 MB; assert
  graceful handling (current `parseSSEStream` may fail; document the
  limit or bump the buffer).
- Existing `cloud_providers_test.go` should pass unchanged.

**Risk**: medium. Streaming changes can break live behavior. Mitigation:
ship behind a feature flag (`GRAYCODE_ROUTER_GEMINI_SHARED_PARSER=0` to revert to
the old path), keep both code paths for one release, then remove.

**Rollback**: feature flag. If regressions appear, set the env var to 0.

---

## PR 6 — Extract shared `providerRequest` builder (H2)

**Refactor**: `client/anthropic.go:375-578` and `client/openai.go:408-507`
each have ~120 / ~70 lines of near-duplicate setup between `Chat` and
`StreamChat`. Every field — `opts.System`, `opts.Temperature`, `opts.TopP`,
`opts.TopK`, `opts.StopSequences`, `opts.EnableCaching`, `tools`, `thinking`,
`metadata`, `outputConfig`, `serviceTier` — is re-applied twice.

**Fix**:
1. For Anthropic: extract `buildAnthropicRequest(opts ChatOptions, stream bool) ([]byte, *http.Request, error)`.
2. For OpenAI: extract `buildOpenAIRequest(opts ChatOptions, stream bool) ([]byte, *http.Request, error)`.
3. Both `Chat` and `StreamChat` call the builder; differ only in the
   `Accept` header and the response handler.
4. Reduce duplication of the 32 MB body-size check (3 sites in anthropic.go).

**Files**:
- `client/anthropic.go` (extract builder, reduce ~120 LOC)
- `client/openai.go` (extract builder, reduce ~70 LOC)
- `client/transport.go` (add a `requestSizeLimit` const)

**Test plan**:
- All existing tests pass unchanged.
- `TestAnthropic_ChatVsStream_SameBody` — assert both methods produce
  byte-identical request bodies (modulo `stream: true/false`).
- `TestOpenAI_ChatVsStream_SameBody` — same.
- Line-count diff: ~200 LOC removed.

**Risk**: medium. Refactor of a hot path. Mitigation: byte-equality tests
on the body; goldens for known-good requests; ship behind a feature flag
in CI but default-on.

**Rollback**: revert.

---

## PR 7 — Unify Anthropic response parsing (H3)

**Refactor**: `client/anthropic.go:457-486`, `client/bedrock.go:432-460`,
`client/vertex.go:85-100` each implement a near-duplicate `responseFromAnthropic`.
A wire-format change needs 3 edits.

**Fix**:
1. Move the parser to `client/anthropic_response.go` (or
   `client/response.go`) as `parseAnthropicResponse(raw []byte, requestID, orgID string) (*GraycodeRouterResponse, error)`.
2. All three call sites import it. They differ only in how `requestID` /
   `orgID` are extracted from the response (HTTP headers), so pass those in.
3. `buildAnthropicMessages` is already shared; mirror the same pattern.

**Files**:
- `client/anthropic.go` (delete local copy)
- `client/bedrock.go` (delete local copy; extract headers)
- `client/vertex.go` (delete local copy; extract headers)
- `client/anthropic_response.go` (NEW)

**Test plan**:
- `TestParseAnthropicResponse_*` — table-driven test covering tool calls,
  thinking blocks, refusal, max-tokens, content-only, multi-part content.
- Existing per-provider tests pass unchanged.

**Risk**: medium. Cross-provider refactor. Mitigation: each provider keeps
its own test; the shared parser is unit-tested independently.

**Rollback**: revert.

---

## PR 8 — Wire `GraycodeRouterError` into provider error paths (H4)

**Refactor**: `client/errors.go:7` defines `GraycodeRouterError` with
`IsRetriable()`, `IsAuthError()`, `IsRateLimited()` methods, but **no
provider returns `*GraycodeRouterError`**. All error paths use
`fmt.Errorf("graycode-router: …")`. `doWithRetry` does its own string classification
instead of using the structured type.

**Fix**:
1. `formatAPIError` returns `*GraycodeRouterError` (wrap into a `*GraycodeRouterError` with
   `StatusCode`, `RequestID`, `Message`, `Err`).
2. `doWithRetry` checks `var graycode-routerErr *GraycodeRouterError; if errors.As(err, &graycode-routerErr) { … }`
   instead of string-matching status codes.
3. All provider error returns flow through `formatAPIError`.
4. Public API consumers (hawk) can now use `errors.As` for typed errors.

**Files**:
- `client/errors.go` (extend `GraycodeRouterError` with `Unwrap()`, helpers)
- `client/anthropic.go`, `client/openai.go`, `client/gemini.go`,
  `client/bedrock.go`, `client/vertex.go`, `client/azure.go` (use the
  shared `formatAPIError`)
- `client/retry.go` (use `errors.As` instead of string match)
- `client/errors_test.go` (extend coverage)
- `client/fallback.go` (use `IsRetriable()` instead of
  `isRetriableError` heuristic)

**Test plan**:
- `TestGraycodeRouterError_IsRetriable` — table of status codes.
- `TestRetry_UsesGraycodeRouterError` — mock 401; assert retry does NOT happen
  (auth errors are not retriable).
- `TestFallback_UsesGraycodeRouterError` — mock auth error in second provider;
  assert fallback to third.
- All existing tests pass.

**Risk**: medium. Error semantics change for consumers. Mitigation: keep
the old string format in `Error()` for backward compat.

**Rollback**: revert. The old behavior is unchanged for callers that
don't `errors.As`.

---

## Cross-cutting guarantees

- **No new dependencies** in any PR. All changes use stdlib + existing
  imports only.
- **No new public API** is removed. New error types are additive.
- **All changes are independently testable**; the branch is a namespace,
  not a single atomic change.

## Verification at the end of the branch

```bash
go mod verify
go build ./...
go test -race -count=1 -shuffle=on ./...
go vet ./...
golangci-lint run
govulncheck ./...
make ci
```

Coverage target: maintained at 60%+ (CI gate).

## Open questions for approval

1. **C7 default-deny vs opt-in flag** — confirm you want the flag
   (instead of hard removal of the dynamic path).
2. **H1 feature flag** — confirm you're OK with a temporary
   `GRAYCODE_ROUTER_GEMINI_SHARED_PARSER` env var for one release.
3. **H2 / H3 sequencing** — H2 first (bigger, but independent) or H3
   first (smaller, lower risk)?
4. **H4 scope** — should `client/recorder.go` and `client/coalesce.go`
   also adopt `GraycodeRouterError`, or is that M-tier?
5. **Branch lifetime** — keep the branch as a long-lived namespace, or
   squash each PR to a single commit on merge?
