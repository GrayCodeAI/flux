# Credential setup: hawk (face) + graycode-router (brain)

**Principle:** Hawk renders UI only. GraycodeRouter owns providers, keys, validation, catalog, models.

See also: [DYNAMIC-MODEL-DISCOVERY.md](./DYNAMIC-MODEL-DISCOVERY.md)

## Supported providers (15 — from `catalog/registry`)

| Provider | ID | Credential | Picker models |
|----------|-----|------------|---------------|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | live `/v1/models` only |
| OpenAI | `openai` | `OPENAI_API_KEY` | live `/v1/models` only |
| Google Gemini | `gemini` | `GEMINI_API_KEY` | live models API only |
| DeepSeek | `deepseek` | `DEEPSEEK_API_KEY` | live `/models` only |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | live `/models` only |
| xAI (Grok) | `grok` | `XAI_API_KEY` | live `/v1/models` only |
| Z.AI | `z-ai` | `ZAI_API_KEY` | live `/models` only |
| CanopyWave | `canopywave` | `CANOPYWAVE_API_KEY` | live `/models` only |
| OpenCode Go | `opencodego` | `OPENCODEGO_API_KEY` | live `/models` only |
| Kimi (Moonshot) | `kimi` | `MOONSHOT_API_KEY` | live `/models` only |
| Xiaomi (MiMo) Pay-as-you-go | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` | `https://api.xiaomimimo.com/v1` (`sk-` keys) |
| Xiaomi (MiMo) Token Plan | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` | region: `cn` / `sgp` / `ams` (`tp-` keys) |
| MiniMax (minimax) Token Plan | `minimax_token_plan` | `MINIMAX_TOKEN_PLAN_API_KEY` | `https://api.minimax.io/v1` |
| MiniMax (minimax) Pay-as-you-go | `minimax_payg` | `MINIMAX_PAYG_API_KEY` | `https://api.minimax.io/v1` |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` | live `/api/tags` only |

Single source: `graycode-router/catalog/registry/providers.go`

### Xiaomi MiMo (two gateways, one key each)

MiMo exposes **OpenAI-compatible** and **Anthropic-compatible** APIs on the same host. You store **one API key per product** (not separate OpenAI vs Anthropic keys). Chat uses OpenAI first; Anthropic compat is a fallback on retryable errors (401/403/5xx/timeout).

| Product | Gateway ID | Env var | Key shape (hint only) | OpenAI base | Anthropic base |
|---------|------------|---------|----------------------|-------------|----------------|
| Pay-as-you-go | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` | `sk-*` | `https://api.xiaomimimo.com/v1` | `https://api.xiaomimimo.com/anthropic` |
| Token Plan | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` | `tp-*` | `https://token-plan-{cn,sgp,ams}.xiaomimimo.com/v1` | `https://token-plan-{cn,sgp,ams}.xiaomimimo.com/anthropic` |

Token Plan region (`cn`, `sgp`, `ams`) is stored in `~/.hawk/provider.json` as `xiaomi_mimo_token_plan_region`. Hawk `/config` prompts for region before key paste on the Token Plan row.

**Auth:** `api-key` header on probe, live fetch, and chat; OpenAI paths also retry once with `Authorization: Bearer` on HTTP 401 (per [OpenAI API](https://platform.xiaomimimo.com/docs/en-US/api/chat/openai-api)).

**Paths (must match Console / docs):**

| Protocol | Pay-as-you-go | Token Plan (per region) |
|----------|---------------|-------------------------|
| OpenAI | `POST https://api.xiaomimimo.com/v1/chat/completions` | `POST {token-plan-*}/v1/chat/completions` |
| Anthropic | `POST https://api.xiaomimimo.com/anthropic/v1/messages` | `POST {token-plan-*}/anthropic/v1/messages` |

GraycodeRouter stores Anthropic **base** as `…/anthropic` (no `/v1`); `AnthropicClient` appends `/v1/messages`, matching [Anthropic API](https://platform.xiaomimimo.com/docs/en-US/api/chat/anthropic-api) and the Python SDK `base_url="https://api.xiaomimimo.com/anthropic"`.

**Legacy:** `xiaomi_mimo` / `XIAOMI_MIMO_API_KEY` / keychain account `xiaomi_mimo_api_key` migrate to pay-as-you-go (`XIAOMI_MIMO_PAYG_API_KEY` / `xiaomi_mimo_payg_api_key`) on load and startup.

**Code:** `graycode-router/catalog/xiaomi/` (URLs), `graycode-router/client/mimo.go` (dual-protocol client), `hawk/cmd/chat_config_xiaomi.go` (region UI).

**Not implemented (out of scope):** ASR/TTS ([Speech Recognition](https://platform.xiaomimimo.com/docs/en-US/api/audio/Speech-Recognition), speech synthesis guides), web-search billing plugins, user toggle for Anthropic-primary routing.

Official: [Token Plan quick access](https://platform.xiaomimimo.com/docs/en-US/price/tokenplan/quick-access), [Pay-as-you-go](https://platform.xiaomimimo.com/docs/en-US/price/pay-as-you-go), [First API call](https://platform.xiaomimimo.com/docs/en-US/quick-start/first-api-call).

### API keys (no prefix rules)

Setup is **gateway-first**: pick the gateway on the Gateways tab, paste any non-empty secret (min length 8; placeholders rejected). GraycodeRouter does **not** validate or infer provider from key prefixes (`sk-ant-`, `tp-`, etc.). Live probe runs only for the selected gateway.

## Flow

```
/config hub (Keys tab — provider first)
  → Select gateway (Add key · Anthropic, …) → paste API key → SaveCredential (single probe) → Models tab
  → Ollama local  → URL → SaveCredential → Discover → ListModels (live) → pick model
  → Pick model    → ListModels (auto) when credentials exist
```

## Host API (hawk uses `internal/graycode-routerclient` only)

- `ResolveCredentialForHost` / `SaveCredentialForHost`
- `ApplyGraycodeRouterCredentials`
- `ListModelsForProvider` — registry-driven live vs cache
- `LocalCredentialInference("ollama")`
- `FormatSetupError(provider, err)`

## Adding a new provider

1. Add one `ProviderSpec` row in `catalog/registry/providers.go`
2. Implement fetcher in `catalog/live/fetchers.go` and register in `Registry`
3. Add deployment row to remote catalog JSON (metadata only; picker uses live list)
4. No hawk changes (registry-driven `/config`)
