# Design Doc: Eyrie Enterprise Gateway

**Status:** Draft / Proposed
**Owner:** Eyrie team
**Last updated:** 2026-06-06
**Scope:** Multi-month product effort. This is an executable design, not a code-session deliverable.

---

## 1. Overview & Competitive Context

Eyrie today is a privacy-first, zero-cloud, Go-static-binary LLM provider runtime. It already
ships several of the *primitives* an enterprise gateway needs — virtual-key budgets, named
routing strategies, an OpenAI-compatible proxy, OIDC keyless auth, OTel-style observability,
and a privacy-preserving audit log — but it lacks the *organizational layer* that turns those
primitives into a managed gateway a team or company can operate: an org/team/user model,
SSO/RBAC, an admin/analytics dashboard UI, a versioned prompt library, canary model testing as
a first-class flow, A2A interop, fine-tuning workflows, and SLA-tiered request scheduling.

This doc designs the **Eyrie Enterprise Gateway** as a cohesive surface built *on top of*
existing eyrie packages, preserving the privacy-first posture (local-first, hash-only audit,
no telemetry exfiltration by default).

### Which Top-20 repos ship these features

From `TOP20_COMPARISON.md` (the eyrie section, lines 89–124):

| Capability | Top-20 repos that ship it | Comparison-doc reference |
|---|---|---|
| Multi-tenant team/project management with SSO/RBAC | LiteLLM, Portkey | `TOP20_COMPARISON.md:96` (P0) |
| Virtual-key budgets + admin dashboard UI | LiteLLM (per-key spend, hard limits, admin dashboard), Portkey | `TOP20_COMPARISON.md:96` |
| Helicone-style analytics dashboard (HQL, sessions, 21+ metrics) | Helicone, Portkey, LangSmith | `TOP20_COMPARISON.md:111` (P1) |
| Versioned prompt library + side-by-side model comparison | Portkey, LangSmith | `TOP20_COMPARISON.md:110` (P1) |
| Canary / blue-green model testing | Portkey (canary routing), LiteLLM | `TOP20_COMPARISON.md:107` (P1) |
| A2A (Agent-to-Agent) protocol | LiteLLM proxy (LangGraph, Vertex Agent Engine, Azure AI Foundry, Bedrock AgentCore) | `TOP20_COMPARISON.md:108` (P1) |
| Fine-tuning workflow integration | OpenAI, Vertex AI, Together AI | `TOP20_COMPARISON.md:123` (P2) |
| Priority queue with SLA tiers | LiteLLM, enterprise LLM gateways | `TOP20_COMPARISON.md:124` (P2) |

Eyrie's gap summary in the comparison doc: **2 P0 / 13 P1 / 5 P2** (`TOP20_COMPARISON.md:271`).
The enterprise gateway is the single largest cluster of those gaps and the one with no
incremental code-session path — it requires a persistent org data model and a UI, which is why
it is treated here as a multi-month effort.

---

## 2. Goals / Non-Goals

### Goals

1. **Org → Team → User → VirtualKey** data model with SSO (OIDC) login and RBAC
   (Owner / Admin / Member), replacing today's single flat `virtual_keys` table.
2. **Admin dashboard UI** for virtual-key budgets: create keys, set USD limits, view live
   spend, rotate provider secrets — surfacing what `storage/budgets.go` already persists.
3. **Analytics dashboard UI** (Helicone-style): per-request segmentation, session tracking,
   a query interface (HQL — "Helicone Query Language") over the request ledger, and the
   21+ metrics eyrie's `MetricsCollector` already computes.
4. **Versioned prompt library** with dynamic variables and side-by-side model comparison
   ("run prompt vX against models A/B/C").
5. **Canary / blue-green model testing** promoted from the existing weighted/preview routing
   primitives into a named, observable production-traffic flow.
6. **A2A protocol** so hawk agents can call external agents (LangGraph, Vertex Agent Engine,
   Azure AI Foundry, Bedrock AgentCore) as tool calls through the eyrie proxy.
7. **Fine-tuning workflow client** that submits training data, polls jobs, and registers the
   resulting model in eyrie's catalog.
8. **Priority queue with SLA tiers** at the rate-limit layer: interactive traffic preempts
   batch/background traffic.

### Non-Goals

