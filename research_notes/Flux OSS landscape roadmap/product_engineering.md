# Flux OSS product surface and engineering maturity audit

**Audit snapshot:** 2026-09-24, commit `f9037a82e403001d50868e4100bcfa19d8a387af` (`main` / `origin/main`).
**Method:** read-only inspection of the complete tracked documentation, API/specification, examples, scripts, workflows, module metadata, public package docs, representative implementation and tests, plus local race/coverage/vet/security checks and read-only GitHub metadata queries. No repository source was edited.

## 1. Product surface, documentation, OSS operations, and trust

### Takeaway

Flux has a credible internal engineering foundation and unusually broad deterministic test coverage, but its public product story is not yet coherent enough for independent adoption. A fresh user cannot reliably execute the advertised `engine` quickstart, several headline surfaces are internal or unwired, documentation has drifted materially from code, and the only current-module release is `v0.0.1` with 29 later commits on `main`; Flux should be positioned as a pre-1.0 provider-runtime library, fix a small set of trust blockers, and release a deliberately narrow `v0.1.0` before pursuing `v1.0`.

### Cited Findings

#### Overall adoption assessment

| Dimension | Assessment | Evidence-based rationale |
|---|---|---|
| Core Go engineering | **Good, with important host-facade gaps** | The current tree passes the full race suite, vet, module verification, and `govulncheck`; test code is larger than production code. However, the stable `engine` path rebuilds stateful transports on every request, so headline reliability middleware is not persistent in normal use. [`engine/engine.go:54-68,283-327`](../../engine/engine.go#L54-L68), [`setup/deployment.go:94-110`](../../setup/deployment.go#L94-L110) |
| Onboarding / quickstart | **Not adoption-ready** | The README constructs `engine.New(Options{})` and immediately streams, while runtime loading requires a valid catalog cache; the advertised default remote catalog currently contains a field rejected by the strict decoder. [`README.md:67-105`](../../README.md#L67-L105), [`engine/state.go:86-103`](../../engine/state.go#L86-L103), [`catalog/v1.go:444-460,615-644`](../../catalog/v1.go#L444-L460) |
| API focus | **Weak** | Fifty-two main-module packages are listed by `go list`; the host-facing `llm.Provider` is a composition of seven broad facets, and important public packages expose many declarations without a documented stability tier. [`llm/provider.go:9-24`](../../llm/provider.go#L9-L24), [`engine/types.go:1-164`](../../engine/types.go#L1-L164) |
| OSS documentation | **Mixed** | The README, architecture boundary, dynamic discovery, and decentralized-routing honesty are useful, but provider counts, env names, examples, OpenAPI, cache/audit claims, release tooling, and SDK claims have drifted. [`README.md:31-44,178-235`](../../README.md#L31-L44), [`docs/guides/CREDENTIAL-SETUP-FLOW.md:7-27`](../../docs/guides/CREDENTIAL-SETUP-FLOW.md#L7-L27), [`api/openapi.yaml:118-340`](../../api/openapi.yaml#L118-L340) |
| Release / supply-chain trust | **Early** | The sole current-module release is signed and CI is strong, but no release-PR automation, Go API diff, SBOM, provenance/attestation, dependency bot, or vulnerability-alert setting exists in the repository/GitHub configuration. [`release.yml:1-31`](../../.github/workflows/release.yml#L1-L31), [v0.0.1 release](https://github.com/GrayCodeAI/flux/releases/tag/v0.0.1), [repository security settings](https://api.github.com/repos/GrayCodeAI/flux) |
| Extensibility | **Mixed** | Custom OpenAI-compatible gateways, injected credential stores, the four-method `core.Provider`, and lower-level HTTP-client injection are good seams. Native provider onboarding, custom routing strategies, custom engine transports, model publication, cache backends, and telemetry sinks are fragmented or inaccessible. [`provider/dynamic.go:11-50`](../../provider/dynamic.go#L11-L50), [`credentials/store.go:27-32`](../../credentials/store.go#L27-L32), [`router/strategy.go:10-31,107-168`](../../router/strategy.go#L10-L31) |

#### Product identity and public story

- The README has a clear one-sentence promise—authentication, model resolution, streaming, retries, rate limiting, and caching—and accurately positions Flux as the provider engine beneath the Rho product face. This is the strongest part of the product narrative. [`README.md:5-9,31-44`](../../README.md#L5-L9)
- The repository simultaneously presents Flux as a host-neutral universal runtime, a library, an HTTP gateway, a gRPC server, a conversation DAG, an SDK suite, and an enterprise foundation. The current project has no standalone `main` package, while the HTTP and gRPC servers live below `internal/`, so general Go consumers cannot import those delivery surfaces. [`Makefile:1-3,50-53`](../../Makefile#L1-L3), [`internal/api/server.go:1-16`](../../internal/api/server.go#L1-L16), [`internal/grpc/README.md:1-20`](../../internal/grpc/README.md#L1-L20)
- The architecture document says operators can run `flux serve <port>`, but the Makefile explicitly says Flux is a library with no standalone binary. This is a direct product-surface contradiction. [`docs/ARCHITECTURE.md:52-59`](../../docs/ARCHITECTURE.md#L52-L59), [`Makefile:1-3`](../../Makefile#L1-L3)
- The architecture's package map includes `errors/`, but the current root tree has no tracked `errors/` package. The host-facing package list is therefore not a generated or checked source of truth. [`README.md:288-325`](../../README.md#L288-L325)
- The README says hosts may import exactly `engine`, `llm`, `graph`, and `tools`, but the host `llm.Provider` combines generation, catalog, credential, selection, gateway, catalog-maintenance, and native-compaction responsibilities. That is a coherent Rho composition boundary, but it is not the smallest API for a general Go adopter. [`README.md:46-62`](../../README.md#L46-L62), [`llm/provider.go:16-24,137-152,201-212,244-281,317-328,388-402`](../../llm/provider.go#L16-L24)

#### Quickstart and first-run experience

- The README quickstart says Go 1.26+, a configured credential, `engine.New(Options{})`, then `Stream`; it does not initialize a catalog, persist provider state, save a key, or call a catalog refresh. [`README.md:67-105`](../../README.md#L67-L105)
- `Engine.loadRuntimeState` always loads the catalog with `RequireCache: true`; a missing cache returns `ErrCatalogCacheRequired`, and the error text tells the caller to run `rho models refresh`, a command outside Flux and unavailable to independent adopters. [`engine/state.go:86-103`](../../engine/state.go#L86-L103), [`catalog/v1.go:615-644`](../../catalog/v1.go#L615-L644), [`catalog/errors.go:3-7`](../../catalog/errors.go#L3-L7)
- `engine.New` silently chooses `~/.flux/model_catalog.json` when no state directory is supplied, while its provider path still comes from Flux's process-default configuration resolver. The quickstart therefore depends on undocumented local state. [`engine/engine.go:70-117`](../../engine/engine.go#L70-L117), [`catalog/refresh.go:26-33`](../../catalog/refresh.go#L26-L33)
- The default remote catalog is hosted at `https://langdag.com/model-catalog/v1/catalog.json`, outside the documented GrayCode ecosystem and without an in-repository publication workflow. [`catalog/v1.go:44-50`](../../catalog/v1.go#L44-L50), [`DYNAMIC-MODEL-DISCOVERY.md:90-108`](../../docs/guides/DYNAMIC-MODEL-DISCOVERY.md#L90-L108)
- On 2026-09-24, that default catalog was reachable and declared `generated_at: 2026-09-23`, but its `openai-direct` deployment included `api_protocol_ids`, a field absent from Flux's `Deployment` type. Because `ParseCatalog` enables `DisallowUnknownFields`, the current default document is expected to fail decoding. This is a verified compatibility break between the shipped default source and the checkout, and CI does not test the live default URL. [current default catalog](https://langdag.com/model-catalog/v1/catalog.json), [`catalog/v1.go:85-96,444-460`](../../catalog/v1.go#L85-L96)
- The catalog's `Provenance` type is descriptive metadata only (`source`, `source_url`, `observed_at`); there is no signature, digest pin, or provenance verification in the catalog package. [`catalog/v1.go:287-291`](../../catalog/v1.go#L287-L291)
- The catalog decoder deliberately rejects every unknown field, which makes additive producer evolution immediately breaking for consumers and directly conflicts with the broader engine policy that future event/field evolution should be additive. [`catalog/v1.go:444-460`](../../catalog/v1.go#L444-L460), [`HOST-ENGINE-BOUNDARY.md:188-193`](../../docs/architecture/HOST-ENGINE-BOUNDARY.md#L188-L193)

#### Documentation coherence

- The README's 28-provider table is mostly clear and useful, but the credential guide still says “Supported providers (15)” and lists a stale `z-ai` ID while omitting 13 registered gateways. [`README.md:200-235`](../../README.md#L200-L235), [`CREDENTIAL-SETUP-FLOW.md:7-27`](../../docs/guides/CREDENTIAL-SETUP-FLOW.md#L7-L27)
- StepFun is `STEPFUN_API_KEY` in the README and `.env.example`, but the canonical registry and constructor use `STEP_API_KEY`. `.env.example` also includes noncanonical `KIMI_API_KEY` while the registry uses `MOONSHOT_API_KEY`. [`README.md:225-233`](../../README.md#L225-L233), [`.env.example:1-34`](../../.env.example#L1-L34), [`catalog/registry/providers.go:72-81,230-240`](../../catalog/registry/providers.go#L72-L81)
- The credential guide's “adding a provider” checklist names only registry data, a live fetcher, and remote catalog metadata. Actual provider onboarding also touches compatibility configuration, runtime construction, provider config/credential inference, tests, env templates, and often protocol-specific logic. [`CREDENTIAL-SETUP-FLOW.md:80-85`](../../docs/guides/CREDENTIAL-SETUP-FLOW.md#L80-L85), [`catalog/live/fetchers.go:62-100`](../../catalog/live/fetchers.go#L62-L100), [`setup/deployment.go:183-383`](../../setup/deployment.go#L183-L383), [`provider/provider_registry.go:74-180`](../../provider/provider_registry.go#L74-L180)
- The docs index advertises planned retry/fallback and caching/audit guides, but those files do not exist. It also shows the OpenAPI file under `docs/api/` in its tree even though the tracked path is root-level `api/openapi.yaml`. [`docs/README.md:31-46`](../../docs/README.md#L31-L46)
- The host-boundary and decentralized-routing documents are unusually candid about secret isolation, last-good route behavior, missing persistence, absent consensus, and the lack of production load/fault-injection evidence. These are high-quality OSS trust documents and should be retained. [`HOST-ENGINE-BOUNDARY.md:108-127,156-193`](../../docs/architecture/HOST-ENGINE-BOUNDARY.md#L108-L127), [`DECENTRALIZED-FLUX.md:64-88`](../../docs/architecture/DECENTRALIZED-FLUX.md#L64-L88)
- The 469-line enterprise design is explicitly a proposed 45-engine-week product expansion into SSO/RBAC, dashboards, HQL, prompt management, canaries, A2A, fine-tuning, and SLA queues. It is not an executable current roadmap and would distract from adoption-critical work. [`FLUX-ENTERPRISE.md:1-22,303-379,442-465`](../../docs/design/FLUX-ENTERPRISE.md#L1-L22)

#### Examples and API documentation

- All three examples import lower-level `provider` and `provider/core`, directly contradicting the README's instruction that hosts use the stable `engine` facade. [`examples/basic/main.go:8-20`](../../examples/basic/main.go#L8-L20), [`examples/streaming/main.go:8-20`](../../examples/streaming/main.go#L8-L20), [`examples/multi-provider/main.go:10-25`](../../examples/multi-provider/main.go#L10-L25)
- The basic example always requests `claude-sonnet-4-6` even though it auto-detects the provider, and it dereferences `resp.Usage` without a nil check. [`examples/basic/main.go:17-35`](../../examples/basic/main.go#L17-L35)
- The streaming example is described as having auto-continuation but calls `StreamChat`, not `StreamChatContinue`; the explicit continuation API is a different method. [`examples/streaming/main.go:1-6,26-33`](../../examples/streaming/main.go#L1-L26), [`provider/chat.go:45-81`](../../provider/chat.go#L45-L81)
- The multi-provider example is manual error handling, not a Flux fallback chain, despite its description. [`examples/multi-provider/main.go:1-7,31-44`](../../examples/multi-provider/main.go#L1-L7)
- OpenAPI documents 11 paths, while the server registers 15. It omits `/ready`, `/rerank`, and `/v1/chat/completions`, even though README and architecture list all three. The prompt schema also omits implemented `system_prompt`, `max_tokens`, and `tools` fields. [`api/openapi.yaml:30-43,118-340`](../../api/openapi.yaml#L30-L43), [`internal/api/server.go:101-121,163-170`](../../internal/api/server.go#L101-L121)
- The OpenAPI version is a hand-maintained literal `0.0.1`; no workflow validates it against routes, implementation, or the module version. [`api/openapi.yaml:1-10`](../../api/openapi.yaml#L1-L10), [`ci.yml:1-300`](../../.github/workflows/ci.yml#L1-L300)
- The TypeScript and Python SDKs live under `internal/sdk`, making their current import paths unusable by external Go/Python/TypeScript consumers. The Go SDK is additionally a nested module excluded from root `go test ./...`. [`internal/sdk/go/go.mod:1-3`](../../internal/sdk/go/go.mod#L1-L3), [`internal/sdk/typescript/flux.ts:1-175`](../../internal/sdk/typescript/flux.ts#L1-L175), [`internal/sdk/python/flux.py:1-188`](../../internal/sdk/python/flux.py#L1-L188)
- The TypeScript package points directly at an unexported `FluxClient` source file, has no build/test scripts, and has no lockfile. The Python client has no tests and expects JSON from the node-delete endpoint even though the API returns 204. These are internal stubs, not distributable SDKs. [`internal/sdk/typescript/package.json:1-8`](../../internal/sdk/typescript/package.json#L1-L8), [`internal/sdk/python/flux.py:68-75`](../../internal/sdk/python/flux.py#L68-L75), [`api/openapi.yaml:223-237`](../../api/openapi.yaml#L223-L237)
- A fresh main-module test run reports only 52 packages and does not include the nested Go SDK. Manual `go test` inside that nested module passed, confirming the code exists but is outside the root gate. [`internal/sdk/go/go.mod:1-3`](../../internal/sdk/go/go.mod#L1-L3), [`go.mod:1-41`](../../go.mod#L1-L41)

#### GoDoc and public API maturity

- `engine/doc.go` clearly states the host boundary, and most engine DTOs have useful comments and an explicit event vocabulary. This is a good foundation for a stable package. [`engine/doc.go:1-14`](../../engine/doc.go#L1-L14), [`engine/types.go:62-84`](../../engine/types.go#L62-L84)
- The lower-level `core.Provider` is admirably small: `Chat`, `StreamChat`, `Ping`, and `Name`. [`provider/core/core.go:21-33`](../../provider/core/core.go#L21-L33)
- Public comments still contain stale ecosystem terms: `core` refers to “eagle/llm,” while `llm.Provider` describes itself as “rho's rho-owned view.” That is confusing in a standalone OSS package. [`provider/core/core.go:1-12`](../../provider/core/core.go#L1-L12), [`llm/provider.go:9-15`](../../llm/provider.go#L9-L15)
- A local `go list -f '{{if not .Doc}}...'` check found 17 packages without package documentation, including public `provider/adapters`, `provider/cache`, `provider/observability`, `provider/testkit`, `catalog/registry`, and `tools`. [`provider/adapters/provider_registry.go:1-12`](../../provider/adapters/provider_registry.go#L1-L12), [`catalog/registry/registry.go:1-18`](../../catalog/registry/registry.go#L1-L18)
- A local `go doc -all` count found approximately 146 exported declarations in `engine`, 134 in `provider/core`, 122 in `catalog`, 65 in `llm`, 58 in `router`, and 48 each in `provider` and `credentials`. There is no stability annotation for these packages beyond prose. [`engine/types.go:1-164`](../../engine/types.go#L1-L164), [`HOST-ENGINE-BOUNDARY.md:188-193`](../../docs/architecture/HOST-ENGINE-BOUNDARY.md#L188-L193)
- The `engine.ContractVersion == "2"` marker exists before a meaningful independent compatibility baseline: the current module has only `v0.0.1`, and no `apidiff` or previous-tag API comparison job exists. [`engine/engine.go:22-23`](../../engine/engine.go#L22-L23), [`ci.yml:1-300`](../../.github/workflows/ci.yml#L1-L300)

#### Contribution, security, community, and release processes

- Positive process assets exist: MIT license, conventional-commit guidance, contribution/security/code-of-conduct documents, issue forms, PR template, CODEOWNERS, lefthook, release workflow, Scorecard, and protected `main`. [`CONTRIBUTING.md:1-136`](../../CONTRIBUTING.md#L1-L136), [`SECURITY.md:1-71`](../../SECURITY.md#L1-L71), [`.github/PULL_REQUEST_TEMPLATE.md:1-59`](../../.github/PULL_REQUEST_TEMPLATE.md#L1-L59)
- The PR template contradicts the contribution guide about changelog ownership: the template says to update `CHANGELOG.md`, while the guide says release automation must generate it and contributors must not edit it. [`PULL_REQUEST_TEMPLATE.md:46-59`](../../.github/PULL_REQUEST_TEMPLATE.md#L46-L59), [`CONTRIBUTING.md:98-109`](../../CONTRIBUTING.md#L98-L109)
- The contribution guide says `make ci` runs “everything CI runs,” but the Makefile gate omits deadcode, duplication, secrets, markdown, gRPC-tag testing, and the cross-platform build matrix. Conversely, `make ci` mutates the tree through tidy and format. [`CONTRIBUTING.md:14-36`](../../CONTRIBUTING.md#L14-L36), [`Makefile:47-108`](../../Makefile#L47-L108), [`ci.yml:34-300`](../../.github/workflows/ci.yml#L34-L300)
- There are two CODEOWNERS files. The `.github/CODEOWNERS` file that GitHub normally discovers has stale `/client/` coverage and no ownership for current `provider/`, `engine/`, or `credentials/` paths; the root duplicate lists `/client/` as well. Team validity and actual maintainer availability cannot be verified from the public repository. [`.github/CODEOWNERS:1-22`](../../.github/CODEOWNERS#L1-L22), [`CODEOWNERS:1-20`](../../CODEOWNERS#L1-L20)
- `SECURITY.md` claims release checksums come from GoReleaser and that Ruff/Mypy/Python lockfiles run in CI. The actual release workflow only creates a GitHub release, there is no GoReleaser config, no Python/TypeScript CI, no Ruff/Mypy job, and no pnpm lockfile. [`SECURITY.md:45-57`](../../SECURITY.md#L45-L57), [`release.yml:1-31`](../../.github/workflows/release.yml#L1-L31)
- The README and contribution guide repeatedly claim release-please automation, but no release-please workflow/config is tracked. [`README.md:358-369`](../../README.md#L358-L369), [`CONTRIBUTING.md:38-42,98-107`](../../CONTRIBUTING.md#L38-L42)
- Branch protection is materially better than average: all 15 CI contexts are required, strict up-to-date checks are enabled, force pushes/deletions are blocked, and admin enforcement is on. However, no required approving review, conversation resolution, required signed commits, or required signed pushes appeared in the public branch-protection response. [branch protection API](https://api.github.com/repos/GrayCodeAI/flux/branches/main/protection), [`CONTRIBUTING.md:111-120`](../../CONTRIBUTING.md#L111-L120)
- GitHub reports vulnerability alerts, Dependabot security updates, secret scanning, and secret-scanning push protection disabled. TruffleHog in CI provides useful coverage, but it is not equivalent to repository-native alert and push-protection services. [repository security settings](https://api.github.com/repos/GrayCodeAI/flux), [`ci.yml:228-238`](../../.github/workflows/ci.yml#L228-L238)
- Git log verification found one human contributor across all commits; GitHub currently reports 3 stars, 0 forks, and 0 issues. This is a very young project with no public adoption or issue-triage history yet. [repository](https://github.com/GrayCodeAI/flux)
- `v0.0.1` is an annotated, signed tag; local `git tag -v` succeeded with an ED25519 signature. This is a real positive trust signal. The release workflow itself neither requires nor verifies that signature. [v0.0.1 release](https://github.com/GrayCodeAI/flux/releases/tag/v0.0.1), [`release.yml:15-31`](../../.github/workflows/release.yml#L15-L31)
- The sole release points to commit `34010bf`; local history shows 29 commits from `v0.0.1` to current `main`. The generated release notes list many unrelated PRs, while the checked-in changelog has conflicting ordering: `0.0.1` appears before a later-dated `0.1.0` section and includes pre-rename history. [v0.0.1 release](https://github.com/GrayCodeAI/flux/releases/tag/v0.0.1), [`CHANGELOG.md:35-50,175-235`](../../CHANGELOG.md#L35-L50)
- The immediately previous module was `github.com/GrayCodeAI/eyrie` at version `0.6.0`; the current module is `github.com/GrayCodeAI/flux` at `0.0.1`. The changelog mentions old module paths but supplies no migration guide with import mapping, state migration ordering, compatibility matrix, or rollback advice. [`go.mod:1-3`](../../go.mod#L1-L3), [`VERSION:1`](../../VERSION#L1), [`CHANGELOG.md:35-50`](../../CHANGELOG.md#L35-L50)
- `go list -m -versions github.com/GrayCodeAI/flux` returned only `v0.0.1`, confirming minimal current-module release history. [module versions](https://pkg.go.dev/github.com/GrayCodeAI/flux?tab=versions)
- `go list -m -u all` found many available updates, including OTel `1.44.0 -> 1.46.0`, gRPC `1.82.1 -> 1.84.0`, tokenizer `0.8.0 -> 0.8.1`, SQLite `1.51.0 -> 1.59.0`, and several `golang.org/x/*` updates. No Dependabot or Renovate configuration is tracked. [`go.mod:5-41`](../../go.mod#L5-L41)
- The versioning policy is external to Flux, in the Rho repository. An independently adoptable library should own its compatibility and support policy rather than requiring consumers to follow a sibling product's document. [`CONTRIBUTING.md:3-5`](../../CONTRIBUTING.md#L3-L5), [`SECURITY.md:9-11`](../../SECURITY.md#L9-L11)

### Inferences

- Flux should present one product: **a Go library for normalized, resilient calls to heterogeneous LLM providers**. HTTP/gRPC serving, conversation storage, distributed control plane, SDKs, budgets, and enterprise administration should either be separately versioned opt-in modules or removed from the core promise until they are real external surfaces. The current mixed story makes it difficult for an adopter to know what is supported, stable, and maintained. [`README.md:31-44,154-176`](../../README.md#L31-L44)
- The fresh-install catalog requirement is a release blocker, not a documentation nicety. A new user currently needs state prepared by an ecosystem host, while the default remote catalog is already schema-incompatible. A minimal embedded bootstrap or explicit `engine.Bootstrap(ctx)`/local-cache constructor is needed before broader release claims. [`engine/state.go:86-103`](../../engine/state.go#L86-L103), [`catalog/v1.go:444-460`](../../catalog/v1.go#L444-L460)
- The documentation should be made partly generated and continuously checked rather than manually expanded: provider/env tables from `ProviderSpec`, OpenAPI route parity from a public handler, examples as external-package tests, and release metadata from `VERSION`. This directly prevents the drift already present in 28-vs-15 provider counts, env-name drift, and OpenAPI omissions. [`catalog/registry/spec.go:31-74`](../../catalog/registry/spec.go#L31-L74), [`internal/api/server.go:101-121`](../../internal/api/server.go#L101-L121)
- The existing `v0.0.1` tag should not be followed by a `v1.0.0` jump. Publish a truthful `v0.1.0` “library foundation” release after the quickstart, engine middleware persistence, docs, and external-consumer smoke gate are fixed; use `v0.2.x` for extension seams and reserve `v1.0.0` for an audited compatibility baseline. [`VERSION:1`](../../VERSION#L1), [`release.yml:1-31`](../../.github/workflows/release.yml#L1-L31)
- The enterprise RFC should remain an RFC/design archive, not a public roadmap. The highest-return work is provider conformance, onboarding, deterministic releases, and operational evidence—not 45 engineer-weeks of adjacent product surface. [`FLUX-ENTERPRISE.md:303-379,442-465`](../../docs/design/FLUX-ENTERPRISE.md#L303-L379)

### Gaps

- No provider API keys or organization credentials were available, so this audit did not verify live model discovery, credential probes, wire compatibility, regional endpoints, or billing behavior against any hosted provider. Existing tests overwhelmingly use `httptest` and mocks. [`catalog/provider_live_parity_test.go:11-33`](../../catalog/provider_live_parity_test.go#L11-L33)
- Public GitHub metadata does not reveal whether the CODEOWNERS teams exist, whether Discussions are enabled, or how responsive the sole maintainer is. [repository metadata](https://github.com/GrayCodeAI/flux)
- The governance, ownership, signing-key custody, incident response, and release authority for the `langdag.com` catalog are not documented in this repository. [default catalog](https://langdag.com/model-catalog/v1/catalog.json)
- There is no public usage telemetry, support history, downstream adopter list, or production SLO evidence from which to assess real operability. Flux itself explicitly says distributed routing has no production load or fault-injection baseline. [`DECENTRALIZED-FLUX.md:64-88`](../../docs/architecture/DECENTRALIZED-FLUX.md#L64-L88)
- The TypeScript check was inconclusive because the installed global TypeScript/DOM libraries failed before yielding a source-specific result; this should not be attributed to Flux without a pinned toolchain run. [`internal/sdk/typescript/tsconfig.json:1-11`](../../internal/sdk/typescript/tsconfig.json#L1-L11)

## 2. Test maturity, critical-path confidence, and extension ergonomics

### Takeaway

Flux's deterministic test suite is a major strength: 2,162 test functions across 271 files, extensive provider/catalog coverage, a real race pass, 65.6% aggregate coverage, and 75 benchmarks. Confidence drops sharply at the boundaries that matter to adopters—the clean-machine quickstart, current remote catalog, actual `engine` transport composition, cross-request middleware state, real provider behavior, nested SDKs, API compatibility, distributed processes, and performance/soak behavior.

### Cited Findings

#### Verified command results

| Check | Result on 2026-09-24 | Interpretation |
|---|---|---|
| `go version && go mod verify` | Go `1.26.6`; all modules verified | Dependency integrity for the checked-in graph is good. [`go.mod:1-41`](../../go.mod#L1-L41) |
| `go test -race -count=1 -shuffle=on -coverprofile=... -covermode=atomic ./...` | All main-module packages passed; aggregate coverage **65.6%** | Strong deterministic/race health; coverage is only 5.6 points above the CI floor. [`ci.yml:138-172`](../../.github/workflows/ci.yml#L138-L172) |
| `go vet ./...` | Passed with no output | Compiler/static baseline is clean. [`ci.yml:101-114`](../../.github/workflows/ci.yml#L101-L114) |
| `go test -tags=grpc -count=1 ./...` | Passed, including `internal/grpc` | The optional gRPC build works, although untagged CI does not run it. [`internal/grpc/server_grpc.go:1-3`](../../internal/grpc/server_grpc.go#L1-L3) |
| `govulncheck ./...` | `No vulnerabilities found` | No currently reachable known Go vulnerability was found; repository-native alerts remain disabled. [`ci.yml:174-194`](../../.github/workflows/ci.yml#L174-L194), [repository security settings](https://api.github.com/repos/GrayCodeAI/flux) |
| `gofumpt -l . && goimports -l .` | No files listed | Formatting matches the pinned tools. [`ci.yml:35-68`](../../.github/workflows/ci.yml#L35-L68) |
| `golangci-lint v2.1.0 run --timeout=5m` | Reported `0 issues`, then the local run timed out | Treat lint as locally inconclusive; the latest GitHub CI run completed successfully. [latest CI run](https://github.com/GrayCodeAI/flux/actions/runs/35504614597) |
| Nested Go SDK `go test ./... && go vet ./...` | Passed | Useful tests exist but are outside the main CI/module gate. [`internal/sdk/go/go.mod:1-3`](../../internal/sdk/go/go.mod#L1-L3) |
| Python `ruff check . && mypy flux.py` | Ruff found two unused imports; mypy passed | The stub is not under an enforced Python gate. [`internal/sdk/python/flux.py:1-9`](../../internal/sdk/python/flux.py#L1-L9) |
| TypeScript `tsc --noEmit` | Failed in installed DOM library declarations | Inconclusive source-quality result; no pinned project toolchain runs in CI. [`internal/sdk/typescript/tsconfig.json:1-11`](../../internal/sdk/typescript/tsconfig.json#L1-L11) |

#### Test volume and distribution

- The repository contains **271 `*_test.go` files, 2,162 `Test*` functions, 4 fuzz targets, and 75 benchmark functions**. Test files total about 56,127 lines versus 46,869 production Go lines. This is excellent test investment. [`fuzz_test.go:1-134`](../../provider/fuzz_test.go#L1-L134), [`benchmarks_test.go:1-281`](../../provider/benchmarks_test.go#L1-L281)
- Test functions are concentrated in `provider` (1,009), `catalog` (309), `internal` (170), `config` (135), `runtime` (121), `engine` (91), `setup` (84), `credentials` (74), and `router` (68). This is appropriate for a provider runtime, but the stable engine is not the largest test domain. [`engine/contract_e2e_test.go:1-184`](../../engine/contract_e2e_test.go#L1-L184)
- There are 191 test files using `t.Parallel` and 69 using `httptest`, showing substantial concurrency and wire-protocol simulation. [`stream_test.go:17-113`](../../provider/core/stream_test.go#L17-L113)
- Only six true Go `Example*` functions exist, all under `types` and one adaptive-rate-limit example; there are no executable engine quickstart examples. [`types/example_test.go:1-56`](../../types/example_test.go#L1-L56), [`adaptive_ratelimit_test.go:501-534`](../../provider/resilience/adaptive_ratelimit_test.go#L501-L534)
- One circuit-breaker timing test is explicitly skipped as flaky, with a manual deterministic replacement. This is transparent, but it identifies remaining wall-clock sensitivity. [`circuitbreaker_test.go:100-110`](../../router/circuitbreaker_test.go#L100-L110)

#### Coverage strengths

- Core provider wire tests cover Anthropic, OpenAI-compatible behavior, cloud transports, streaming, tools, thinking, usage, retries, errors, and security behavior using realistic local servers. The shared SSE tests cover multiline data, cancellation, tool calls, thinking deltas, and TTFT. [`stream_test.go:17-250`](../../provider/core/stream_test.go#L17-L250)
- Catalog tests are extensive across provider registration, protocol matrices, deployment env derivation, cache loading, live parsing, deprecation, and topology. [`provider_live_parity_test.go:11-84`](../../catalog/provider_live_parity_test.go#L11-L84), [`protocol_matrix_test.go:27-87`](../../catalog/registry/protocol_matrix_test.go#L27-L87)
- Security tests are unusually broad for secret persistence, state sanitization, atomic migration, environment conflict detection, keyring cancellation behavior, and no-secret-leak assertions. [`state_security_test.go:1-220`](../../engine/state_security_test.go#L1-L220), [`keyring_platform_test.go:12-161`](../../credentials/keyring_platform_test.go#L12-L161)
- Distributed control-plane tests validate atomic last-good application, revision conflicts, signed manifest tampering, and periodic refresh. They use an in-memory `RoundTripper`, not TLS or separate processes. [`controlplane_test.go:64-196,198-313`](../../router/controlplane/controlplane_test.go#L64-L196)
- A public `verify` package describes a behavioral conformance harness with canonical chat/tool cases and baseline diffs. In practice, its only uses are its own fake-provider tests; no adapter, provider, workflow, or example invokes it. [`verify/verify.go:1-206`](../../verify/verify.go#L1-L206), [`verify/cases.go:1-46`](../../verify/cases.go#L1-L46), [`verify_test.go:13-149`](../../verify/verify_test.go#L13-L149)
- The engine contract E2E test is valuable for normalized DTO/event behavior, but it replaces the private `resolveTransport` with a mock. It does not exercise catalog-to-adapter construction, real routing, middleware, credentials, or the default transport. [`engine/contract_e2e_test.go:43-113`](../../engine/contract_e2e_test.go#L43-L113)
- HTTP “integration” tests run an `httptest.Server` with local SQLite and mock providers. They are solid component integration tests, not deployable end-to-end tests. [`integration_test.go:20-116`](../../internal/api/integration_test.go#L20-L116)

#### Coverage gaps and weak critical paths

- Aggregate coverage hides uneven critical areas: `provider/resilience` is 28.3%, `provider/media` 26.2%, `provider/observability` 39.1%, `provider/extraction` 44.1%, `operationsgraph` 44.1%, `graph` 12.7%, untagged `internal/grpc` 20.0%, and the root/`llm`/testkit packages report 0% in their own package runs. These numbers came from the verified race/coverage command and align with the package layout. [`operationsgraph/projection.go:1-200`](../../operationsgraph/projection.go#L1-L200), [`provider/resilience/roles.go:1-120`](../../provider/resilience/roles.go#L1-L120)
- The CI coverage command does not use `-coverpkg=./...`, despite an audit plan claiming that was done. Its 65.6% gate is therefore vulnerable to package-boundary accounting and does not enforce minimums for reliability, engine transport, or operations-critical packages. [`ci.yml:154-166`](../../.github/workflows/ci.yml#L154-L166), [`audit-remediation.md:272-279`](../../docs/plans/audit-remediation.md#L272-L279)
- There is no test for a fresh `engine.New(...).Stream(...)` path with an empty temporary `StateDir`; tests explicitly write `catalog.SeedCatalog()` first. This misses the README's primary adoption path. [`engine/contract_e2e_test.go:43-62`](../../engine/contract_e2e_test.go#L43-L62)
- There is no CI test against the current default remote catalog. The remote-source tests inject local HTTP clients/servers, so a producer schema addition can break production while CI remains green. [`v1_test.go:83-234`](../../catalog/v1_test.go#L83-L234), [default catalog](https://langdag.com/model-catalog/v1/catalog.json)
- No engine test invokes `defaultTransport`, `DeploymentProviderFromState`, or verifies that cache/rate-limit/circuit-breaker state survives multiple requests. Searches found only tests of DTO conversion and injected private transports. [`engine/engine.go:283-327`](../../engine/engine.go#L283-L327), [`contract_e2e_test.go:61-62`](../../engine/contract_e2e_test.go#L61-L62)
- Fuzzing covers only message sanitation, role merging, cache-key determinism, and guardrails. It does not target SSE framing, JSON decoding, retry-after parsing, auth redirect handling, catalog parsing, state migration, tool-argument accumulation, or stream cancellation. [`fuzz_test.go:10-134`](../../provider/fuzz_test.go#L10-L134), [`ci.yml:253-273`](../../.github/workflows/ci.yml#L253-L273)
- Benchmarks exist for cache keys, request building, sanitization, guardrails, metrics, circuit breakers, and adaptive limiting, but CI never runs them and no committed baseline, `benchstat` comparison, regression threshold, or published result exists. [`Makefile:73-74`](../../Makefile#L73-L74), [`ci.yml:34-300`](../../.github/workflows/ci.yml#L34-L300)
- No test starts separate Flux processes, serves TLS, restarts a replica from persisted state, injects network partitions/partitions clocks, or runs a soak/load profile. The distributed-routing document acknowledges these missing tests. [`DECENTRALIZED-FLUX.md:64-88`](../../docs/architecture/DECENTRALIZED-FLUX.md#L64-L88)
- The only Go compatibility axis is one exact toolchain, Go `1.26.6`; CI varies OS/architecture but not supported Go versions or the previous released API. [`ci.yml:21-23,275-300`](../../.github/workflows/ci.yml#L21-L23)
- The nested Go SDK tests are not run by root CI. No workflow tests Python or TypeScript. The main CI has no clean external-module `go get`/compile test against the published tag. [`internal/sdk/go/go.mod:1-3`](../../internal/sdk/go/go.mod#L1-L3), [`ci.yml:1-300`](../../.github/workflows/ci.yml#L1-L300)
- No test compares OpenAPI paths/schemas to registered routes, and no API compatibility tool checks exported Go symbols against the prior release. [`api/openapi.yaml:118-340`](../../api/openapi.yaml#L118-L340), [`internal/api/server.go:101-121`](../../internal/api/server.go#L101-L121)

#### Critical functional gaps exposed by test architecture

- `Engine` stores no memoized transport. Every generation calls `defaultTransport`, which reloads state, builds a new deployment router, and wraps it in newly allocated rate-limit/cache decorators. Circuit-breaker, rate-window, in-flight, and cache state therefore reset each call. [`engine/engine.go:54-68,283-327`](../../engine/engine.go#L54-L68), [`setup/deployment.go:94-110`](../../setup/deployment.go#L94-L110)
- Even if the cache wrapper were retained, `NewCachedProvider` claims zero config fields are replaced by defaults but copies `cfg.Enabled` directly; a zero `CacheConfig` is disabled. Thus `engine.Options{EnableCaching: true}` with the documented zero config does not cache. [`engine/engine.go:45-51,323-325`](../../engine/engine.go#L45-L51), [`provider/cache/semantic_cache.go:15-38,73-92`](../../provider/cache/semantic_cache.go#L15-L38)
- `Engine` describes its cache as semantic, but the wrapper it creates is deterministic hash/LRU caching. A real embedding-similarity cache exists in `provider/embeddings`, but is not wired into `engine.Options`. [`engine/engine.go:45-51`](../../engine/engine.go#L45-L51), [`provider/cache/semantic_cache.go:52-68,296-345`](../../provider/cache/semantic_cache.go#L52-L68), [`provider/embeddings/cache.go:17-90`](../../provider/embeddings/cache.go#L17-L90)
- The lower-level transport has a shared `http.Transport`, but no proxy support, forced HTTP/2 setting, response-header timeout, or custom redirect policy. Tests assert pool identity and a few timeout values, not proxy, redirects, auth-header stripping, or response-header stalls. [`provider/core/transport.go:23-64`](../../provider/core/transport.go#L23-L64), [`transport_test.go:9-84`](../../provider/core/transport_test.go#L9-L84)
- The SSE parser has useful unit tests, but the default 2 MiB scanner cap, idle/stall behavior, cross-host redirect behavior, and cross-request resource usage are not covered by contract or performance tests. [`provider/core/stream.go:22-89`](../../provider/core/stream.go#L22-L89), [`stream_test.go:17-113`](../../provider/core/stream_test.go#L17-L113)

#### Extension-seam assessment

| Extension target | Current seam | Assessment |
|---|---|---|
| OpenAI-compatible provider/gateway | `FluxClient.RegisterCustomProvider` and `engine.Options.CustomGateways` validate names/URLs and remain instance-local. [`provider/dynamic.go:11-50`](../../provider/dynamic.go#L11-L50), [`engine/host_runtime.go:19-106`](../../engine/host_runtime.go#L19-L106) | **Good.** This is the strongest extension path. |
| Credential backend | Three-method `credentials.Store` is injected through `engine.Options.SecretStore`. [`credentials/store.go:27-32`](../../credentials/store.go#L27-L32), [`engine/engine.go:25-38`](../../engine/engine.go#L25-L38) | **Good core interface**, but service naming remains a mutable process global and provider state is string-account based. [`credentials/store.go:11-25`](../../credentials/store.go#L11-L25) |
| Lower-level HTTP transport | Adapters accept `core.WithHTTPClient`; adapters implement a common configurable setter surface. [`provider/core/options.go:9-24,66-94`](../../provider/core/options.go#L9-L24) | **Good for advanced users**, but not exposed through `engine.Options`. |
| Custom engine transport | The hook is the unexported `Engine.resolveTransport` function field, mutated only by same-package tests. [`engine/engine.go:54-68,283-300`](../../engine/engine.go#L54-L68) | **Poor.** Independent adapters cannot participate in normal engine resolution. |
| Native provider | `ProviderSpec`, compatibility maps, live fetcher registry, `setup` switch, `provider` switch, config inference, env docs, and tests are separate edits. [`catalog/registry/providers.go:16-342`](../../catalog/registry/providers.go#L16-L342), [`catalog/live/fetchers.go:62-100`](../../catalog/live/fetchers.go#L62-L100), [`setup/deployment.go:183-383`](../../setup/deployment.go#L183-L383) | **Poor-to-moderate.** Metadata is declarative, construction is not. |
| Model | Live provider fetchers plus a remote `model-catalog/v1` document. [`catalog/live/fetchers.go:52-100`](../../catalog/live/fetchers.go#L52-L100), [`catalog/v1.go:44-67`](../../catalog/v1.go#L44-L67) | **Weak for ecosystem contributors.** No in-repo generator/publication/signature workflow; strict decoding makes producer changes brittle. |
| Routing strategy | String enum plus a central `switch`; no strategy interface. [`router/strategy.go:10-31,107-168`](../../router/strategy.go#L10-L31) | **Poor for third-party extension.** New strategies require modifying Flux. |
| Cache backend | `CacheBackend` and a Redis skeleton exist only under `internal/cache`; neither is wired into engine caching. [`internal/cache/backend.go:23-33,97-139`](../../internal/cache/backend.go#L23-L33) | **Not a public extension seam.** Redis explicitly lacks pooling, auth, DB selection, and reconnection guarantees. |
| Audit sink | `AuditSink` and JSONL sink exist only under `internal/observability` and have no production caller outside their own tests. [`internal/observability/audit.go:1-100`](../../internal/observability/audit.go#L1-L100) | **Not a public extension seam and currently inert.** |
| Telemetry | A public lower-level OTel `TracingProvider` exists, but engine does not wire it and it emits custom `provider.name`/`usage.*` attributes rather than the repository's `gen_ai.*` vocabulary. [`provider/observability/tracing.go:12-68`](../../provider/observability/tracing.go#L12-L68), [`internal/observability/genai_semconv.go:1-80`](../../internal/observability/genai_semconv.go#L1-L80) | **Partial.** Suitable for manual lower-level wrapping, not a coherent engine sink contract. |
| HTTP/gRPC server | Implementations are internal and therefore unavailable to ordinary external importers. [`internal/api/server.go:1-16`](../../internal/api/server.go#L1-L16), [`internal/grpc/grpc.go:1-120`](../../internal/grpc/grpc.go#L1-L120) | **Not adoptable as a standalone OSS service today.** |

### Inferences

- The suite currently provides strong confidence that individual packages behave as their same-package unit tests define them. It provides only moderate confidence that the stable `engine` composition retains state, a fresh user can start, or adapters agree with current hosted APIs. The missing confidence is concentrated at seams, not inside leaf functions. [`engine/contract_e2e_test.go:43-113`](../../engine/contract_e2e_test.go#L43-L113)
- A percentage-only 60% coverage gate is not the best next step. Add package-specific floors for `engine/defaultTransport`, `provider/core` transport/SSE, `router`, `credentials`, and catalog parsing, plus one clean-machine external consumer test; do not spend the next cycle chasing aggregate coverage in peripheral packages. [`ci.yml:154-166`](../../.github/workflows/ci.yml#L154-L166)
- The existing `verify` package should become the common adapter contract suite. Every built-in adapter should run the same sanitized fixture corpus for blocking chat, streaming, tools, usage, errors, cancellation, and prompt/provider-block replay. Live provider calls should be a separate opt-in/nightly job, not the deterministic PR gate. [`verify/verify.go:1-15`](../../verify/verify.go#L1-L15)
- Provider breadth should become support-tiered rather than expanded blindly: Tier A for Anthropic/OpenAI/Gemini/Azure/Bedrock/Vertex with nightly live conformance; Tier B for compatible gateways with deterministic wire fixtures; custom OpenAI-compatible endpoints remain user responsibility. This improves trust more than adding another gateway name. [`catalog/provider_live_parity_test.go:11-33`](../../catalog/provider_live_parity_test.go#L11-L33)
- Routing, cache, telemetry, and native provider extension should be unified behind a small set of public interfaces or explicitly declared non-extensible. Today several README features are implemented as isolated lower-level or internal components without a reachable composition path. [`router/strategy.go:107-168`](../../router/strategy.go#L107-L168), [`internal/cache/backend.go:23-33`](../../internal/cache/backend.go#L23-L33), [`internal/observability/audit.go:38-41`](../../internal/observability/audit.go#L38-L41)
- The first performance baseline should be a reproducible client benchmark against a local fake provider and local OpenAI-compatible server, measuring construction, catalog load, keychain/cache hit/miss, stream TTFT overhead, allocations, and concurrency. It should not attempt GPU/provider throughput ranking. Flux's own distributed-routing document correctly calls for measured load/soak evidence. [`DECENTRALIZED-FLUX.md:81-88`](../../docs/architecture/DECENTRALIZED-FLUX.md#L81-L88)

### Gaps

- No live-provider credentials were available, so current hosted API compatibility, model availability, regional routing, auth-header behavior, and provider-specific error shapes were not empirically validated. [`catalog/live/fetchers.go:20-36`](../../catalog/live/fetchers.go#L20-L36)
- No prior API baseline exists for `apidiff`, and the current release is too young to distinguish intentional pre-1.0 breakage from regressions. [module versions](https://pkg.go.dev/github.com/GrayCodeAI/flux?tab=versions)
- The test suite does not reveal production call volume, concurrency, memory profile, long-lived stream count, or secret-store latency. The 102-second race run and unit benchmarks are not operating evidence. [`Makefile:58-74`](../../Makefile#L58-L74)
- The TypeScript result is environment-inconclusive; a pinned Node/npm/TypeScript matrix is needed before drawing a source-quality conclusion. [`internal/sdk/typescript/package.json:1-8`](../../internal/sdk/typescript/package.json#L1-L8)
- Chaos, restart, persistence, and TLS behavior of distributed routing remain unverified beyond in-process mocks. [`router/controlplane/controlplane_test.go:131-196`](../../router/controlplane/controlplane_test.go#L131-L196)

## 3. Smallest high-leverage public API and release strategy

### Takeaway

The smallest credible OSS contract is not the current 50-plus-package, seven-facet facade. It is a root-level Flux client with `New`, `Generate`, `Stream`, and `Close`, provider-neutral request/response/stream/error DTOs, injected secrets, catalog/state, OpenAI-compatible gateways, and a public transport resolver; `core.Provider` remains the extension port. Publish this as `v0.1.0` only after a clean-machine consumer test and engine-state fix, then automate compatibility, provenance, dependency updates, and conformance before `v1.0`.

### Cited Findings

- Flux already has the right generation primitives: `engine.New`, `Engine.Generate`, `Engine.Stream`, a pull-based `EventStreamer`, and typed errors. These are the natural core of a smaller public API. [`engine/engine.go:22-23,70-75,126-188`](../../engine/engine.go#L22-L188), [`llm/provider.go:26-43`](../../llm/provider.go#L26-L43), [`engine/errors.go:10-68`](../../engine/errors.go#L10-L68)
- Flux also already has the right extension port: the four-method concurrent-safe `core.Provider`. [`provider/core/core.go:21-33`](../../provider/core/core.go#L21-L33)
- What prevents a small API today is that the canonical host port bundles generation with model catalog, credential management, selection, gateway inspection, maintenance, and compaction; the stable package also re-exports setup/media/credential helpers beyond that port. [`llm/provider.go:16-24,137-152,201-212,244-281,317-328,388-402`](../../llm/provider.go#L16-L24), [`engine/types.go:94-164`](../../engine/types.go#L94-L164)
- The release system currently begins only after a human pushes a `v*` tag and then generates notes; it has no release PR, changelog/version update, API compatibility gate, or module-proxy smoke check. [`release.yml:1-31`](../../.github/workflows/release.yml#L1-L31)
- The repository's own audit plan identifies the highest-value missing work as conformance fixtures, README/GoDoc reconciliation, API diff, dependency/security/release operations, and a transport memoization fix, but that plan is not a completed release policy. [`audit-remediation.md:256-279,293-309`](../../docs/plans/audit-remediation.md#L256-L279)

### Inferences

#### 1. Recommended smallest stable API

Move the primary public import to the root module for discoverability, while keeping `engine` as a temporary compatibility alias if needed. The stable v1 candidate should conceptually be:

```go
package flux

type Options struct {
    Secrets       SecretStore
    StateDir      string
    Catalog       CatalogSource
    Gateways      []OpenAICompatibleGateway
    Transport     TransportResolver
}

type Client struct { /* implementation */ }

func New(Options) (*Client, error)
func (*Client) Generate(context.Context, Request) (Response, error)
func (*Client) Stream(context.Context, Request) (*Stream, error)
func (*Client) Close() error
```

The stable DTO set should be only:

- `Request`, `Response`, `Stream`, `Event`, `Route`, `Usage`, `Error`;
- capability/preference/limits needed to make routing decisions;
- `SecretStore` (`Get`, `Set`, `Delete`);
- `TransportResolver` as the advanced adapter seam;
- `OpenAICompatibleGateway` for the common custom endpoint.

This is smaller than the current `llm.Provider`, while preserving the existing pull-based stream and typed error design. The current generation API and `core.Provider` already supply most of the required shapes. [`engine/types.go:7-92,142-164`](../../engine/types.go#L7-L92), [`llm/provider.go:26-43,45-135`](../../llm/provider.go#L26-L43)

Keep setup/control APIs concrete and secondary, for example on a `Controller` returned by an optional `NewController` or in a separate `engine/setup` package. Do not make every host implement a mega-interface merely to configure credentials. [`llm/provider.go:137-152,201-212,244-328`](../../llm/provider.go#L137-L152)

#### 2. Stability tiers

| Tier | Packages | Policy |
|---|---|---|
| Stable | root `flux`, generation DTOs/errors/stream, `SecretStore`, `TransportResolver` | SemVer-protected; additions may be minor, removals/semantic changes major; checked with `apidiff`. |
| Advanced | `provider/core`, decorator interfaces, selected catalog/router types | Explicitly documented extension surface; slower-moving, tested against compatibility fixtures. |
| Experimental | current `catalog`, `router`, `provider/*`, conversation/storage/control-plane packages | No stability promise until a use case and external consumer justify promotion. |
| Internal / separate module | HTTP, gRPC, SDKs, distributed control plane | Either move to independently versioned modules or stop presenting them as core Flux deliverables. |

This follows the repository's own observation that lower-level packages are public for staged migration but not the Rho product boundary. [`HOST-ENGINE-BOUNDARY.md:188-193`](../../docs/architecture/HOST-ENGINE-BOUNDARY.md#L188-L193)

#### 3. Provider and extension redesign

- Replace the current metadata/factory split with one provider plugin descriptor owned by a dependency-neutral registry: identity, credential metadata, protocol, factory, model lister, compatibility flags, and conformance tier. The catalog registry should consume this descriptor rather than requiring parallel edits in `setup` and `provider`. Current duplication is visible across `ProviderSpec`, live `Registry`, compatibility maps, and the two construction switches. [`catalog/registry/spec.go:31-74`](../../catalog/registry/spec.go#L31-L74), [`catalog/live/fetchers.go:62-100`](../../catalog/live/fetchers.go#L62-L100), [`setup/deployment.go:183-383`](../../setup/deployment.go#L183-L383), [`provider/provider_registry.go:74-180`](../../provider/provider_registry.go#L74-L180)
- Define a `Selector` interface for routing strategies rather than a string switch. Keep weighted/rendezvous/least-request built-ins; do not expose mutable global strategy registries. [`router/strategy.go:107-168`](../../router/strategy.go#L107-L168)
- Either promote and productionize cache/telemetry extension contracts or remove the corresponding README claims. The current internal Redis skeleton and audit sink should not be marketed as distributed/auditable infrastructure. [`internal/cache/backend.go:97-139`](../../internal/cache/backend.go#L97-L139), [`internal/observability/audit.go:38-100`](../../internal/observability/audit.go#L38-L100)
- Use the OpenTelemetry API and `log/slog` as the default extension standards. A custom telemetry DTO/sink adds surface without ecosystem value; a wrapped `core.Provider` remains available for advanced decorators. [`provider/observability/tracing.go:12-68`](../../provider/observability/tracing.go#L12-L68)
- Make model publication reproducible: an in-repo generator, source manifest, schema compatibility test, signed release artifact or digest, last-known-good cache, and tolerant decoding of additive fields. The current default-source schema mismatch demonstrates why this is adoption-critical. [`catalog/v1.go:44-50,444-460`](../../catalog/v1.go#L44-L50), [default catalog](https://langdag.com/model-catalog/v1/catalog.json)

#### 4. Release strategy

**Phase A — truth release (`v0.1.0`, 1–2 weeks)**

1. Make a clean `HOME`/temporary state execute a complete mock-backed quickstart without `rho` commands.
2. Fix or replace the default catalog source; test its current schema and publish a signed/digested catalog artifact.
3. Memoize the engine transport and preserve router/breaker/cache/rate state; either fix or remove broken engine options.
4. Rewrite all examples through the stable API and add them as external-package compile tests.
5. Reconcile provider/env docs and either extract or clearly mark HTTP/gRPC/SDK features as internal/opt-in.
6. Remove false release/security claims and archive the enterprise RFC as non-roadmap design.
7. Add a clean external consumer job that creates a temporary module, `go get github.com/GrayCodeAI/flux@<candidate>`, compiles `New/Generate/Stream`, and runs a mock provider.

**Release acceptance gates:**

- candidate tag equals `VERSION` and embedded `flux.Version`;
- full race suite, gRPC-tag build, `go vet`, lint, format, `govulncheck`, and module verification pass;
- `apidiff old-tag candidate` has no unapproved incompatible changes;
- README/examples are executable tests;
- OpenAPI is either public and route-validated or removed from the core product promise;
- default catalog parses in CI and has a signed/digested source;
- signed annotated tag is verified by the release job;
- SBOM and GitHub build provenance/attestation are attached to the release;
- module proxy resolution is verified before announcement.

**Phase B — extension release (`v0.2.x`, 3–6 weeks)**

- introduce root stable API and public `TransportResolver`/provider plugin registry;
- run shared conformance fixtures for the Tier-A providers;
- enable opt-in nightly live-provider tests with redacted cassettes and budget limits;
- add Dependabot/Renovate, vulnerability alerts, secret scanning/push protection, and a Go 1.26 patch/minor compatibility matrix;
- publish API diffs and package stability labels;
- establish a small local-server benchmark/soak baseline and publish machine-readable results.

**Phase C — 1.0 gate (after at least 90 days or three real design partners)**

- no unresolved compatibility exceptions in the supported API;
- documented support window and security policy owned inside Flux, not delegated to Rho;
- reproducible release from a clean clone;
- provider support matrix with conformance tier and last-verified date;
- migration guide from Eyrie and from the current pre-1.0 API;
- one or more downstream maintainers and a documented release/security succession path.

The current `v0.0.1` module and recent rename make this the cheapest moment to narrow the API; after more consumers adopt the current facade, narrowing becomes materially harder. [`CHANGELOG.md:35-50`](../../CHANGELOG.md#L35-L50)

#### 5. Prioritized practical roadmap

| Priority | Change | Why it matters | Done when |
|---|---|---|---|
| P0 | Fresh-state quickstart + external consumer test | First five-minute adoption gate | A clean temporary environment runs a mock-backed engine example with no Rho CLI or pre-existing files. |
| P0 | Fix default catalog compatibility and trust | Prevents startup failure and untrusted metadata dependence | CI validates current artifact; schema additions are tolerated; artifact has publisher identity/digest. |
| P0 | Persist engine transport and stateful middleware | Makes retry/rate/cache/circuit claims real | Two-request tests prove cache hit, retained breaker state, and shared rate window through `Engine`. |
| P0 | Truthful docs/examples/API scope | Prevents users depending on inaccessible or nonfunctional surfaces | Every README claim maps to an external import/run/test; internal SDK/server claims are removed or extracted. |
| P0 | Automated release PR + compatibility gate | Turns one signed snapshot into a repeatable OSS process | A conventional merge can produce a version/changelog PR and candidate tag with no manual file edits. |
| P1 | Provider plugin/factory consolidation | Lowers contribution cost and drift | A new OpenAI-family provider requires one descriptor, one adapter/lister, and tests—not parallel switches. |
| P1 | Shared conformance suite | Builds provider trust | Tier-A adapters pass identical sanitized chat/stream/tool/usage/error/cancellation fixtures. |
| P1 | Real TLS/process/restart/soak test | Establishes distributed-routing confidence | A signed manifest is served over TLS, fetched by another process, survives peer outage, and restarts from a persisted last-good snapshot—or the feature remains experimental. |
| P1 | Performance baseline | Prevents “production-ready” by assertion | Reproducible local benchmark reports allocations, latency, concurrency, and stream behavior with committed result schema. |
| P2 | HTTP/gRPC/SDK extraction | Makes optional surfaces genuinely adoptable | Each is a public, tested, independently documented module with release ownership—or is deleted. |
| Defer | Enterprise org/RBAC/dashboard/HQL/A2A/SLA | High scope, low confidence return for a provider runtime | Keep as RFC only until external demand justifies it. [`FLUX-ENTERPRISE.md:442-465`](../../docs/design/FLUX-ENTERPRISE.md#L442-L465) |

### Gaps

- The recommended API is an engineering judgment, not a validated demand study. Before freezing it, interview a small set of intended Go adopters and test the `New/Generate/Stream` shape in two external applications. The current repository has only one human contributor and no public issue history. [repository](https://github.com/GrayCodeAI/flux)
- A `v0.1.0` date and effort estimate depend on maintainer capacity and whether the catalog publication infrastructure is available; neither can be established from the public repository. [`DECENTRALIZED-FLUX.md:64-88`](../../docs/architecture/DECENTRALIZED-FLUX.md#L64-L88)
- GitHub artifact attestations and SBOM tooling were not selected or tested in this audit; they should be chosen based on the repository's actual release mechanics rather than treated as already available. [`release.yml:1-31`](../../.github/workflows/release.yml#L1-L31)
- The exact stable/advanced package split needs an `apidiff` inventory and downstream-import census before deprecating any current package. The current tree already has ecosystem consumers beyond the documented Rho boundary. [`HOST-ENGINE-BOUNDARY.md:188-193`](../../docs/architecture/HOST-ENGINE-BOUNDARY.md#L188-L193)
