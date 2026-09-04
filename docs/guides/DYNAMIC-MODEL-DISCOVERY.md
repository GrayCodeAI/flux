# Dynamic Model Discovery

Status: implemented through the `graycode-router/engine` contract v2.

This guide describes the host-facing path used by Hawk. Hawk is the face; GraycodeRouter
is the engine and source of truth for provider metadata, credentials, live
model discovery, catalog compilation, selection, and provider transport.

## Ownership

| Hawk owns | GraycodeRouter owns |
|---|---|
| credential and model-picker UX | safe credential resolution and persistence |
| conversation/session state | provider registry and deployment metadata |
| tools and permission prompts | provider-scoped live model fetchers |
| presentation of safe DTOs | catalog merge, compilation, and cache health |
| product settings and lifecycle | model ownership, aliases, selection, and routing |
| normalized request construction | provider adapters, streams, usage, and errors |

Hawk calls its integration wrapper, which delegates to `graycode-router/engine`. Hawk UI,
command, and conversation packages do not call GraycodeRouter's lower-level runtime or
client packages.

## End-to-end path

```text
User enters API key or custom-gateway settings in Hawk
  |
  v
Hawk integration --> Engine.ResolveCredential
  |
  v
Engine.SaveCredential
  |
  +--> validate secret/provider
  +--> write secret to the injected credentials.Store
  `--> probe when the provider contract requires it
  |
  v
Engine.ApplyCredentials / Engine.ListLiveModels
  |
  +--> load routing metadata from the injected provider-config path
  +--> resolve credential aliases from the injected store
  +--> construct a provider-scoped discovery environment
  +--> call only that provider's registered live fetcher
  +--> merge offerings and live metadata
  `--> atomically compile the injected catalog path
  |
  v
Engine.ListModels --> Hawk picker --> Engine.SetSelection
  |
  v
Hawk conversation --> Engine.Generate or Engine.Stream --> provider API
  |
  `--> normalized route, content, thinking, tool-call, usage, and done events
```

The provider-scoped environment is important: OpenAI discovery cannot receive
Anthropic, Gemini, or another provider's secrets merely because those secrets
exist in the same store.

## Engine construction and isolation

Create the Engine once at Hawk's composition root and inject all host-owned
state:

```go
e, err := engine.New(engine.Options{
    SecretStore:        store,
    StateDir:           stateDir,
    CatalogPath:        catalogPath,
    ProviderConfigPath: providerConfigPath,
    RemoteCatalogURL:   trustedCatalogURL,
    CustomGateways:     customGateways,
})
```

- `StateDir` derives default catalog and provider paths; explicit paths win.
- An empty `RemoteCatalogURL` selects GraycodeRouter's compiled-in HTTPS seed and does
  not consult a process-environment override.
- Custom gateways are normalized, validated, and snapshotted per Engine.
- `UseRegisteredCustomGateways` exists only for callers that deliberately opt
  into the deprecated process-global compatibility registry.
- The Engine uses the injected secret store for setup, discovery, transport,
  status, removal, compaction, and preflight.

These rules allow multiple Engine instances with different paths, stores, and
custom gateways to coexist safely in one process.

## Catalog sources

GraycodeRouter keeps three complementary sources:

```text
published remote catalog
  |  deployment/protocol/bootstrap metadata
  v
live provider listing ---------> merge/compile ---------> local catalog cache
  provider model IDs, metadata       |                    fast offline reads
                                     v
                             stable Engine.Model DTOs
```

`RefreshCatalog` may refresh the trusted remote source and provider offerings.
`ListLiveModels` performs a direct provider-scoped live listing and does not
mutate the catalog cache. `ListModels` serves stable catalog rows and can ask
for a refresh. `ListPublicModels` is for registered public listing surfaces
that do not require the host to reproduce provider logic.

The stable model DTO intentionally distinguishes:

- `ID`: provider-native/listed model identifier.
- `CanonicalID`: GraycodeRouter's canonical alias target where known.
- `Owner`: catalog model owner.
- `ProviderID`: provider associated with the offering.
- `GatewayID`: selected/listed gateway or deployment identity.
- `Source` and `LiveMetadata`: origin and provider-native metadata.

Hawk should render these fields; it should not infer ownership from model-name
prefixes or parse raw provider responses.

## Provider registry

The declarative provider registry is the shared source for safe setup metadata:

```text
ProviderSpec
  +--> provider/deployment identity and labels
  +--> credential primary name, fallbacks, and aliases
  +--> non-secret base URL and regional routing variables
  +--> live-discovery capability and fetcher lookup
  +--> setup order and chat preference