- **Eyrie does not become a hosted SaaS.** The enterprise gateway is a self-hostable binary +
  embedded UI. Multi-tenancy means *org isolation within one operator's deployment*, not
  Anthropic-style managed multi-tenant cloud.
- **No billing/payments engine.** Budgets are USD spend *caps and accounting*, not invoicing.
- **No new heavyweight runtime deps by default.** gRPC, Postgres, and Redis remain opt-in
  (the existing `internal/grpc/README.md` policy of not adding `google.golang.org/grpc`
  speculatively is preserved).
- **No model training** — only orchestration of provider-side fine-tuning APIs.
- The browser/IDE/cloud-execution gaps belong to hawk, not this doc.

---

## 3. Architecture

### 3.1 Components

```
                         ┌─────────────────────────────────────────────┐
                         │             Eyrie Enterprise Server          │
                         │            (internal/api/server.go)          │
   Browser ──HTTPS──▶    │  ┌──────────┐  ┌───────────┐  ┌───────────┐  │
   (Admin/Analytics UI)  │  │ Auth /   │  │ Org/RBAC  │  │ Prompt    │  │
                         │  │ SSO(OIDC)│  │ middleware│  │ Library   │  │
   API clients ──────▶   │  └────┬─────┘  └─────┬─────┘  └─────┬─────┘  │
   (OpenAI-compat /v1)   │       │              │              │        │
                         │  ┌────▼──────────────▼──────────────▼─────┐  │
                         │  │   Conversation Engine (conversation/)   │  │
                         │  └────┬──────────────┬──────────────┬─────┘  │
                         │       │              │              │        │
                         │  ┌────▼────┐  ┌──────▼─────┐  ┌─────▼─────┐  │
                         │  │Priority │  │  Router    │  │ A2A       │  │
                         │  │Queue/SLA│  │(canary,LB) │  │ adapter   │  │
                         │  └────┬────┘  └──────┬─────┘  └─────┬─────┘  │
                         │       │      ┌───────▼──────┐       │        │
                         │  ┌────▼──────▼──┐  ┌────────▼───┐   │        │
                         │  │BudgetProvider│  │ Telemetry/ │   │        │
                         │  │(client/)     │  │ Audit/Metrics│ │        │
                         │  └──────┬───────┘  └──────┬─────┘   │        │
                         └─────────┼─────────────────┼─────────┼────────┘
                                   │                 │         │
                          ┌────────▼─────┐    ┌──────▼──────┐  ▼ external agents
                          │ SQLite/PG    │    │ JSONL audit │  (LangGraph, Vertex,
                          │ (storage/)   │    │  + OTLP     │   Azure, Bedrock)
                          └──────────────┘    └─────────────┘
```

New components (green-field):

- **Org/RBAC store** (`storage/org.go`, new): orgs, teams, memberships, roles.
- **SSO/OIDC login provider** (`internal/api/auth_sso.go`, new): reuses the OIDC *fetch/verify*
  machinery patterns from `credentials/oidc.go` but for *inbound* user login (Authorization
  Code + PKCE), distinct from the *outbound* CI credential exchange that file does today.
- **Prompt library store + API** (`storage/prompts.go`, `internal/api/prompts.go`, new).
- **Canary router** (`router/canary.go`, new) wrapping existing strategies.
- **A2A adapter** (`client/a2a/`, new) implementing the `client.Provider` interface so A2A
  targets route through the same pipeline as native providers.
- **Fine-tuning client** (`client/finetune/`, new).
- **Priority queue** (`router/priority.go` or `client/priority.go`, new) at the rate-limit layer.
- **Embedded UI** (`internal/ui/`, `go:embed` single bundle) served from the existing HTTP server.

### 3.2 Data Model

Existing tables (keep, extend):

- `virtual_keys`, `virtual_key_secrets`, `key_budgets`, `request_costs`
  (`storage/budgets.go:62-99`). Today a virtual key is a flat row with no owner.
- `cost_records` analytics table (`storage/analytics.go:86-101`).
- `nodes` (conversation DAG) used by `GetUsageStats` / `GetProviderHealth`
  (`storage/analytics.go:125-215`).

New tables:

