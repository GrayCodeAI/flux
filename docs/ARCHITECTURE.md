<div align="center">

# <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bird.svg" width="16" height="16" alt="bird" /> flux Architecture

**Universal LLM Provider Runtime**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Library](https://img.shields.io/badge/type-Go%20library-blue)](https://pkg.go.dev/github.com/GrayCodeAI/flux)

</div>

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/target.svg" width="16" height="16" alt="target" /> Overview

flux is the LLM provider runtime for the rho ecosystem. It sits between the application and LLM APIs, handling **authentication**, **model resolution**, **streaming**, **retries**, **rate limiting**, and **caching**.

> <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/lightbulb.svg" width="16" height="16" alt="lightbulb" /> No rho ecosystem component talks to an LLM API directly — all communication goes through flux.

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/blocks.svg" width="16" height="16" alt="blocks" /> Components

```
flux/
├── engine/                  Stable host-facing facade (rho imports only this + llm/graph/tools)
├── llm/                     Host-facing DTOs + Provider port (engine re-exports)
├── graph/                   Portable execution-graph vocabulary
├── tools/                   Tool-call/result contracts
├── provider/                Client composition root
│   ├── core/                Provider-neutral contracts, stream and transport
│   ├── adapters/            Provider wire-protocol adapters
│   ├── batch/ cache/        Batch execution and response caches
│   ├── embeddings/ media/   Embeddings and multimodal features
│   ├── resilience/          Rate limits, continuation, guardrails, health, error policy
│   └── observability/       Usage, metrics, tracing and recording
├── catalog/                 Model catalog and capabilities
├── config/ + credentials/   Config + keyring/env credential resolution
├── router/                  Deployment policy and instance-local circuit breakers
│   └── controlplane/        Versioned, signed peer manifests and replicas
├── runtime/                 Engine-internal provider/model/credential resolution
├── conversation/ + storage/ Conversation graph (branching DAG) + SQLite store
└── internal/api|cache|grpc|health|observability  HTTP server, cache, gRPC, health, OTel
```

The current distributed-routing foundation and its limits are described in
[Decentralized Flux routing](architecture/DECENTRALIZED-FLUX.md).

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/globe.svg" width="16" height="16" alt="globe" /> API

flux is a Go library: it ships no binary, no `cmd/`, and no `flux serve`
command. Hosts such as rho call the [`engine`](../engine/) facade in-process.

`internal/api` contains an HTTP server (`api.NewServer(api.Config{...})`, then
`ListenAndServe(addr)`) whose contract is [`api/openapi.yaml`](../api/openapi.yaml).
The package is internal, so only code inside the flux module can construct it,
and nothing in flux does today. The OpenAPI `servers` entry
(`http://localhost:8080`) is an example address, not a default.

| | |
|---|---|
| **Contract** | [`api/openapi.yaml`](../api/openapi.yaml) |
| **Address** | Whatever the embedding code passes to `ListenAndServe(addr)`; there is no default port |
| **Auth** | `Authorization: Bearer <key>` or `X-API-Key: <key>`, compared with `api.Config.APIKey` on every route except `/health` and `/ready`. With an empty key the server refuses to bind a non-loopback address. There is no environment variable for the key. |

<details>
<summary><b><img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> Endpoint Summary</b></summary>

| Method | Path | Tag | Description |
|--------|------|-----|-------------|
| `GET` | `/health` | health | Health check |
| `POST` | `/prompt` | prompt | Execute a prompt at root |
| `POST` | `/nodes/{id}/prompt` | prompt | Continue from a node |
| `GET` | `/nodes` | nodes | List root nodes |
| `GET` | `/nodes/{id}` | nodes | Get a specific node |
| `DELETE` | `/nodes/{id}` | nodes | Delete node + descendants |
| `GET` | `/nodes/{id}/tree` | nodes | Get subtree |
| `PUT` | `/nodes/{id}/aliases/{alias}` | aliases | Create alias |
| `DELETE` | `/aliases/{alias}` | aliases | Delete alias |
| `GET` | `/api/usage` | analytics | Token usage analytics |
| `GET` | `/api/costs` | analytics | Cost breakdown |
| `GET` | `/api/health/providers` | providers | Provider health |
| `POST` | `/v1/chat/completions` | chat | OpenAI-compatible proxy |
| `POST` | `/rerank` | rerank | Provider rerank + lexical fallback |
| `GET` | `/ready` | health | Readiness probe (vs `/health` liveness) |

The last three endpoints are served by `internal/api` but are not yet described
in `api/openapi.yaml`.

</details>

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/search.svg" width="16" height="16" alt="search" /> Provider Detection

`provider.DetectProvider()` walks `config.APIProviderDetectionOrder`
(`config/profiles.go`) and returns the first provider whose credentials are
present in the credential store (by default the OS secret store; hosts inject
their own through `engine.Options.SecretStore`), defaulting to `anthropic` when
none is found. Multi-field providers need every field: Azure
needs `AZURE_OPENAI_API_KEY` and `AZURE_OPENAI_ENDPOINT`, Bedrock needs
`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, Vertex needs
`VERTEX_PROJECT_ID` and `VERTEX_ACCESS_TOKEN`, and Ollama is detected from
`OLLAMA_BASE_URL`. The detection order is separate from the registry
`SortOrder` used for display and from `ChatPreference`; every provider in
[`catalog/registry/providers.go`](../catalog/registry/providers.go) appears in
it. Hosts do not call `DetectProvider`; they select through `engine`.

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> Streaming

Providers implement both a blocking `Chat` and an SSE-based `StreamChat`;
streamed provider events are normalized into `FluxStreamEvent`s. Hosts use
`engine.Stream` (pull-based, must be closed) or `engine.Generate`. Inside
flux, a `*provider.FluxClient` exposes the same pair:

```go
sr, err := client.StreamChat(ctx, messages, opts)
if err != nil { ... }
defer sr.Close()
for event := range sr.Events { ... }
```

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/refresh-cw.svg" width="16" height="16" alt="refresh-cw" /> Retry & Rate Limiting

| Feature | Behavior |
|---------|----------|
| **Retries** | HTTP 429, 500, 502, 503, 529 (`core.DefaultRetryConfig`) |
| **Backoff** | Exponential + jitter |
| **Retry-After** | Honored (seconds or HTTP date, capped at `MaxDelay`) on any retried response |
| **Rate Limiting** | Per-provider token bucket; optional adaptive limiter driven by rate-limit headers |

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/database.svg" width="16" height="16" alt="database" /> Caching

| Layer | Where | Strategy | Key |
|-------|-------|----------|-----|
| **Exact** | `provider/cache` (`CachedProvider`) | SHA-256 match, LRU + TTL | model, system prompt, temperature, messages |
| **Semantic** | `provider/embeddings` (`EmbeddingCachedProvider`) | Cosine similarity ≥ 0.95 by default, LRU + TTL | prompt embedding from a configured embedding model |

Both layers are opt-in, skip requests above the temperature threshold, and
cache only blocking `Chat` responses; `StreamChat` passes through uncached.

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/git-branch.svg" width="16" height="16" alt="git-branch" /> Conversation Graph

Conversations are stored as a **DAG** in SQLite. Each prompt creates a `Node`; branching is first-class. Nodes are addressable by ID or named alias.
