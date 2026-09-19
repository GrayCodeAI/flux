<div align="center">

# <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bird.svg" width="16" height="16" alt="bird" /> flux Architecture

**Universal LLM Provider Runtime**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Port](https://img.shields.io/badge/Port-8080-orange)](https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml)
[![Protocol](https://img.shields.io/badge/Protocol-REST-blue)](https://swagger.io/specification/)

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
│   ├── resilience/          Retry, fallback, rate limits and health
│   └── observability/       Usage, metrics, tracing and recording
├── catalog/                 Model catalog and capabilities
├── config/ + credentials/   Config + keyring/env credential resolution
├── router/                  Deployment policy and instance-local circuit breakers
│   └── controlplane/        Versioned, signed peer manifests and replicas
├── runtime/                 Host-facing construction
├── conversation/ + storage/ Conversation graph (branching DAG) + SQLite store
└── internal/api|cache|health|observability  HTTP server, cache, health, OTel
```

The current distributed-routing foundation and its limits are described in
[Decentralized Flux routing](architecture/DECENTRALIZED-FLUX.md).

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/globe.svg" width="16" height="16" alt="globe" /> API

| | |
|---|---|
| **Contract** | [`api/openapi.yaml`](../api/openapi.yaml) |
| **Port** | `:8080` (default). Override: `flux serve <port>` |
| **Auth** | Bearer token or `X-API-Key` header. Set via `FLUX_API_KEY` |

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

</details>

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/search.svg" width="16" height="16" alt="search" /> Provider Detection

Auto-detects active provider from env vars in priority order:

| Priority | Env Var | Provider |
|:--------:|---------|----------|
| 1 | `ANTHROPIC_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> Anthropic Claude |
| 2 | `OPENAI_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> OpenAI |
| 3 | `GEMINI_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> Google Gemini |
| 4 | `OPENROUTER_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/shuffle.svg" width="16" height="16" alt="shuffle" /> OpenRouter |
| 5 | `CANOPYWAVE_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> CanopyWave |
| 6 | `XAI_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/zap.svg" width="16" height="16" alt="zap" /> Grok (xAI) |
| 7 | `ZAI_API_KEY` | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bot.svg" width="16" height="16" alt="bot" /> ZAI |
| 8 | — | <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/server.svg" width="16" height="16" alt="server" /> Ollama (localhost socket) |

*Top 8 shown; full 28 in `catalog/registry/providers.go` — see [`CREDENTIAL-SETUP-FLOW.md`](./guides/CREDENTIAL-SETUP-FLOW.md) and `config` ChatPreference order.*

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> Streaming

All responses are streamed via **SSE**. Blocking responses wrap the stream internally.

```go
sr, err := provider.StreamChat(ctx, messages, opts)
defer sr.Close()
for event := range sr.Events() { ... }
```

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/refresh-cw.svg" width="16" height="16" alt="refresh-cw" /> Retry & Rate Limiting

| Feature | Behavior |
|---------|----------|
| **Retries** | HTTP 429, 500, 502, 503, 529 |
| **Backoff** | Exponential + jitter |
| **Retry-After** | Respected on 429 responses |
| **Rate Limiting** | Per-provider token-bucket |

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/database.svg" width="16" height="16" alt="database" /> Caching

| Layer | Strategy | Key |
|-------|----------|-----|
| **Exact** | Hash match | provider + model + message hash |
| **Semantic** | Cosine similarity | Prompt embeddings (optional, configurable TTL) |

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/git-branch.svg" width="16" height="16" alt="git-branch" /> Conversation Graph

Conversations are stored as a **DAG** in SQLite. Each prompt creates a `Node`; branching is first-class. Nodes are addressable by ID or named alias.