```sql
-- Org / team / user / role
CREATE TABLE orgs        (id TEXT PRIMARY KEY, name TEXT, created_at TEXT);
CREATE TABLE teams       (id TEXT PRIMARY KEY, org_id TEXT REFERENCES orgs(id), name TEXT);
CREATE TABLE users       (id TEXT PRIMARY KEY, org_id TEXT, email TEXT, sso_subject TEXT, created_at TEXT);
CREATE TABLE memberships (user_id TEXT, team_id TEXT, role TEXT, -- owner|admin|member
                          PRIMARY KEY (user_id, team_id));
-- Link virtual keys into the hierarchy (add columns to existing virtual_keys)
ALTER TABLE virtual_keys ADD COLUMN org_id  TEXT;
ALTER TABLE virtual_keys ADD COLUMN team_id TEXT;
ALTER TABLE virtual_keys ADD COLUMN owner_user_id TEXT;

-- Prompt library
CREATE TABLE prompts          (id TEXT PRIMARY KEY, org_id TEXT, name TEXT, created_at TEXT);
CREATE TABLE prompt_versions  (id TEXT PRIMARY KEY, prompt_id TEXT, version INTEGER,
                               template TEXT, variables_json TEXT, created_by TEXT, created_at TEXT,
                               UNIQUE(prompt_id, version));

-- Session/request ledger for HQL (richer than today's request_costs)
ALTER TABLE request_costs ADD COLUMN session_id  TEXT;
ALTER TABLE request_costs ADD COLUMN user_id     TEXT;
ALTER TABLE request_costs ADD COLUMN prompt_id   TEXT;
ALTER TABLE request_costs ADD COLUMN tags_json   TEXT;   -- arbitrary segmentation
```

Note `request_costs` already carries `model`, `tokens_in/out`, `cost_usd`, `created_at`
(`storage/budgets.go:87-95`) — adding `session_id`/`user_id`/`tags_json` makes it the HQL fact
table. SQLite remains the default backend (`OpenBudgetStore`, `storage/budgets.go:45`);
Postgres becomes an opt-in backend behind the same `BudgetStore`/`AnalyticsStore` interfaces.

### 3.3 API Surface (additions to `internal/api/server.go:93-113`)

Existing routes to keep: `/health`, `/ready`, `/v1/chat/completions`, `/prompt`,
`/api/usage`, `/api/costs`, `/api/health/providers`, `/rerank`, node CRUD.

New routes:

```
# SSO / session
GET    /auth/login              -> redirect to OIDC IdP (Auth Code + PKCE)
GET    /auth/callback           -> exchange code, mint eyrie session cookie
POST   /auth/logout

# Org / RBAC admin (Admin/Owner only)
GET    /api/orgs/{id}/teams
POST   /api/orgs/{id}/teams
POST   /api/teams/{id}/members          {user, role}
GET    /api/teams/{id}/keys             -> virtual keys scoped to a team
POST   /api/teams/{id}/keys             {name, provider, limit_usd}   (wraps CreateVirtualKey)
PATCH  /api/keys/{id}/budget            {limit_usd}
DELETE /api/keys/{id}

# Analytics dashboard / HQL
GET    /api/analytics/summary?period=24h            (wraps GetUsageStats/GetCostSummary)
POST   /api/analytics/query             {hql: "..."} -> rows
GET    /api/analytics/sessions/{session_id}

# Prompt library
GET    /api/prompts
POST   /api/prompts                     {name}
POST   /api/prompts/{id}/versions       {template, variables}
POST   /api/prompts/{id}/compare        {version, models:[...], variables:{...}}  (side-by-side)

# Canary
POST   /api/canary                      {primary, canary, split_pct}
GET    /api/canary/{id}/report          (latency/cost/error deltas)

# A2A
POST   /v1/a2a/{agent}/invoke           (proxies to external A2A endpoint as a tool call)

# Fine-tuning
POST   /api/finetune/jobs               {provider, base_model, training_file}
GET    /api/finetune/jobs/{id}
```

RBAC is enforced by a middleware layered into the existing `auth()` wrapper
(`internal/api/server.go:115-140`): today `auth()` validates a bearer token and optionally
resolves a virtual key via `VirtualKeyResolver`. The enterprise version extends this to resolve
a **user + role** from either an SSO session cookie or a virtual key, then checks the route's
required role.

### 3.4 Key Flows

**Flow A — SSO login + scoped admin action**

