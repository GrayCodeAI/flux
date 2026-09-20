# Feature-oriented Flux architecture

Status: active refactor. This is a feature-oriented Go module, not a monorepo;
Rho remains a separate repository in the parent workspace.

Flux is one Go module with explicit feature packages. A feature owns its
contract, implementation, tests, and documentation in one directory. Packages
must have one direction of dependency; convenience re-export packages are not
architecture.

## Target shape

```text
engine/                 host-facing composition and normalized API
llm/                    host-facing request/response contracts
catalog/                model metadata and discovery
catalog/capabilities/   model capability and deprecation policy
credentials/            secret storage and safe status
router/                 selection and deployment policy
router/controlplane/    versioned peer manifests and local replicas
runtime/                host-facing construction
provider/               provider runtime composition root
provider/core/          provider-neutral contracts and wire primitives
provider/adapters/      one adapter family per provider protocol
provider/embeddings/    embedding capability
provider/cache/         exact and semantic response caching
provider/resilience/    retry, fallback, rate limits, health, coalescing
provider/media/         image, audio, moderation, structured output
provider/extraction/    typed relationship and knowledge-graph extraction
provider/observability/ usage, tracing, recording, callbacks
provider/testkit/       deterministic mock providers for tests and examples
conversation/           conversation graph and persistence
internal/api/           HTTP and gRPC delivery only
examples/               executable consumers of engine
```

The current branch has completed the hard package boundary: the old `client`
package path and production alias facade are gone; the provider runtime is now
under `provider/`.
`provider/core`, `provider/adapters`, `provider/embeddings`,
`provider/resilience`, `provider/cache`, `provider/media`, `provider/batch`, and
`provider/observability` are feature packages. The remaining files at
`provider/` are the composition root and its provider-client operations; they
are not protocol adapters or cross-feature implementations.

## Rules

1. Hosts import `engine`, `llm`, `graph`, and `tools`; they do not assemble
   provider internals.
2. `provider/core` imports no provider package.
3. A provider feature may depend on `provider/core` and shared standard-library
   code, but not on another provider feature.
4. Provider adapters translate wire protocols. They do not own routing,
   caching, credentials, or product semantics.
5. New cross-feature behavior is composed in `provider/`, not added to
   package-global registries. Custom OpenAI-compatible providers are now
   registered on a `FluxClient` instance only.
6. No compatibility aliases or deprecated import paths ship in production.
   Root-package test helpers may use local aliases while tests are migrated;
   they are not part of the module API.
7. Each feature directory contains production code and table-driven tests;
   package boundaries are checked in CI.

## Migration order

1. Rename the legacy `client` domain to `provider` (complete on this branch).
2. Move resilience decorators, health, roles, condensation, and their tests to
   `provider/resilience` (done).
3. Move exact and semantic response caching to `provider/cache` (done); move
   cache analytics with observability.
4. Move media and auxiliary capabilities to `provider/media` (media,
   moderation, and structured output are feature-owned; the root only resolves
   `FluxClient` and delegates).
5. Move relationship extraction to `provider/extraction` and provider-neutral
   message/stream primitives to `provider/core` (done).
6. Move model capability/deprecation policy to `catalog/capabilities` and
   request logging to `provider/observability` (done).
7. Move telemetry, recording, callbacks, and usage accounting to
   `provider/observability` (metrics, cost, tracing, recording, and callbacks
   done, including budgets and usage limits).
8. Keep the remaining `provider/` files limited to composition-root operations
   and move any new cross-cutting feature into a subpackage. Provider-neutral
   message primitives belong in `provider/core`; test doubles belong in
   `provider/testkit`.
9. Update all consumers and documentation, then run the full test and boundary
   suite.

For live instance-owned routing and shared peer manifests, see
[Decentralized Flux routing](DECENTRALIZED-FLUX.md).