```

Provider-specific HTTP parsing remains inside registered fetchers/adapters.
Adding a built-in provider should normally require registry data, its adapter
or live fetcher, and tests—not new branches in Hawk UI code.

Custom OpenAI-compatible gateways are invocation-scoped Engine options rather
than registry mutations. Their URLs must be HTTP(S) with a host and must not
contain userinfo, query parameters, or fragments. Credentials remain in the
injected store under the declared credential key.

## Discovery behavior

### Cached listing

```text
Engine.ListModels(provider, refresh=false)
  --> require/load injected catalog cache
  --> select offerings for provider/gateway
  --> canonicalize and enrich capabilities
  --> return stable []engine.Model
```

### Direct live listing

```text
Engine.ListLiveModels(provider)
  --> strict-load injected provider state
  --> resolve only provider credential aliases
  --> add only provider-owned routing/base URL variables
  --> call registered live fetcher
  --> return stable []engine.Model (cache unchanged)
```

### Apply and refresh

```text
Engine.ApplyCredentials(provider)
  --> strict-load provider state
  --> import/sanitize any recognized legacy secret fields
  --> build scoped discovery credentials
  --> refresh provider catalog data
  --> preserve unrelated deployment metadata
  --> atomically persist catalog and routing state
```

Unknown or unsupported providers fail with a typed Engine error. An empty or
failed live response is not silently represented as successful live readiness.

## Selection and conversation handoff

Hawk persists a choice through `Engine.SetSelection(provider, model)`. GraycodeRouter
validates provider/model ownership, preserves custom model IDs, canonicalizes
built-in aliases, and stores routing metadata at the injected provider path.

At generation time Hawk passes provider-neutral messages, tools,
requirements, preferences, limits, and metadata. `Resolve`, `Generate`, and
`Stream` use the same Engine-owned catalog, provider state, credentials, and
custom-gateway snapshot. Hawk neither constructs a provider client nor exports
credentials into the process environment.

## Local and live preflight

Use `Engine.PreflightWithOptions` explicitly:

```text
Local (VerifyLive=false)
  [x] injected state paths and state-file integrity
  [x] real catalog cache health (no bootstrap false positive)
  [x] persisted active provider and model
  [x] exact provider credential/deployment readiness
  [x] local provider transport can be constructed

Live (VerifyLive=true)
  [x] every local check
  [x] selected built-in provider can list models live, or custom gateway probes
  [x] persisted selected model is present in that live result
```

Local preflight is safe for normal startup and offline diagnostics. Live
preflight is an explicit network check and should be labeled accordingly in
Hawk.

## State and secret safety

Secrets are stored only in `credentials.Store`. `provider.json` contains
routing metadata and `model_catalog.json` contains public catalog data.

Engine provider-state mutations:

- serialize by provider-state path, including across Engine instances;
- reject corrupt JSON, unknown fields, and unsupported versions;
- refuse unknown credential-shaped fields;
- import recognized legacy secrets into the injected store before sanitizing;
- preserve unrelated safe deployment metadata;
- write atomically with restrictive permissions; and
- never create a plaintext secret backup.

`MigrateProviderSecretsContext` is the explicit legacy migration. If mapping or
store persistence fails, it aborts and restores the original provider state.
Hawk diagnostics can use `ProviderStateSecurityStatus`, `CatalogHealth`, and
the safe credential/gateway reports without reading either file directly.

## Failure handling

| Failure | Required behavior |
|---|---|
| empty/placeholder secret | reject before persistence |
| credential probe rejects authentication | return an actionable typed error |
| injected store unavailable | fail closed; never fall back to another host path |
| malformed provider state | block mutation and transport construction |
| catalog cache missing/corrupt | report unavailable; do not claim bootstrap readiness |
| live list fails or selected model is absent | fail live preflight |
| custom gateway URL contains embedded data | reject configuration |
| stream caller exits | close/cancel the Engine stream |

Provider-specific friendly error formatting remains GraycodeRouter-owned; Hawk decides
where and how to display it.

## Release order for Hawk

GraycodeRouter is changed and released before Hawk advances its dependency:

```text
standalone GraycodeRouter change
  --> GraycodeRouter tests (two passes)
  --> signed GraycodeRouter commit
  --> publish a resolvable GraycodeRouter module release/commit
  --> update Hawk module dependency when needed
  --> update Hawk's GraycodeRouter module pin to the same published commit
  --> Hawk integration + boundary + clean-clone verification (two passes)
  --> commit Hawk code and gitlink together
```

The parent workspace must use a committed GraycodeRouter checkout, never working-tree-
only code, and Hawk's module pin must resolve to that same published commit.
Both workspace builds and `GOWORK=off` builds must expose the same Engine
contract.

## Related documentation

- `docs/architecture/HOST-ENGINE-BOUNDARY.md` — ownership and compatibility
  policy.
- `CREDENTIAL-SETUP-FLOW.md` — product setup flow.
- Hawk's dynamic-model and architecture docs — host integration and UI behavior.