```
User → GET /auth/login → 302 to IdP
IdP  → GET /auth/callback?code=... → server exchanges code (PKCE) → user row (sso_subject)
     → set signed session cookie → 302 to dashboard
User → POST /api/teams/T/keys {limit_usd:50}
     → auth(): cookie → user → membership(role=admin on T) → allow
     → storage.BudgetStore.CreateVirtualKey(...)  (budgets.go:103) with team_id/org_id set
     → 201 {virtual_key_id}
```

**Flow B — Metered chat with budget + analytics + audit**

```
Client → POST /v1/chat/completions  (Bearer = virtual key)
       → auth(): token → VirtualKeyResolver → WithVirtualKey(ctx) (server.go:132-135)
       → Priority queue admits by SLA tier (from key/team config)
       → Conversation Engine → BudgetProvider.Chat (budget_provider.go:75)
           · CheckBudget (budgets.go:148)
           · inner Provider (Router → canary/strategy)
           · RecordUsage (budgets.go:171) + RecordCost (analytics.go:106)
           · Telemetry.EndSpan → MetricsCollector (observability.go:166)
           · AuditSink.Record  (audit.go:73, hashes only)
       → SSE response
```

**Flow C — Prompt compare (side-by-side)**

```
User → POST /api/prompts/P/compare {version:3, models:[A,B], variables:{...}}
     → render template v3 with variables
     → fan-out: Router.Chat once per model (reusing fallback/retry)
     → collect (content, tokens, cost via ActualCostUSD budget_provider.go:127, latency)
     → 200 {results:[{model:A,...},{model:B,...}]}  (UI renders columns)
```

**Flow D — Canary**

```
Admin → POST /api/canary {primary:M1, canary:M2, split_pct:5}
      → CanaryRouter holds two RouteEntry sets; each Chat rolls dice:
         5% → M2 (record under canary bucket), 95% → M1
      → Telemetry tags spans with canary=true; report endpoint diffs
         p50/p95 latency, error_rate, cost_usd between buckets (observability.go metrics)
```

---

## 4. Integration With Existing Eyrie Code (what is reusable today)

This is the crux: most enterprise primitives already exist as *internals* and need an org
layer + UI, not a rewrite.

| Enterprise feature | Existing primitive (reuse) | What's missing |
|---|---|---|
| Virtual-key budgets | `client.BudgetProvider` wraps any `Provider`, enforces per-key USD caps (`client/budget_provider.go:53-107`); SQLite `BudgetStore` with `virtual_keys`/`key_budgets`/`request_costs` (`storage/budgets.go`) | Org/team ownership columns; admin UI; budget alerts |
| Per-key attribution into requests | `WithVirtualKey`/`VirtualKeyFromContext` (`client/budget_provider.go:23-33`), wired through `auth()` via `VirtualKeyResolver` (`internal/api/server.go:42-46, 132-135`) | Map key→user→role instead of key→key |
| Realized cost accounting | `ActualCostUSD` (`client/budget_provider.go:127`), `RecordCost` + `cost_records` (`storage/analytics.go:106`) | session_id/user_id/tags for segmentation |
| 21+ analytics metrics | `MetricsCollector`: request counts, in/out tokens, P50/P95/P99 latency, error rates, cost, cache hit rate; `ExportJSON`/`ExportPrometheus` (`internal/observability/observability.go:247-569`) | Persisted time-series + UI + HQL query layer |
| Usage/cost/health SQL aggregations | `GetUsageStats`, `GetCostSummary`, `GetProviderHealth` (`storage/analytics.go:125-252`) exposed at `/api/usage`, `/api/costs`, `/api/health/providers` (`internal/api/analytics.go`) | HQL free-form query; session drill-down; dashboard front-end |
| Routing strategies (LB) | 6 named strategies — weighted, simple-shuffle, least-busy, latency-based, cost-based, usage-based — with EWMA latency + in-flight + usage telemetry (`router/strategy.go:15-168`); `WithStrategy` option (`router/router.go:49`) | Canary/blue-green as a named, reportable flow |
| Deployment routing + circuit breakers | `DeploymentRouter` with weighted stages, circuit breakers, fallback (`router/deployment_router.go`) | — (canary builds on `selectDeploymentChoice`, `deployment_router.go:563`) |
| Routing preview (no API calls) | `ResolveRouting` / `RoutingPreviewJSON` (`router/preview.go:19-48`) | Surfacing in the canary/admin UI |
| OIDC keyless auth | Full GitHub-Actions OIDC → AWS STS / GCP WIF exchange, stdlib-only (`credentials/oidc.go`) | *Inbound* user SSO login (Auth Code + PKCE) — same crypto/HTTP patterns, opposite direction |
| Privacy-preserving audit | `AuditEvent` (hashes only), `AuditSink`, `JSONLFileSink`, `HashContent` (`internal/observability/audit.go`) | Per-org audit views; OTLP export wiring |
| OpenAI-compatible ingress | `POST /v1/chat/completions` with `user` field already parsed (`internal/api/openai_proxy.go:41`) | Map `user` field → session/user analytics |
| Conversation/session DAG | `conversation.Engine`, `nodes` table powering analytics (`conversation/engine.go`, `storage/analytics.go:125-215`) | Stable `session_id` propagation to ledger |
| ChatOptions extensibility | `ReasoningEffort`, `ThinkingBudgetTokens`, `ResponseFormat`, `VirtualKeyID` already on `ChatOptions` (`client/options.go:18-40`) | Add `PromptID/Version`, `Priority`, `SessionID`, `Tags` |
| gRPC contract | `ChatService` interface + build-tag-guarded server skeleton (`internal/grpc/grpc.go`, `server_grpc.go`, `README.md`) | Generate stubs only if/when adopted (kept opt-in) |
| SDKs | Go/Python/TS SDK stubs (`internal/sdk/{go,python,typescript}`) | Add org/prompt/analytics methods |

