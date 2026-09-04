<p align="center">
  <img src="assets/logo.svg" alt="graycode-router" width="480"/>
</p>

<h1 align="center">Universal LLM Provider Runtime</h1>

<p align="center">
  One interface for every model. Authentication, routing, streaming, retries, caching — handled.
</p>

<p align="center">
  <a href="https://golang.org/"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <a href="https://github.com/GrayCodeAI/graycode-router/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/GrayCodeAI/graycode-router/ci.yml?style=flat-square&label=tests" alt="CI"></a>
  <a href="https://github.com/GrayCodeAI/graycode-router/releases"><img src="https://img.shields.io/github/v/release/GrayCodeAI/graycode-router?style=flat-square&label=release&color=green" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/GrayCodeAI/graycode-router"><img src="https://img.shields.io/badge/godoc-reference-00ADD8?style=flat-square&logo=go" alt="GoDoc"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="#documentation">Docs</a> ·
  <a href="#examples">Examples</a> ·
  <a href="#supported-providers">Providers</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#contributing">Contributing</a>
</p>

---

## What is graycode-router

graycode-router is the LLM provider runtime that powers the [hawk](https://github.com/GrayCodeAI/hawk) coding agent. It handles everything between your application and LLM APIs — authentication, model resolution, streaming, retries, rate limiting, and caching.

When your app calls a model, graycode-router figures out which provider to use, how to talk to it, and how to stream the response back. Switch from Anthropic to Ollama? graycode-router handles the translation. API returns 529? graycode-router retries with backoff. Response hits `max_tokens`? graycode-router continues automatically.

**Your app never talks to an LLM API directly. graycode-router does.**

Hawk is the product face: it owns UX, agent orchestration, tools, permissions,
sessions, and product semantics. GraycodeRouter is the provider engine: it owns
credentials, catalog and route resolution, provider transports, normalized
streams, retry/fallback, usage, and provider telemetry. Hawk integrates through
the stable [`engine`](engine/) facade rather than assembling GraycodeRouter's internal
provider packages.

## Ecosystem Boundaries

graycode-router is a Hawk support engine. Keep the dependency edge one-way:

- host-facing DTOs and the `Provider` port live in `eagle/llm`; `engine/` re-exports them as aliases (`*Engine` implements `llm.Provider`)
- internal provider/transport types stay graycode-router-scoped (not shared contracts)
- do not import `hawk/internal/*`
- do not import removed legacy path `hawk/shared/types`
- do not import other engines (`harrier`, `shrike`, `swift`, `kestrel`, `merlin`) — engines are peers, not dependencies

## Quick Start

```bash
go get github.com/GrayCodeAI/graycode-router
```

Requires Go 1.26+. Minimal dependencies (UUID, OpenTelemetry, SQLite, keyring).

```go
import "github.com/GrayCodeAI/graycode-router/client"

// Create a client — provider auto-detected from environment
c := client.NewGraycodeRouterClient(&client.GraycodeRouterConfig{
    Provider: client.DetectProvider(),
})

// Stream a response
sr, err := c.StreamChat(ctx, messages, client.ChatOptions{
    Model: "claude-sonnet-4-6",
})
defer sr.Close()

for evt := range sr.Events {
    switch evt.Type {
    case "content":   // stream text
    case "tool_call": // execute tool
    case "done":      // response complete
    }
}
```

## Features

### Provider Routing

Automatically detects and routes to the right provider based on environment variables, config files, or explicit selection.

### Model Resolution

Maps abstract tiers (opus/sonnet/haiku) to concrete model IDs per provider. Ships with an embedded catalog of pricing, context windows, and capabilities.

### Streaming

Parses SSE for Anthropic and OpenAI formats — text, tool calls, and thinking blocks.

### Reliability

- Retries on 429/500/529 with exponential backoff and `Retry-After` support
- Auto-continuation when `stop_reason == max_tokens`
- Provider fallback chains for high availability

### Rate Limiting

Token bucket rate limiter per provider — prevents hitting API limits before they happen.

### Caching

- Response caching with configurable TTL
- Semantic similarity caching for repeated prompts
- Anthropic prompt caching breakpoints on system prompt and conversation prefix

### Cost Tracking

Built-in cost estimation per call, with per-provider pricing from the embedded model catalog.

### Reasoning Controls

Passes `reasoning_effort` and Anthropic extended-thinking `thinking_budget_tokens` through to capable models — omitted when unset.

### Keyless CI Auth

GitHub OIDC keyless authentication for cloud deployments — mints a short-lived token in GitHub Actions and exchanges it for AWS Bedrock (STS `AssumeRoleWithWebIdentity`) or GCP Vertex (Workload Identity Federation) credentials, no stored secrets.

### OpenAI-Compatible Proxy

Serves `POST /v1/chat/completions` so existing OpenAI SDK clients can talk to graycode-router unchanged.

### Load-Balancing Strategies

Named routing strategies beyond weighted random: `simple-shuffle`, `least-busy`, `latency-based`, `cost-based`, and `usage-based`.

### Pluggable Cache & Audit Sinks

Distributed `CacheBackend` interface (in-memory default, RESP/Redis-capable) and an `AuditSink` interface (no-op default, JSONL file sink) for privacy-preserving call metadata.

### Model Role Slots

Named `primary` / `weak` / `editor` model slots with fallback to primary, plus an LLM summarizing condenser that shrinks long conversation histories using the weak model.

### Rerank & Readiness

`POST /rerank` endpoint (provider-backed with lexical fallback) and a `GET /ready` readiness probe alongside the existing health check.

### gRPC Skeleton

Dependency-free gRPC API skeleton behind the `grpc` build tag — wired when generated stubs are available.

## Documentation

Detailed documentation is available in the [docs/](docs/) directory:

- **[Architecture](docs/ARCHITECTURE.md)** — System design, data flow, and reliability features
- **[Provider Setup Guide](docs/guides/CREDENTIAL-SETUP-FLOW.md)** — Credential configuration and provider setup
- **[Dynamic Model Discovery](docs/guides/DYNAMIC-MODEL-DISCOVERY.md)** — Live model discovery architecture

## Examples

Runnable examples are in the [examples/](examples/) directory:

- **[Basic Chat](examples/basic/)** — Simple synchronous chat
- **[Streaming](examples/streaming/)** — SSE streaming with event handling
- **[Multi-Provider](examples/multi-provider/)** — Fallback chains across providers

Run any example with:

```bash
ANTHROPIC_API_KEY=sk-... go run ./examples/basic/
```

## Supported Providers

22 provider gateways in `catalog/registry/providers.go` (hawk `/config` uses the same list), listed in registry `SortOrder`:

| Provider | ID | Env variable |
|---|---|---|
| **Anthropic** | `anthropic` | `ANTHROPIC_API_KEY` |
| **OpenAI** | `openai` | `OPENAI_API_KEY` |
| **Google Gemini** | `gemini` | `GEMINI_API_KEY` |
| **DeepSeek** | `deepseek` | `DEEPSEEK_API_KEY` |
| **xAI (Grok)** | `grok` | `XAI_API_KEY` |
| **Kimi (Moonshot)** | `kimi` | `MOONSHOT_API_KEY` |
| **Z.AI — Coding Plan** | `zai_coding` | `ZAI_CODING_API_KEY` |
| **Z.AI — Pay-as-you-go** | `zai_payg` | `ZAI_API_KEY` |
| **Xiaomi (MiMo) Token Plan** | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` (+ region `cn` / `sgp` / `ams`) |
| **Xiaomi (MiMo) Pay-as-you-go** | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` |
| **MiniMax — Token Plan** | `minimax_token_plan` | `MINIMAX_TOKEN_PLAN_API_KEY` |
| **MiniMax — Pay-as-you-go** | `minimax_payg` | `MINIMAX_PAYG_API_KEY` |
| **Azure OpenAI** | `azure` | `AZURE_OPENAI_API_KEY` (+ `AZURE_OPENAI_ENDPOINT`) |
| **Amazon Bedrock** | `bedrock` | `AWS_SECRET_ACCESS_KEY` (+ `AWS_ACCESS_KEY_ID`, `AWS_SESSION_TOKEN`) |
| **Vertex AI** | `vertex` | `VERTEX_ACCESS_TOKEN` (or `GOOGLE_OAUTH_ACCESS_TOKEN`) |
| **OpenRouter** | `openrouter` | `OPENROUTER_API_KEY` |
| **CanopyWave** | `canopywave` | `CANOPYWAVE_API_KEY` |
| **Poolside** | `poolside` | `POOLSIDE_API_KEY` |
| **Groq** | `groq` | `GROQ_API_KEY` |
| **ClinePass** | `clinepass` | `CLINE_API_KEY` |
| **OpenCode Go** | `opencodego` | `OPENCODEGO_API_KEY` |
| **Ollama** | `ollama` | `OLLAMA_BASE_URL` (local; no API key) |

Runtime auto-detection uses a separate priority order for chat when no deployment is pinned; see `config` profiles.

## Usage

### Basic Chat

```go
resp, err := c.Chat(ctx, messages, client.ChatOptions{
    Model: "gpt-4o",
})
```

### Streaming with Continuation

```go
// Auto-continues when max_tokens is hit
resp, err := client.ChatWithContinuation(ctx, provider, messages,
    client.ChatOptions{Model: model},
    client.DefaultContinuationConfig(),
)
```

### Mock Provider for Testing

```go
mock := client.NewMockProvider(client.MockModeFixed)
mock.Response = "Here is the code you asked for..."

resp, _ := mock.Chat(ctx, messages, opts)
// No real API calls — perfect for tests
```

### Model Catalog

```go
cat := catalog.DefaultModelCatalog()

// Get the best model for a tier
model := catalog.GetPreferredProviderModel("anthropic", catalog.TierSonnet, &cat)
// → "claude-sonnet-4-6"

// Check deprecation warnings
warn := catalog.GetModelDeprecationWarning("claude-3-7-sonnet", "anthropic")
```

### Provider Configuration

```go
cfg := config.LoadProviderConfig("")             // load from disk
config.ApplyProviderConfigToEnv(cfg, false, nil) // apply to environment
config.SaveProviderConfig(cfg, "")               // save changes
```

## Architecture

```
graycode-router/
├── engine/                 # Stable host-facing facade and provider-neutral DTOs
├── client/                 # Backwards-compatible public client facade
│   ├── core/               # Provider-neutral wire, stream, retry, and transport primitives
│   ├── adapters/           # Provider protocol adapters and construction registry
│   └── embeddings/         # Embedding clients, cache, and defaults
├── config/                 # Provider configuration & routing
│   └── credential/         # Credential file management
├── catalog/                # Model catalog & tier system
│   ├── discover/           # Model discovery
│   ├── legacy/             # Legacy model support
│   ├── live/               # Live model data
│   └── registry/           # Model registry
├── codeagent/              # Code agent retry & fallback strategies
├── conversation/           # Conversation engine with branching
├── credentials/            # Credential management
├── docs/                   # Documentation & guides
├── examples/               # Runnable code examples
├── router/                 # Provider routing strategies
├── operationsgraph/        # Privacy-safe route and generation telemetry projection
├── runtime/                # Runtime manifest & routing policies
├── storage/                # SQLite conversation DAG store
├── types/                  # Branded types & API errors
├── errors/                 # Error message constants
├── constants/              # API limits
├── utils/                  # Error utilities
├── internal/
│   ├── api/                # HTTP API handlers
│   ├── cache/              # Response cache warmer
│   ├── health/             # Provider health checker
│   ├── observability/      # OpenTelemetry spans & metrics
│   ├── sdk/                # Go, Python, TypeScript client SDKs
│   └── version/            # Version information
└── assets/                 # Logo and branding
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed system design and data flows.

`operationsgraph.Build` projects resolved routes and normalized usage into
`graycode-router.graph/v1` operations nodes. Provider, model, request ID, and generated
content are represented only by SHA-256 digests; token counts, finish reason,
tool-call count, and deployment-routing state remain queryable.

## Ecosystem

graycode-router is part of the hawk-eco:

| Component | Repository | Purpose |
|---|---|---|
| **hawk** | [GrayCodeAI/hawk](https://github.com/GrayCodeAI/hawk) | AI coding agent |
| **graycode-router** | This repo | LLM provider runtime |
| **shrike** | [GrayCodeAI/shrike](https://github.com/GrayCodeAI/shrike) | Tokenizer & compression |
| **harrier** | [GrayCodeAI/harrier](https://github.com/GrayCodeAI/harrier) | Graph-based memory |
| **swift** | [GrayCodeAI/swift](https://github.com/GrayCodeAI/swift) | Session capture |

## Development

### Prerequisites

- Go 1.26+

### Build & Test

```bash
go build ./...               # Verify the library compiles
go test -race ./...           # Run all tests with race detector
make ci                       # Run full CI suite (lint, test, security)
make cover                    # Generate coverage report
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, commit conventions, and the PR process.

Quick start:

1. Fork and create a branch: `git checkout -b feat/short-description`
2. Make changes in small, focused commits
3. Run `make ci` locally
4. Open a pull request

Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages — release-please uses them for versioning.

## License

MIT — see [LICENSE](LICENSE) for details.

© 2026 GrayCode AI
