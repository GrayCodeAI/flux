<div align="center">

# <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bird.svg" width="16" height="16" alt="bird" /> graycode-router Architecture

**Universal LLM Provider Runtime**

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Port](https://img.shields.io/badge/Port-8080-orange)](https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml)
[![Protocol](https://img.shields.io/badge/Protocol-REST-blue)](https://swagger.io/specification/)

</div>

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/target.svg" width="16" height="16" alt="target" /> Overview

graycode-router is the LLM provider runtime for the hawk ecosystem. It sits between the application and LLM APIs, handling **authentication**, **model resolution**, **streaming**, **retries**, **rate limiting**, and **caching**.

> <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/lightbulb.svg" width="16" height="16" alt="lightbulb" /> No hawk ecosystem component talks to an LLM API directly — all communication goes through graycode-router.

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/blocks.svg" width="16" height="16" alt="blocks" /> Components

```
graycode-router/
├── api/openapi.yaml         <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/file-text.svg" width="16" height="16" alt="file-text" /> REST API contract (OpenAPI 3.1) — embedded HTTP server surface
├── client/
│   ├── client.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/plug.svg" width="16" height="16" alt="plug" /> Provider interface + GraycodeRouterClient factory
│   ├── anthropic.go         <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> Anthropic Claude provider
│   ├── openai.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> OpenAI / OpenAI-compat provider
│   ├── gemini.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> Google Gemini provider
│   ├── bedrock.go           <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> AWS Bedrock provider
│   ├── vertex.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/circle.svg" width="16" height="16" alt="circle" /> Google Vertex AI provider
│   ├── azure.go             <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/diamond.svg" width="16" height="16" alt="diamond" /> Azure OpenAI provider
│   ├── provider_registry.go <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/search.svg" width="16" height="16" alt="search" /> Auto-detection + registration
│   ├── compat.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/wrench.svg" width="16" height="16" alt="wrench" /> Compatibility configs (Grok, OpenRouter, etc.)
│   ├── stream.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> SSE stream parsing
│   ├── retry.go             <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/refresh-cw.svg" width="16" height="16" alt="refresh-cw" /> Exponential backoff + Retry-After
│   ├── ratelimit.go         <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bucket.svg" width="16" height="16" alt="bucket" /> Token-bucket rate limiting per provider
│   ├── cache.go             <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/database.svg" width="16" height="16" alt="database" /> Response caching
│   ├── semantic_cache.go    <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/brain.svg" width="16" height="16" alt="brain" /> Similarity-based cache lookup
│   ├── fallback.go          <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/shuffle.svg" width="16" height="16" alt="shuffle" /> Provider fallback chains
│   └── errors.go            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/x-circle.svg" width="16" height="16" alt="x-circle" /> GraycodeRouterError type
├── catalog/                 <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/list.svg" width="16" height="16" alt="list" /> Model catalog — pricing, context windows, tiers
├── config/                  <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/settings.svg" width="16" height="16" alt="settings" /> Configuration and credential resolution
├── conversation/            <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/git-branch.svg" width="16" height="16" alt="git-branch" /> Conversation graph engine (branching DAG)
├── credentials/             <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/key.svg" width="16" height="16" alt="key" /> API key management and env detection
├── router/                  <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/traffic-cone.svg" width="16" height="16" alt="traffic-cone" /> Weighted provider routing
├── storage/                 <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/archive.svg" width="16" height="16" alt="archive" /> Conversation store (SQLite DAG)
└── internal/
    ├── api/                 <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/globe.svg" width="16" height="16" alt="globe" /> HTTP server, route handlers, auth middleware
    ├── cache/               <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/database.svg" width="16" height="16" alt="database" /> Cache infrastructure
    ├── health/              <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/heart.svg" width="16" height="16" alt="heart" /> Provider health checker
    ├── observability/       <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/bar-chart.svg" width="16" height="16" alt="bar-chart" /> OpenTelemetry spans and metrics
    ├── shrink/              <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/package.svg" width="16" height="16" alt="package" /> Response compression
    └── version/             <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/tag.svg" width="16" height="16" alt="tag" /> Version constants
```

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/globe.svg" width="16" height="16" alt="globe" /> API

| | |
|---|---|
| **Contract** | [`api/openapi.yaml`](../api/openapi.yaml) |
| **Port** | `:8080` (default). Override: `graycode-router serve <port>` |
| **Auth** | Bearer token or `X-API-Key` header. Set via `GRAYCODE_ROUTER_API_KEY` |

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

*Top 8 by priority; 7 more (`azure`, `bedrock`, `vertex`, `deepseek`, `opencodego`, `kimi`, `xiaomi_mimo_payg`, `xiaomi_mimo_token_plan`, `minimax_token_plan`, `minimax_payg`) — see [`CREDENTIAL-SETUP-FLOW.md`](./guides/CREDENTIAL-SETUP-FLOW.md).*

---

## <img src="https://cdn.jsdelivr.net/gh/lucide-icons/lucide@latest/icons/radio.svg" width="16" height="16" alt="radio" /> Streaming

All responses are streamed via **SSE**. Blocking responses wrap the stream internally.

```go
sr, err := client.StreamChat(ctx, messages, opts)
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