**Reuse takeaway:** budgets, metrics, routing strategies, OIDC, audit, and the OpenAI proxy are
*already implemented*. The enterprise effort is dominated by (a) the org/RBAC data model + SSO
login, and (b) the embedded dashboard UI — not by re-implementing gateway internals.

---

## 5. Phased Rollout

### P0 — Foundation (org model, RBAC, admin dashboard, analytics UI)

Maps to the two eyrie **P0** comparison gaps (`TOP20_COMPARISON.md:96`) plus the analytics-UI
P1 (`:111`), because they share the org/auth/UI foundation.

Milestones:

1. **Org/RBAC store** (`storage/org.go`): orgs/teams/users/memberships tables + CRUD; backfill
   existing `virtual_keys` into a default org/team.
2. **SSO login** (`internal/api/auth_sso.go`): OIDC Auth Code + PKCE; signed session cookie;
   reuse OIDC HTTP/JSON patterns from `credentials/oidc.go`.
3. **RBAC middleware**: extend `auth()` (`internal/api/server.go:115`) to resolve user+role and
   gate routes.
4. **Admin dashboard UI** (`internal/ui/`, `go:embed`): list/create teams & virtual keys, set
   budgets (wraps `CreateVirtualKey` / a new `SetBudget`), live spend from `BudgetStore.Get`
   (`storage/budgets.go:200`).
5. **Analytics dashboard UI**: charts over `/api/usage` + `/api/costs`; add session drill-down
   by propagating `session_id` from the OpenAI `user` field / conversation node into
   `request_costs`.
6. **HQL v1**: a safe, allow-listed query DSL compiled to parameterized SQL over `request_costs`
   (no raw SQL passthrough — see Security).

Exit: a self-hosted operator can SSO-login, create a team, mint budgeted keys, and watch
per-key/per-session spend and latency in a browser.

### P1 — Differentiators (prompt library, canary, A2A)

7. **Prompt library** (`storage/prompts.go`, `internal/api/prompts.go`, UI): versioned
   templates, `{{var}}` substitution, side-by-side compare (Flow C) reusing `Router.Chat`.
8. **Canary router** (`router/canary.go`): wrap two entry sets; tag spans; report endpoint diffs
   metrics from `MetricsCollector`. Build on `WithStrategy` (`router/router.go:49`) and
   `selectDeploymentChoice` (`router/deployment_router.go:563`).
9. **A2A adapter** (`client/a2a/`): implement `client.Provider` so A2A targets route through
   `BudgetProvider`/`Router`/audit unchanged; expose `POST /v1/a2a/{agent}/invoke`. Agent cards
   discovered via A2A spec; map `message/send` to `Chat`.

Exit: teams manage prompts as versioned artifacts, run canaries against prod traffic with a
quantified report, and call external agents through the same metered pipeline.

### P2 — Enterprise polish (fine-tuning, SLA priority queue)

