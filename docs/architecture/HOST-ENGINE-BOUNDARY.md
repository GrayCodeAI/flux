# Host–Eyrie Engine Boundary

Status: accepted. The host-facing compatibility contract is
`engine.ContractVersion == "2"`.

## Decision

Hawk is the product face. It owns the terminal UI, agent loop, tool execution,
permissions, conversation history, checkpoints, and product semantics. Eyrie is
the engine. It owns credentials, provider/deployment metadata, model discovery,
catalog compilation, selection, transport construction, provider request and
stream normalization, resilience, normalized usage, and provider telemetry.

```text
User
  |
  v
Hawk (face: UX, session, tools, permissions)
  |
  | stable DTOs and methods
  v
github.com/GrayCodeAI/eyrie/engine  [contract v2]
  |
  +--> injected secret store
  +--> injected state paths
  +--> invocation-scoped custom gateways
  +--> catalog, routing, and provider adapters
  |
  v
Provider model APIs
```

The dependency is one-way: Eyrie must not import Hawk. Hawk's integration layer
may import `eyrie/engine`; Hawk command, conversation, and UI packages must not
assemble Eyrie's `catalog`, `client`, `config`, `credentials`, `router`,
`runtime`, or `setup` packages.

## Composition root

Hawk constructs one `engine.Engine` from host-owned dependencies:

```go
e, err := engine.New(engine.Options{
    SecretStore:        store,
    StateDir:           stateDir,
    CatalogPath:        catalogPath,        // optional StateDir override
    ProviderConfigPath: providerConfigPath, // optional StateDir override
    RemoteCatalogURL:   trustedCatalogURL,  // optional; compiled HTTPS default
    CustomGateways:     gateways,           // snapshotted for this Engine
})
```

`StateDir` derives `model_catalog.json` and `provider.json` when explicit paths
are absent. Explicit paths win. The store, paths, remote catalog URL, and custom
gateways belong to the Engine instance; production behavior does not depend on
ambient Hawk paths or a process-global custom-gateway registry. The global
registry remains an opt-in compatibility path through
`UseRegisteredCustomGateways`.

## Stable contract

The facade exposes provider-neutral request, response, stream, catalog,
credential, gateway, selection, health, and preflight DTOs. The principal
runtime operations are:

```text
New / StatePaths
Resolve / Generate / Stream
Catalog / RefreshCatalog / ListModels / ListLiveModels / ListPublicModels
ResolveCredential / SaveCredential / RemoveCredential / CredentialStatus
GatewayDefinitions / Gateways / SetGatewayRegion
ActiveSelection / EffectiveSelection / SetSelection / ClearSelection
CatalogHealth / ProviderStateSecurityStatus / PreflightWithOptions
MigrateProviderSecretsContext
```

Provider-specific wire types, authentication headers, retry behavior, and raw
stream events do not cross this boundary. `Model` keeps distinct `Owner`,
`ProviderID`, `GatewayID`, `CanonicalID`, `Source`, and `LiveMetadata` fields so
Hawk does not reconstruct catalog meaning.

## Credential-to-conversation flow

```text
paste API key / configure gateway
  |
  v
Engine.ResolveCredential
  |
  v
Engine.SaveCredential --> injected credentials.Store
  |                         (secret material only)
  v
Engine.ApplyCredentials / ListLiveModels
  |
  +--> load provider routing from injected provider.json
  +--> resolve only the selected provider's credential aliases
  +--> pass a provider-scoped environment to its live fetcher
  +--> merge/compile catalog and atomically update model_catalog.json
  v
Engine.ListModels --> Hawk picker --> Engine.SetSelection
  |
  v
Hawk builds provider-neutral GenerateRequest from its conversation
  |
  +--> Engine.Generate
  `--> Engine.Stream --> normalized route/content/thinking/tool/usage events
```

Secrets never belong in catalog rows, requests, logs, diagnostics, telemetry,
or host tool environments. Credential status and gateway DTOs expose only safe
identifiers, booleans, and masked presentation values. Live discovery receives
only the active provider's credential and routing variables, not the complete
secret-store contents.

## State and migration contract

`provider.json` contains routing metadata, never credential material. Engine
mutations serialize per provider-state path, reject corrupt, unknown-version,
or unknown-field state, sanitize before persistence, and replace files
atomically with restrictive permissions. They fail closed instead of silently
overwriting malformed state.

Legacy provider files may contain credential-shaped fields. The explicit
`MigrateProviderSecretsContext` flow maps every recognized secret to the
injected store before writing a sanitized provider file. An unmapped secret or
store failure aborts the migration and restores the original state; no
plaintext backup is created. Hawk should run security status/migration before
normal provider-state writes.

## Selection and generation

Hosts express capabilities (`streaming`, `tools`, `vision`, structured JSON,
reasoning, and context size) plus an intent or explicit provider/model. An
explicit model is a hard constraint unless the host enables fallback. Eyrie
owns canonicalization, gateway ownership, deployment routing, and capability
matching.

Eyrie generation is stateless from Hawk's point of view:

```text
Hawk owns                         Eyrie owns
----------                        -----------
conversation history              route decision
tool permission and execution     provider transport
checkpoints and replay            retry/fallback policy
product memory                    stream normalization
session lifecycle                 normalized usage/telemetry
```

`Stream` is pull-based, cancellable, and must be closed. It emits the selected
route before provider events and normalizes content, thinking, tool calls,
usage, retry/continuation, TTFT, and completion. Unknown future event types are
additive and must be ignored safely. Eyrie emits tool requests; Hawk authorizes
and executes tools, appends results to its history, and begins the next model
turn.

## Readiness

Preflight has two explicit modes:

- Local preflight validates injected paths, catalog health, provider-state
  integrity, exact selected provider/model, credential presence, deployment
  configuration, and local transport construction without requiring network.
- Live preflight additionally probes the selected custom gateway or performs a
  provider-scoped live model listing and verifies that the selected model is
  actually available.

These modes must not be conflated: local readiness is suitable for startup and
diagnostics; live readiness is an explicit network operation.

## Release and sibling-repository order

The boundary is delivered Eyrie-first:

```text
1. Change and verify standalone Eyrie
2. Commit Eyrie and publish a resolvable release/commit
3. Update Hawk's Eyrie module version when required
4. Update Hawk's Eyrie module pin to that exact published commit
5. Verify Hawk integration, boundary checks, and clean-clone/module builds
6. Commit the Hawk module-pin update in Hawk's repository
```

Hawk must never depend on an uncommitted Eyrie worktree. The parent workspace
uses a sibling checkout for source identity during local development; a
resolvable published module version is also required for workflows that build
Hawk with `GOWORK=off`.

## Compatibility policy

Lower-level Eyrie packages remain public for non-Hawk consumers and staged
migration, but they are not part of Hawk's product boundary. Additive fields
and stream events are allowed within contract v2. Removing or changing stable
DTO semantics requires a contract-version and semantic-version boundary.