10. **Fine-tuning client** (`client/finetune/`): submit/poll for OpenAI, Vertex, Together; on
    completion register the fine-tuned model in the catalog (`catalog/registry`).
11. **Priority queue / SLA tiers** (`router/priority.go`): a bounded priority queue at the
    rate-limit layer; `ChatOptions.Priority` (new) or per-team tier; interactive preempts batch.
    Integrate with existing rate-limiting/retry (`router/retry.go`).
12. **OTLP audit/metrics export** wiring (`OnSpanEnd` hook already exists,
    `observability.go:112`); Postgres/Redis opt-in backends.

Exit: full enterprise parity with LiteLLM/Portkey gateway feature set per the comparison doc.

---

## 6. Build-vs-Buy & Dependencies

| Concern | Decision | Rationale / licensing |
|---|---|---|
| Org/RBAC store | **Build** on existing SQLite `storage/` | No external dep; matches privacy-first, zero-cloud posture. |
| SSO / OIDC login | **Build** (stdlib `net/http` + `crypto`), patterns from `credentials/oidc.go` | Avoids a heavy OIDC lib; the outbound exchange code proves stdlib-only OIDC is viable. Optionally add `github.com/coreos/go-oidc` (Apache-2.0) only if JWKS verification proves burdensome. |
| Session cookies / JWT | **Build** (HMAC-signed cookie, stdlib) | No dep; rotate signing key per deployment. |
| Dashboard UI | **Build** a small bundle, `go:embed` into the binary | Keeps eyrie a single static binary (its core differentiator). Consider Preact/Svelte (MIT) compiled to a static bundle — no runtime server-side framework. |
| HQL parser | **Build** an allow-listed DSL → parameterized SQL | Buying a SQL engine would invite injection; a constrained DSL is safer and smaller. |
| Canary | **Build** on existing router | Pure reuse of `router/strategy.go`. |
| A2A protocol | **Build** a client implementing the open A2A spec | A2A is an open spec; implement as a `Provider`. No mandatory SDK. |
| Fine-tuning | **Build** thin REST clients per provider | Same posture as existing provider clients (stdlib HTTP). |
| Priority queue | **Build** (Go `container/heap`) | Trivial; no dep. |
| Postgres backend | **Buy/adopt** `modernc.org/sqlite` is already in `go.mod`; add `lib/pq`/`pgx` (MIT/BSD) **opt-in** | Only for operators who outgrow SQLite. |
| Redis (distributed state, priority across instances) | **Adopt opt-in** `redis/go-redis` (BSD-2) | Behind interface; not default. |
| gRPC | **Defer** — keep skeleton, do not add `google.golang.org/grpc` until demanded | Per `internal/grpc/README.md` policy. |

**Licensing:** eyrie is MIT (`LICENSE`, "Copyright (c) 2026 Hawk Contributors"). All proposed
default deps (SQLite, go-keyring, OTel, uuid) are already MIT/BSD/Apache-2.0 (`go.mod`). Any
opt-in adds (go-oidc Apache-2.0, pgx MIT, go-redis BSD) are MIT-compatible. **No GPL/AGPL.**
A UI build toolchain (Vite/esbuild, MIT) is a dev-time dependency only — it does not ship in the
binary.

---

## 7. Security / Privacy Considerations

These repos are privacy-first; the enterprise layer must not regress that.

1. **Hash-only audit stays the default.** `AuditEvent` stores only sha256 of prompt/response
   (`internal/observability/audit.go:23-36`, `HashContent` at `:97`). Per-org audit views read
   these hashes — raw text is never persisted. Do not add a "store full prompt" option without
   an explicit, off-by-default org flag.
2. **No telemetry exfiltration by default.** OTLP export uses the existing opt-in `OnSpanEnd`
   hook (`observability.go:112`); the gateway must ship with export disabled and require
   explicit endpoint config.
3. **HQL is an allow-listed DSL, never raw SQL.** Compile to parameterized queries over a fixed
   set of columns/aggregations. This is the single largest injection surface; treat raw-SQL
   passthrough as a non-goal.
4. **Constant-time token comparison** is already used (`internal/api/server.go:142-150`); extend
   it to session-cookie signature verification.
5. **Provider secrets stay encrypted-at-rest / in keyring.** `virtual_key_secrets`
   (`storage/budgets.go:71-76`) holds real provider keys; integrate with the existing keyring
   layer (`credentials/keyring_platform.go`) so they are not stored plaintext in SQLite for
   single-user deployments, and document an encryption-at-rest requirement for the shared DB.
6. **SSO PKCE + state nonce** to prevent code interception/CSRF; short-lived signed session
   cookies with `HttpOnly`, `Secure`, `SameSite=Lax`.
7. **RBAC default-deny.** Unknown routes / missing role → 403, not 200. Backfilled legacy keys
   land in a single default org so no key is silently org-less.
8. **Budget enforcement is fail-closed for known keys, fail-open for unknown** today
   (`budget_provider.go:77` passes through when no key) — for enterprise, add a server config
   `require_virtual_key=true` so unmetered passthrough can be disabled per deployment.
9. **A2A egress allow-list.** External agent endpoints must be explicitly registered per org;
   no arbitrary outbound from a prompt.
10. **Data isolation between orgs** enforced at the query layer (every analytics/prompt query
    scoped by `org_id`), with tests asserting cross-org reads return empty.

---

## 8. Open Questions

1. **Single-tenant per binary vs. multi-org per binary?** This doc assumes multi-org within one
   operator's deployment. Is true cross-customer multi-tenancy ever in scope, or does that stay
   a graycode-cloud concern (`TOP20_COMPARISON.md:33-34`)?
2. **SSO scope:** OIDC only for P0, or do enterprise buyers require SAML (which needs a heavier
   dep)? SAML likely pushes to P2.
3. **Where does `session_id` originate?** From the OpenAI `user` field (`openai_proxy.go:41`), a
   new header, or the conversation node id? Need one canonical source for HQL session tracking.
4. **HQL surface:** how Helicone-compatible must the query language be? Full HQL parity is large;
   a pragmatic subset (filter + group-by + aggregate over a fixed schema) may suffice.
5. **Canary comparison semantics:** automatic response-quality scoring (LLM judge) vs.
   operational metrics only (latency/cost/error)? P1 ships operational; quality judging is a
   later add.
6. **Postgres timing:** at what scale does SQLite (single conn, `SetMaxOpenConns(1)` at
   `budgets.go:50`) become the bottleneck, and should Postgres be P0 for shared deployments?
7. **A2A auth model:** how are credentials to external agents stored — reuse
   `virtual_key_secrets`, or a separate agent-credential store?
8. **Fine-tuned model lifecycle:** who owns the registered model in the catalog (org-scoped vs.
   global), and how is it garbage-collected when a job is deleted?
9. **Priority queue fairness:** strict priority risks starving batch traffic — do we need
   aging/weighted-fair-queueing instead of strict preemption?

---

## 9. Effort Estimate (rough, eng-weeks)

Assumes 1–2 senior Go engineers + 1 front-end engineer for the UI; estimates are build+test+docs.

| Workstream | Phase | Eng-weeks |
|---|---|---|
| Org/team/user/RBAC store + migration/backfill | P0 | 3 |
| SSO (OIDC Auth Code + PKCE) login + sessions | P0 | 3 |
| RBAC middleware + route gating + tests | P0 | 2 |
| Admin dashboard UI (keys, budgets) | P0 | 3 |
| Analytics dashboard UI + session drill-down | P0 | 3 |
| HQL v1 (allow-listed DSL → parameterized SQL) | P0 | 3 |
| **P0 subtotal** | | **~17** |
| Versioned prompt library (store+API+UI) | P1 | 4 |
| Side-by-side model comparison | P1 | 2 |
| Canary / blue-green router + report | P1 | 3 |
| A2A adapter (Provider impl + invoke endpoint) | P1 | 4 |
| **P1 subtotal** | | **~13** |
| Fine-tuning client (3 providers) + catalog registration | P2 | 4 |
| Priority queue / SLA tiers | P2 | 3 |
| OTLP export wiring + Postgres/Redis opt-in backends | P2 | 4 |
| **P2 subtotal** | | **~11** |
| Cross-cutting: security review, docs, SDK updates, e2e | all | 4 |
| **Total** | | **~45 eng-weeks (~9–11 calendar months for a small team)** |

The estimate is front-loaded into P0 because the org/auth/UI foundation is the genuinely new
surface; P1/P2 lean heavily on the already-implemented routing, budget, metrics, and audit
primitives cataloged in Section 4.
