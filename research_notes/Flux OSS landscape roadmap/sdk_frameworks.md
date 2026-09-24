# Flux OSS landscape roadmap: SDK and framework capability donors

**Research cutoff and access date:** 2026-09-24. All web links in this document were checked on 2026-09-24 unless a page itself reports a different verification date. Repository counts and release labels are snapshots, not quality scores.

**Evidence labels:**

- **Shipped:** visible in current source, API reference, package metadata, or a stable documentation page.
- **Beta/experimental:** the project explicitly labels the feature beta, experimental, preview, or otherwise unstable.
- **Claim:** a project or vendor description that was not independently benchmarked in this research.
- **Roadmap:** future or planned behavior; it is not treated as an existing Flux capability.

**Flux-specific observations** in this document come from the local checkout on 2026-09-24. Public repository links are included for navigability, but the local checkout is authoritative for the current branch.

## Research scope and selection

### Takeaway

Flux should remain a provider-neutral generation engine, not become an agent framework, application server, or UI. The most valuable donors fall into three groups: (1) SDK/runtime contracts (LiteLLM, Vercel AI SDK, any-llm, Pydantic AI, OpenAI Agents SDK, LangChain, Google Genkit, and Google ADK); (2) host/orchestration boundaries (LangGraph, Microsoft Agent Framework and its AutoGen/Semantic Kernel lineage, Haystack, LlamaIndex, BAML, Mastra, Strands, DSPy, Agno, and CrewAI); and (3) product-boundary comparators (Dify and Open WebUI). The final top-20 below is a research shortlist, not a recommendation to embed all twenty projects.

### Cited Findings

- Flux's accepted boundary says that `engine`, `llm`, `graph`, and `tools` are host contracts, while credentials, catalog, routing, transports, resilience, usage, and provider telemetry remain engine concerns; the host owns UX, agent loops, tools, permissions, sessions, checkpoints, and product semantics. [Flux host–engine boundary](https://github.com/GrayCodeAI/flux/blob/main/docs/architecture/HOST-ENGINE-BOUNDARY.md) (accessed 2026-09-24).
- Flux's public facade is contract v2, exposes a pull-based cancellable stream, emits a route before provider events, treats unknown event types as additive, and explicitly leaves tool execution and the next model turn to the host. [Flux engine contract](https://github.com/GrayCodeAI/flux/blob/main/engine/engine.go) and [Flux stream implementation](https://github.com/GrayCodeAI/flux/blob/main/engine/stream.go) (accessed 2026-09-24).
- The project already has useful donor-like primitives: opaque provider blocks, raw tool arguments, normalized usage semantics, typed engine errors, model provenance fields, a pull stream, retry configuration, and a data-only graph vocabulary. [Flux DTOs](https://github.com/GrayCodeAI/flux/blob/main/llm/types.go), [tool contracts](https://github.com/GrayCodeAI/flux/blob/main/tools/tool.go), [error types](https://github.com/GrayCodeAI/flux/blob/main/engine/errors.go), and [graph contracts](https://github.com/GrayCodeAI/flux/blob/main/graph/graph.go) (accessed 2026-09-24).
- GitHub repository pages and APIs were used to verify public URLs, language, license signals, and recent activity. A high star count was not used as the primary selection criterion; directness to Flux's boundary, distinct capability patterns, and current maintenance were more important. [GitHub API repository metadata example: Vercel AI](https://api.github.com/repos/vercel/ai) and [GitHub API repository metadata example: Flux](https://api.github.com/repos/GrayCodeAI/flux) (accessed 2026-09-24).

### Final top-20 selection

The order is by practical relevance to Flux, not by popularity. “Direct donor” means Flux can learn a contract or implementation pattern; “host donor” means the project is useful mainly to clarify what must stay outside Flux; “product comparator” means it is valuable for UI, deployment, governance, or product-boundary lessons rather than for a library dependency.

| # | Project or family | Why it belongs in the final set | Flux use | Boundary signal |
|---:|---|---|---|---|
| 1 | [LiteLLM](https://github.com/BerriAI/litellm) | Best public reference for gateway routing, deployment fallback, spend/usage, cache behavior, and end-to-end GenAI OTel. | Router semantics, cost accounting, OTel privacy defaults, passthrough rules. | Proxy/admin/multi-tenant control plane stays host-side. |
| 2 | [Vercel AI SDK](https://github.com/vercel/ai) | Explicit language-model specification, provider registry, middleware hooks, typed tool loop, stream parts, and telemetry. | Model port, middleware, provider metadata, event lifecycle, privacy filtering. | Core is a donor; UI, RSC, harnesses, and agent loop are not. |
| 3 | [any-llm Python](https://github.com/mozilla-ai/any-llm) and [any-llm-go](https://github.com/mozilla-ai/any-llm-go) | Closest newly discovered Go-oriented provider abstraction, with optional capability interfaces and normalized errors. | Go package shape, optional provider interfaces, capability declarations, error sentinels. | Provider library only; gateway/platform is separate. |
| 4 | [Pydantic AI](https://github.com/pydantic/pydantic-ai) | Strongest typed message-part, tool-schema, validation, usage, test-model, and evaluation patterns. | Canonical parts, schema validation, offline conformance fixtures, OTel instrumentation. | Agent loop, harness, memory, durable runtime, and CLI stay host-side. |
| 5 | [OpenAI Agents SDK](https://github.com/openai/openai-agents-python) | Unusually clear stream-completion semantics, usage aggregation, raw usage preservation, tool lifecycle, and trace hierarchy. | Stream terminal rules, usage provenance, cancellation/interrupt semantics, model-adapter boundary. | Runner, handoffs, sessions, guardrails, and agent state are explicitly host concerns. |
| 6 | [LangChain](https://github.com/langchain-ai/langchain) | Broadest practical comparison for model profiles, provider-native content blocks, structured output, retries, and middleware. | Capability profiles, content-block normalization, model-call middleware, retry policy. | Agent memory, retrieval ecosystem, LangSmith, and agent loop are host concerns. |
| 7 | [LangGraph](https://github.com/langchain-ai/langgraph) | Best reference for durable stateful graph execution, checkpoint modes, interrupts, and typed stream projections. | Graph contract vocabulary and lifecycle vocabulary only. | No graph runtime, checkpointer, or scheduler in Flux. |
| 8 | [Google Genkit](https://github.com/genkit-ai/genkit) | Action/flow/plugin architecture is available in Go as well as TypeScript and Python, with typed schemas, streaming, interrupts, and developer tooling. | Go action/flow ergonomics, plugin lifecycle, typed stream outputs, trace boundaries. | Flows, agent runtime, Dev UI, and deployment are host concerns; Agents API is beta. |
| 9 | [Google ADK](https://github.com/google/adk-python) | Rich event semantics (`partial`, complete, interrupted, final response), callbacks/plugins, evaluation, artifacts, and multi-language support. | Event flags, callback/telemetry hooks, capability and artifact references, conformance cases. | Runner, sessions, memory, workflow agents, and web UI are host concerns. |
| 10 | [Microsoft Agent Framework](https://github.com/microsoft/agent-framework), with [AutoGen](https://github.com/microsoft/autogen) and [Semantic Kernel](https://github.com/microsoft/semantic-kernel) lineage | Current Microsoft successor combines AutoGen's simple agent patterns with Semantic Kernel's state, type safety, filters, telemetry, and graph workflows. | Layered model/function middleware, typed routing, workflow event vocabulary, OTel sensitivity controls. | Agent/session/workflow runtime remains host-side; AutoGen is now maintenance mode. |
| 11 | [Haystack](https://github.com/deepset-ai/haystack) | Mature component/pipeline contracts, validation, serialization, async cancellation, and pluggable tracing. | Component-like extension points, cancellation invariants, custom tracer interface, stream chunks. | Pipelines, RAG, document stores, loops, and agent components are host concerns. |
| 12 | [LlamaIndex](https://github.com/run-llama/llama_index) | Event-driven workflows with typed events, graph validation, streaming, state/resources, durable execution, and a large RAG ecosystem. | Event envelope, state/resource separation, workflow validation, data-aware capability boundaries. | Workflow execution, indexing, retrieval, and document processing are host concerns. |
| 13 | [BAML](https://github.com/BoundaryML/baml) | Schema-first structured output, generated contracts, streaming typed data, editor/test tooling, and multi-language clients. | Schema descriptor, validator/repair hook, golden structured-output fixtures. | DSL, prompt editor, optimizer, and evaluation application stay outside the engine. |
| 14 | [Mastra](https://github.com/mastra-ai/mastra) | Strong TypeScript reference for model routing, typed workflows, durable suspension, storage, observability, and evals. | Router, typed workflow boundary, lifecycle hooks, eval-friendly trace fields. | Core framework, Studio, memory, and server are host concerns; enterprise code is separately licensed. |
| 15 | [Strands Agents](https://github.com/strands-agents/sdk-python) | Minimal model-driven agent protocol with explicit `invoke_async`/`stream_async`, lifecycle hooks, sessions, OTel, MCP, and multi-agent patterns. | Minimal adapter protocol, hook points, stream event design, session-independent usage. | Agent loop, tool execution, sandboxes, and multi-agent runtime stay host-side. |
| 16 | [DSPy](https://github.com/stanfordnlp/dspy) | Best donor for evaluation metrics, program signatures, prompt/program optimization, and versioned optimization artifacts. | Trace/metric export and deterministic evaluation hooks, not the optimizer itself. | Prompt/program optimization, fine-tuning, and agent modules stay host/tooling-side. |
| 17 | [Agno](https://github.com/agno-agi/agno) | Useful separation between SDK, AgentOS runtime, and control plane; explicit storage, approval, RBAC, and OTel surfaces. | Storage/usage interfaces, approval event vocabulary, runtime-vs-library boundary. | AgentOS, control plane, memory, knowledge, UI, and scheduling stay host-side. |
| 18 | [CrewAI](https://github.com/crewAIInc/crewAI) | Clear separation between autonomous Crews and event-driven Flows with state, branching, routing, and config-as-code. | Host workflow event/state vocabulary and declarative configuration lessons. | Crews, tasks, roles, memory, and execution engine are host concerns. |
| 19 | [Dify](https://github.com/langgenius/dify) | Product reference for visual workflows, prompt versioning/testing, model management, observability, APIs, and deployment modes. | Prompt/catalog/eval UX requirements and provider configuration semantics. | UI, app builder, RAG, tenant gateway, and commercial control plane are explicitly outside Flux. |
| 20 | [Open WebUI](https://github.com/open-webui/open-webui) | Product reference for self-hosted model UX, plugins, RBAC, artifacts, analytics, and explicit tool-loop ownership. | Extension ownership rules, model connection semantics, usage/UI requirements, safety warnings. | UI, auth, RAG, tool execution, and user-facing application semantics are outside Flux. |

### Activity, language, and license snapshot

The following is a compact verification record. “Recent push” means the GitHub API or repository page showed a push in the last few days before the cutoff; a page can change after this snapshot.

| Family | Language(s) | License/governance signal | Activity signal at cutoff |
|---|---|---|---|
| LiteLLM | Python plus Rust components | MIT outside `enterprise/`; enterprise directory separately licensed. [License](https://github.com/BerriAI/litellm/blob/main/LICENSE) (accessed 2026-09-24) | GitHub API showed a push on 2026-09-23. [API](https://api.github.com/repos/BerriAI/litellm) (accessed 2026-09-24) |
| Vercel AI SDK | TypeScript | Apache-2.0. [License](https://github.com/vercel/ai/blob/main/LICENSE) (accessed 2026-09-24) | API showed a push on 2026-09-23. [API](https://api.github.com/repos/vercel/ai) (accessed 2026-09-24) |
| any-llm | Python; new Go port | Apache-2.0 for both repositories. [Python license](https://github.com/mozilla-ai/any-llm/blob/main/LICENSE) and [Go license](https://github.com/mozilla-ai/any-llm-go/blob/main/LICENSE) (accessed 2026-09-24) | Python API push 2026-09-23; Go port was created in 2026 and had roughly 80 commits at the cutoff. [Python API](https://api.github.com/repos/mozilla-ai/any-llm), [Go repository](https://github.com/mozilla-ai/any-llm-go) (accessed 2026-09-24) |
| Pydantic AI | Python | MIT. [License](https://github.com/pydantic/pydantic-ai/blob/main/LICENSE) (accessed 2026-09-24) | API showed a push on 2026-09-23. [API](https://api.github.com/repos/pydantic/pydantic-ai) (accessed 2026-09-24) |
| OpenAI Agents SDK | Python and TypeScript | MIT. [Repository](https://github.com/openai/openai-agents-python) (accessed 2026-09-24) | Python API showed a push on 2026-09-23. [API](https://api.github.com/repos/openai/openai-agents-python) (accessed 2026-09-24) |
| LangChain / LangGraph | Python and JavaScript/TypeScript | MIT. [LangChain API](https://api.github.com/repos/langchain-ai/langchain), [LangGraph API](https://api.github.com/repos/langchain-ai/langgraph) (accessed 2026-09-24) | Both APIs showed pushes on 2026-09-23. |
| Google Genkit / ADK | TypeScript, Go, Python, Dart; ADK also Java/Kotlin | Apache-2.0 repositories. [Genkit API](https://api.github.com/repos/genkit-ai/genkit), [ADK API](https://api.github.com/repos/google/adk-python) (accessed 2026-09-24) | Both APIs showed pushes on 2026-09-23. |
| Microsoft lineage | C#, Python; Agent Framework also has .NET/Go workflow documentation | Agent Framework and Semantic Kernel repositories show MIT; AutoGen documentation/code has split CC-BY/MIT terms. [Agent Framework license](https://github.com/microsoft/agent-framework/blob/main/LICENSE), [AutoGen repository](https://github.com/microsoft/autogen), [Semantic Kernel API](https://api.github.com/repos/microsoft/semantic-kernel) (accessed 2026-09-24) | Semantic Kernel API push 2026-09-19; AutoGen README says maintenance mode and the API last push was 2026-04-15; Agent Framework is the recommended successor. |
| Haystack / LlamaIndex | Python | Apache-2.0 and MIT respectively. [Haystack API](https://api.github.com/repos/deepset-ai/haystack), [LlamaIndex API](https://api.github.com/repos/run-llama/llama_index) (accessed 2026-09-24) | Both APIs showed pushes on 2026-09-23. |
| BAML | Rust plus generated clients | Apache-2.0 repository. [API](https://api.github.com/repos/BoundaryML/baml) (accessed 2026-09-24) | API showed a push on 2026-09-23. |
| Mastra | TypeScript | Apache-2.0 core; `ee/` directories use the Mastra Enterprise License. [Repository licensing](https://github.com/mastra-ai/mastra/blob/main/README.md) (accessed 2026-09-24) | Repository page showed a highly active 2026 codebase; current package/repo pages showed recent releases. [Repository](https://github.com/mastra-ai/mastra) (accessed 2026-09-24) |
| Strands | Python and TypeScript | Apache-2.0. [Python repository](https://github.com/strands-agents/sdk-python), [harness licensing](https://github.com/strands-agents/harness-sdk/blob/main/LICENSE.APACHE) (accessed 2026-09-24) | Python repository and harness pages showed active 2026 development. |
| DSPy | Python | MIT. [License](https://github.com/stanfordnlp/dspy/blob/main/LICENSE) (accessed 2026-09-24) | Release page showed 3.3.1 on 2026-08-21 and a newer beta line. [Releases](https://github.com/stanfordnlp/dspy/releases) (accessed 2026-09-24) |
| Agno / CrewAI | Python | Apache-2.0 and MIT respectively. [Agno API](https://api.github.com/repos/agno-agi/agno), [CrewAI API](https://api.github.com/repos/crewAIInc/crewAI) (accessed 2026-09-24) | Both APIs showed pushes on 2026-09-23. |
| Dify / Open WebUI | TypeScript plus Python; Python backend/frontend stacks | Dify uses a modified Apache-2.0-based license with multi-tenant and branding conditions. Open WebUI uses a custom license with branding conditions; older contributions retain other terms. [Dify license](https://github.com/langgenius/dify/blob/main/LICENSE), [Open WebUI license](https://github.com/open-webui/open-webui/blob/main/LICENSE) (accessed 2026-09-24) | Both APIs showed pushes on 2026-09-23. |

**Selection decisions for projects not in the final 20:**

- **Portkey AI Gateway** is a useful secondary gateway comparison, with routing, retries, guardrails, logs, and a unified endpoint, but LiteLLM covers the same donor space with a more directly inspectable OSS/proxy boundary. [Portkey gateway](https://github.com/Portkey-AI/gateway) (accessed 2026-09-24).
- **Instructor** and **Atomic Agents** are excellent focused donors for schema validation and small composable agents, but BAML, Pydantic AI, LangChain, and Pydantic Graph cover those patterns with broader production context. [Instructor](https://github.com/567-labs/instructor), [Atomic Agents](https://github.com/Eigenwise/atomic-agents) (accessed 2026-09-24).
- **Letta** is a valuable memory/context-management and stateful-agent comparator, but its center of gravity is a persistent agent server and ADE rather than a provider-neutral engine. [Letta](https://github.com/letta-ai/letta) (accessed 2026-09-24).
- **Continue** is a host/product reference, but its repository is described as no longer actively maintained and read-only; it should not drive Flux architecture. [Continue repository](https://github.com/continuedev/continue) (accessed 2026-09-24).
- **MCP, AG-UI, OpenResponses, and workflow products** are protocols or host integration surfaces, not competing provider runtimes. They are useful compatibility targets, not top-20 framework donors. [MCP](https://modelcontextprotocol.io/), [Google ADK/AG-UI integration announcement](https://developers.googleblog.com/delight-users-by-combining-adk-agents-with-fancy-frontends-using-ag-ui) (accessed 2026-09-24).

### Inferences

- The strongest Flux roadmap is not “copy an agent framework”; it is to make the existing model-call contract more explicit, testable, provider-neutral, and observable.
- Go-specific donors are unusually important for Flux: any-llm-go, Genkit Go, Google ADK Go, and the Microsoft workflow documentation show that a Go engine can expose typed provider/flow primitives without adopting Python agent semantics.
- Dify and Open WebUI belong in the final set precisely because they make the non-goals visible: UI, auth, RAG, prompt management, multi-tenant control planes, and application deployment are separate products.
- AutoGen and Semantic Kernel should be studied as design lineages and migration precedents, but new capability work should follow Microsoft Agent Framework when comparing current Microsoft agent features.

### Gaps

- GitHub API rate limits prevented a uniform API snapshot for a few repositories; those rows use their official repository pages and package/release pages instead. Exact stars, forks, and issue counts should be refreshed before publication.
- Several projects market “production-ready,” “100+ providers,” “millions of downloads,” or performance numbers. Those are retained as project claims, not independently verified conclusions.
- The research did not clone and execute every project. Source/docs establish shipped interfaces and stated behavior, not correctness under all providers.
- Version labels can move quickly after the cutoff, especially for AI SDK 7, Pydantic AI Harness, Genkit Agents, Google ADK 2.0, and Microsoft Agent Framework workflows.

## Project findings and capability patterns

### 1. LiteLLM — gateway and runtime donor

**Evidence status:** The Python SDK, gateway/proxy, router, caching, observability documentation, and OTel v2 documentation are shipped. OTel v2 is explicitly opt-in and off by default. [LiteLLM documentation](https://docs.litellm.ai/docs), [OTel v2](https://docs.litellm.ai/docs/observability/opentelemetry_v2) (accessed 2026-09-24).

**Capability patterns:**

- The documented request path is virtual-key/budget authorization, rate limiting, router load balancing/fallback/retry, provider translation, response, then asynchronous spend/logging updates. This is a useful decomposition for Flux's route, resilience, usage, and telemetry phases. [Life of a Request](https://docs.litellm.ai/docs/proxy/architecture) (accessed 2026-09-24).
- Retry and fallback have distinct scopes: retries stay within a model group/deployment set, while fallbacks move to another model group. Flux currently exposes route and attempt metadata; this distinction should remain explicit rather than collapsing both into “retry.” [LiteLLM routing architecture](https://docs.litellm.ai/docs/proxy/architecture) (accessed 2026-09-24).
- Image URL handling demonstrates an explicit compatibility policy: pass a URL through when supported, otherwise download and convert to base64 with a size cap and cache. Flux should copy the policy and observability, not silently fetch arbitrary URLs. [LiteLLM image URL handling](https://docs.litellm.ai/docs/proxy/architecture) (accessed 2026-09-24).
- OTel v2 uses canonical `gen_ai.*` attributes, content capture off by default, trace-level filtering, and separate span kinds for model, tool, datastore, guardrail, and MCP work. This is a strong privacy and interoperability baseline. [OTel v2 capture and span attributes](https://docs.litellm.ai/docs/observability/opentelemetry_v2) (accessed 2026-09-24).
- Caching documentation explicitly warns that provider-specific parameters can change output and therefore must be represented in cache keys; Anthropic-format and passthrough routes may bypass ordinary response caching. [LiteLLM caching](https://docs.litellm.ai/docs/proxy/caching) (accessed 2026-09-24).

**Boundary:** LiteLLM's virtual keys, teams, budgets, admin UI, database, and multi-tenant gateway are platform concerns. Flux already owns provider credentials and routing but should not absorb an organization control plane merely because LiteLLM offers one. [LiteLLM architecture](https://docs.litellm.ai/docs/proxy/architecture) (accessed 2026-09-24).

**Assessment:** Highest-value donor for route/attempt semantics, spend normalization, cache-key discipline, and privacy-safe OTel. Treat its OpenAI-shaped response as a compatibility surface, not as Flux's canonical internal contract.

### 2. Vercel AI SDK — provider specification and middleware donor

**Evidence status:** AI SDK 7 documentation and the `LanguageModelV4` TypeScript specification are shipped source. Agent, harness, workflow, and some newer surfaces are separate product areas; the Python SDK is labeled beta on the documentation site. [AI SDK Core](https://ai-sdk.dev/docs/ai-sdk-core), [language-model-v4.ts](https://github.com/vercel/ai/blob/main/packages/provider/src/language-model/v4/language-model-v4.ts) (accessed 2026-09-24).

**Capability patterns:**

- The provider layer is a small explicit model interface with a specification version, provider identity, generate/stream operations, and provider metadata. That is a better extension seam than exposing each vendor SDK throughout Flux. [LanguageModelV4 source](https://github.com/vercel/ai/blob/main/packages/provider/src/language-model/v4/language-model-v4.ts) (accessed 2026-09-24).
- `customProvider` and `createProviderRegistry` centralize aliases, model restrictions, default settings, fallback providers, files/skills interfaces, and model-kind-specific registries. This maps well to Flux's existing catalog/gateway/route split. [Provider and model management](https://ai-sdk.dev/docs/ai-sdk-core/provider-management) (accessed 2026-09-24).
- Middleware has three useful hooks: `transformParams`, `wrapGenerate`, and `wrapStream`. Middleware order is defined, and streaming middleware must preserve stream-part identity and block boundaries. Flux should expose a similarly narrow model-call middleware chain. [Language Model Middleware](https://ai-sdk.dev/docs/ai-sdk-core/middleware) (accessed 2026-09-24).
- Tool definitions separate schema, execution, strictness, examples, approval, and tool context. Multi-step calls expose a stopping condition and accumulated `responseMessages`; Flux should expose the same distinction between “model requested a tool” and “host executed it.” [Tool Calling](https://ai-sdk.dev/docs/ai-sdk-core/tools-and-tool-calling) (accessed 2026-09-24).
- Telemetry records operation, provider call, and tool spans, filters runtime/tool context before export, follows OTel GenAI conventions, and supports explicit content recording controls. The separation between full runtime context and telemetry-filtered context is directly relevant to Flux's secret-handling boundary. [Telemetry](https://ai-sdk.dev/docs/ai-sdk-core/telemetry) (accessed 2026-09-24).

**Boundary:** AI SDK Core is a donor. AI SDK UI, React/Svelte/Vue hooks, RSC, Generative UI, terminal UI, harness adapters, and the built-in multi-step agent surface are host/product layers. Flux should not become a TypeScript UI toolkit or a harness runtime.

**Assessment:** Best donor for a versioned provider port, composable middleware, provider metadata, and privacy-aware telemetry. Copy semantics, not UI or agent abstractions.

### 3. any-llm — closest Go-oriented provider abstraction

**Evidence status:** The Python library and the official Go port are shipped repositories. The Go port is new relative to Flux's mature codebase, so its provider matrix should be treated as a current snapshot rather than a guarantee of parity. [Python repository](https://github.com/mozilla-ai/any-llm), [Go repository](https://github.com/mozilla-ai/any-llm-go), [Go provider package](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers) (accessed 2026-09-24).

**Capability patterns:**

- Python re-exports OpenAI completion types and extends them where providers add reasoning content; streaming exposes completion chunks and reasoning deltas, while response format accepts schemas and tool calls remain visible. This is a pragmatic compatibility strategy, but it also demonstrates why Flux needs its own canonical parts and provider blocks. [Completion types](https://docs.mozilla.ai/api-reference/completion-1) and [AnyLLM interface](https://docs.mozilla.ai/api-reference/any-llm) (accessed 2026-09-24).
- The Python exception hierarchy is opt-in and preserves provider, status, code, parameter, and original exception information. Flux already has a structured provider error, so the donor is the stable category set and `errors.Is`/`errors.As` ergonomics rather than the exact class names. [Unified exceptions](https://docs.mozilla.ai/api-reference/exceptions) (accessed 2026-09-24).
- The Go port defines a small `Provider` interface with `Name`, `Completion`, and `CompletionStream`, then adds optional interfaces for embeddings, model listing, capabilities, and error conversion. This is an excellent pattern for avoiding a monolithic provider interface. [Go provider interface](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers) and [contributor interface assertions](https://github.com/mozilla-ai/any-llm-go/blob/main/CONTRIBUTING.md) (accessed 2026-09-24).
- The Go port uses channels plus a separate error channel for streaming, and its provider matrix explicitly marks unsupported combinations such as reasoning or embeddings instead of pretending every provider is equivalent. [Go README](https://github.com/mozilla-ai/any-llm-go) and [provider matrix](https://github.com/mozilla-ai/any-llm-go/blob/main/docs/providers.md) (accessed 2026-09-24).
- The optional `ProviderData`/provider-specific metadata escape hatch and the OpenAI-compatible base provider are useful compatibility patterns, provided they remain adapter-internal or explicitly versioned. [Go provider types](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers) and [OpenAI-compatible provider](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers/openai) (accessed 2026-09-24).

**Boundary:** any-llm is a provider library, not an agent runtime. Its optional gateway, virtual keys, and hosted platform are separate products; Flux should borrow the library shape and capability declarations, not its control plane.

**Assessment:** Highest-priority Go donor. The main risk is copying an OpenAI-shaped response too literally and losing provider-native reasoning, signed thinking blocks, or media semantics.

### 4. Pydantic AI — typed messages, schemas, and testability donor

**Evidence status:** Core typed agent/model/tool functionality, streaming, MCP, OTel instrumentation, and offline testing are documented. Durable execution and the Harness are integration/package surfaces; their availability should be checked per deployment rather than assumed to be part of the core. [Pydantic AI overview](https://pydantic.dev/docs/ai/overview/) and [repository](https://github.com/pydantic/pydantic-ai) (accessed 2026-09-24).

**Capability patterns:**

- Messages are represented as typed parts such as user prompt, text response, tool call, and tool return, with request/response usage, model name, timestamps, run ID, and conversation ID. This is a stronger model than Flux's current `Content` plus parallel `ToolUse`/`ToolResults` fields for new contracts. [Pydantic AI tools and message output](https://pydantic.dev/docs/ai/tools-toolsets/tools/) (accessed 2026-09-24).
- Function signatures and docstrings generate JSON Schema; arguments are validated before execution, and tool validation errors can be returned to the model for another attempt. Output types are validated and can trigger reflection/self-correction. [Function tools](https://pydantic.dev/docs/ai/tools-toolsets/tools/) and [Pydantic AI overview](https://pydantic.dev/docs/ai/overview/) (accessed 2026-09-24).
- The `TestModel` runs without an API key, which is a strong pattern for deterministic provider-adapter and host-contract tests. Flux should have an equivalent offline fake model/stream source for conformance tests. [Pydantic AI overview](https://pydantic.dev/docs/ai/overview/) (accessed 2026-09-24).
- Instrumentation emits standard OTel spans for model and tool calls, and Pydantic Evals tests agent behavior separately from the core runtime. This supports a boundary where Flux exports normalized events/usage while hosts own evaluations. [Instrumentation and evaluations](https://pydantic.dev/docs/ai/overview/) (accessed 2026-09-24).
- `pydantic-graph` is deliberately a separate typed state-machine library. That separation is directly relevant: graph vocabulary can be shared without making the model engine own graph execution. [Pydantic Graph](https://pydantic.dev/docs/ai/graph/) (accessed 2026-09-24).

**Boundary:** Pydantic AI's `Agent`, Runner-equivalent loop, Harness, memory, planning, subagents, CLI, built-in web/voice interfaces, durable integrations, and Evals product are host concerns. Flux should adopt typed DTO and validation patterns only.

**Assessment:** Best donor for schema-first contracts and deterministic tests. Do not import Pydantic's agent semantics into `engine`.

### 5. OpenAI Agents SDK — stream completion, usage, and tool lifecycle donor

**Evidence status:** The Python SDK and TypeScript SDK are official. The docs call the Python package production-ready, but many features are OpenAI-specific or hosted; those are not portable capabilities. [Official SDK selection page](https://developers.openai.com/api/docs/guides/agents/sdk) and [Python repository](https://github.com/openai/openai-agents-python) (accessed 2026-09-24).

**Capability patterns:**

- The documentation makes the boundary unusually explicit: `Agent` plus `Runner` manages turns, tools, guardrails, handoffs, and sessions; applications that want to own the loop should use the lower-level Responses API instead. This is a direct endorsement of Flux's “engine emits requests, host owns the loop” rule. [Agents](https://openai.github.io/openai-agents-python/agents) (accessed 2026-09-24).
- Streaming has a crucial completion rule: callers must consume the event iterator to the end because session persistence, approval bookkeeping, or history compaction can finish after the last visible token. `cancel()` can stop immediately or after the current turn, and interrupted runs expose resumable state. [Streaming](https://openai.github.io/openai-agents-python/streaming) (accessed 2026-09-24).
- The SDK has explicit raw response, run-item, and agent-update stream events, plus nested-agent streaming and a distinction between a handoff request and a tool call. Flux should use stable event phases and correlation IDs rather than infer turn boundaries from text deltas. [Streaming](https://openai.github.io/openai-agents-python/streaming) and [agent tool stream events](https://openai.github.io/openai-agents-python/ref/agent) (accessed 2026-09-24).
- Usage is aggregated across model calls, tool calls, handoffs, and compaction. `preserve_raw_usage` retains provider-specific usage fields, while third-party adapters may require `include_usage=True`; omission and zero must remain distinguishable. [Usage](https://openai.github.io/openai-agents-python/usage) (accessed 2026-09-24).
- Tracing has workflow, task, turn, agent, generation, function, guardrail, handoff, and audio spans. Its default sensitive-data behavior is permissive, and tracing is unavailable under zero-data-retention organizations; Flux should default to metadata-only and make content capture explicit. [Tracing](https://openai.github.io/openai-agents-python/tracing) (accessed 2026-09-24).
- Tools distinguish local function tools, provider-executed tools, hosted MCP, computer/code tools, deferred approval, and output trimming. Flux should expose tool requests and provider state but not execute or approve them. [Tools](https://openai.github.io/openai-agents-python/tools) (accessed 2026-09-24).

**Boundary:** Runner, handoffs, guardrails, sessions, hosted tools, computer use, voice/realtime, and agent tracing are host or platform concerns. The portable donor is the model adapter and stream/usage contract.

**Assessment:** High-value donor for terminal stream semantics, raw usage preservation, and cancellation state. Treat OpenAI-only feature claims as non-portable.

### 6. LangChain — broad model and middleware comparison

**Evidence status:** The current LangChain docs and integrations are shipped; the ecosystem is large and changes quickly. [LangChain models](https://docs.langchain.com/oss/python/langchain/models) and [repository](https://github.com/langchain-ai/langchain) (accessed 2026-09-24).

**Capability patterns:**

- The standard chat-model interface supports invoke, stream, batch, tool calling, structured output, multimodal blocks, reasoning, and provider-specific parameters. This is a useful capability matrix for Flux's model catalog and adapter contract. [Models](https://docs.langchain.com/oss/python/langchain/models) (accessed 2026-09-24).
- `model.profile` exposes context limits, image inputs, reasoning output, tool calling, and related capabilities, with much of the data sourced from models.dev. Flux already has context, pricing, capability, source, and live metadata fields; a typed, provenance-bearing profile is the next step. [Model profiles](https://docs.langchain.com/oss/python/langchain/models) and [models.dev](https://models.dev/) (accessed 2026-09-24).
- Message content may be a string or provider-native content blocks, while the framework exposes a normalized content-block view. This supports a dual representation: canonical parts for hosts plus opaque provider-native blocks for replay. [Messages](https://docs.langchain.com/oss/python/langchain/messages) (accessed 2026-09-24).
- Default model retries use exponential backoff for network, 429, and 5xx errors, but not ordinary client errors; the number of retries is configurable. This is a useful default, with Flux retaining provider-specific retryability and retry budgets. [Models: connection resilience](https://docs.langchain.com/oss/python/langchain/models) (accessed 2026-09-24).
- Middleware offers before/after-agent, before/after-model, and wrap-model-call hooks, including model/tool retries, tool-error conversion, content transformation, and stream transformers. Flux should adopt only the model-call layer, not agent state or tool execution middleware. [Custom middleware](https://docs.langchain.com/oss/python/langchain/middleware/custom) and [built-in middleware](https://docs.langchain.com/oss/python/langchain/middleware/built-in) (accessed 2026-09-24).

**Boundary:** LangChain agents, LangSmith, retrieval/vector stores, memory, and deployment tooling are host/platform concerns. Flux can learn the standardized model interface and model-call middleware without depending on the LangChain package graph.

**Assessment:** Broadest comparative reference for catalog capabilities, content blocks, structured output, and retry/middleware policy. Avoid importing its broad integration surface.

### 7. LangGraph — durable graph and host-state donor

**Evidence status:** LangGraph is a low-level orchestration runtime for stateful, long-running workflows; its persistence and streaming features are shipped. [LangGraph overview](https://docs.langchain.com/oss/python/langgraph/overview) and [repository](https://github.com/langchain-ai/langgraph) (accessed 2026-09-24).

**Capability patterns:**

- The framework explicitly separates deterministic steps from LLM-driven steps and focuses on durable execution, streaming, human-in-the-loop, and persistence rather than prompts or model abstraction. [LangGraph overview](https://docs.langchain.com/oss/python/langgraph/overview) (accessed 2026-09-24).
- Durable execution uses a checkpointer, thread identifier, and three durability modes: `exit`, `async`, and `sync`. The docs emphasize deterministic, idempotent tasks around non-deterministic side effects. [Durable execution](https://docs.langchain.com/oss/python/langgraph/durable-execution) (accessed 2026-09-24).
- Streaming exposes updates, messages, and custom projections, with a newer typed event-streaming API. This supports the idea that Flux's normalized model stream should be composable with host graph events without becoming a graph runtime. [LangChain streaming](https://docs.langchain.com/oss/python/langchain/streaming) and [LangGraph streaming](https://docs.langchain.com/oss/python/langgraph/streaming) (accessed 2026-09-24).

**Boundary:** Checkpointers, thread stores, reducers, interrupt/resume commands, graph compilers, and durable execution belong in the host or workflow engine. Flux's `graph` package should remain contracts/projections only.

**Assessment:** Important negative boundary donor: it demonstrates exactly how much stateful machinery a host may need around a stateless model call.

### 8. Google Genkit — Go action/flow/plugin donor

**Evidence status:** Genkit flows, actions, plugins, typed schemas, streaming, and developer tooling are shipped in multiple languages. The Agents API is explicitly beta and may make breaking changes in minor releases. [Genkit Go flows](https://genkit.dev/docs/go/flows) and [Genkit agents](https://genkit.dev/docs/go/agents/overview) (accessed 2026-09-24).

**Capability patterns:**

- A flow is a typed function with schema-validated input/output, partial streaming, trace visibility, and HTTP deployment adapters. This is a clean separation between model calls and host workflows. [Defining AI workflows in Go](https://genkit.dev/docs/go/flows) (accessed 2026-09-24).
- Actions, models, tools, prompts, retrievers, and evaluators are registry-like extension points. Go code can define tools, structured data, interrupts, and streaming flows without a UI dependency. [Genkit repository](https://github.com/genkit-ai/genkit) and [Genkit Go documentation](https://genkit.dev/docs/go/get-started) (accessed 2026-09-24).
- The developer CLI/UI can run flows, evaluate them, inspect traces, and expose model/tool/prompt components. This is a useful host-tooling boundary, not a reason for Flux to ship a Dev UI. [Genkit developer tools](https://genkit.dev/docs/dart/devtools/) (accessed 2026-09-24).
- Tool interrupts and restartable tools model approval/resume as explicit protocol states. Flux can normalize an `interrupt`/approval event for host consumption without implementing approval policy. [Genkit JS API reference](https://js.api.genkit.dev/) (accessed 2026-09-24).

**Boundary:** Genkit flows, agents, Dev UI, evaluation UI, and deployment adapters are host concerns. The portable donor is the action/flow separation and typed streaming discipline; the beta Agents API should not drive Flux API commitments.

**Assessment:** One of the best Go-language donors because it treats flows and model actions as separate contracts while still supporting rich streaming.

### 9. Google ADK — event semantics and evaluation donor

**Evidence status:** ADK is an open-source code-first agent toolkit in Python, TypeScript, Go, Java, and Kotlin. The docs identify graph-based workflows and other agent features as current, while some streaming/tutorial pages label future support. [ADK agents](https://google.github.io/adk-docs/agents) and [repository](https://github.com/google/adk-python) (accessed 2026-09-24).

**Capability patterns:**

- ADK's event stream distinguishes partial text, complete text, turn completion, interruption, tool calls, state deltas, and final-response filtering. These are better than treating every provider chunk as an untyped delta. [ADK events](https://google.github.io/adk-docs/events) (accessed 2026-09-24).
- Streaming is documented as unbuffered and event-driven, with explicit handling for audio, transcription, partial flags, and interrupted turns. Flux should preserve partial/complete semantics in its canonical stream even when it does not implement realtime audio. [ADK streaming guide](https://google.github.io/adk-docs/streaming/dev-guide/part3) (accessed 2026-09-24).
- Workflow agents include sequential, parallel, and loop primitives; plugins and callbacks add logging, monitoring, and side effects without changing the core agent. [ADK agents](https://google.github.io/adk-docs/agents) and [callbacks](https://google.github.io/adk-docs/callbacks) (accessed 2026-09-24).
- ADK exposes sessions, state, memory, artifacts, callbacks, evaluation, and deployment integrations. Those are excellent examples of host-owned surfaces around a model runtime. [ADK technical overview](https://google.github.io/adk-docs/get-started/about) (accessed 2026-09-24).

**Boundary:** Runner, agent classes, workflow agents, sessions, memory, artifacts service, web interface, and deployment belong to the host/platform. Flux may borrow event flags and callback points, not the runtime.

**Assessment:** Strong donor for event conformance and multimodal/artifact references. The framework itself is too host-oriented for Flux.

### 10. Microsoft Agent Framework, AutoGen, and Semantic Kernel — convergence and middleware donor

**Evidence status:** Microsoft Agent Framework is the direct successor to AutoGen and Semantic Kernel. AutoGen's current README says it is in maintenance mode and directs new users to Agent Framework; Semantic Kernel remains an active repository, while its agent/process capabilities have historically included preview surfaces. [Microsoft overview](https://learn.microsoft.com/en-us/agent-framework/overview), [AutoGen README](https://github.com/microsoft/autogen/blob/main/README.md), and [Semantic Kernel repository](https://github.com/microsoft/semantic-kernel) (accessed 2026-09-24).

**Capability patterns:**

- Agent Framework combines AutoGen-style single/multi-agent abstractions with Semantic Kernel-style session state, type safety, filters, telemetry, and graph-based workflows. [Microsoft overview](https://learn.microsoft.com/en-us/agent-framework/overview) (accessed 2026-09-24).
- Middleware is layered into agent-run, function-calling, and chat-client boundaries. Execution order is documented, and each layer can inspect/modify input, output, and control flow. Flux should expose only the chat/model-call layer, while hosts own agent and function middleware. [Agent middleware](https://learn.microsoft.com/en-us/agent-framework/agents/middleware) (accessed 2026-09-24).
- Workflows are directed graphs of executors and edges, support streaming and non-streaming execution, and document Go workflow packages in addition to Python/.NET. This reinforces the value of a portable graph vocabulary without a Flux-owned executor. [Workflow builder and execution](https://learn.microsoft.com/en-us/agent-framework/workflows/workflows) (accessed 2026-09-24).
- Workflow observability emits build/run/executor spans, can disable sensitive data, and exposes fine-grained telemetry options. [Workflow observability](https://learn.microsoft.com/en-us/agent-framework/workflows/observability) (accessed 2026-09-24).
- The migration guide explicitly maps AutoGen's event-driven team model to Agent Framework's typed graph workflow and calls out differences in distributed execution, sessions, and tool iteration. Those differences are useful boundary evidence, not a reason to merge the frameworks. [AutoGen migration guide](https://learn.microsoft.com/en-us/agent-framework/migration-guide/from-autogen) (accessed 2026-09-24).

**Boundary:** Agent sessions, graph execution, checkpoints, context providers, orchestration patterns, and hosting are host concerns. Flux should not implement AutoGen/MAF agents or Semantic Kernel process runtime.

**Assessment:** Use Agent Framework as the current Microsoft reference; use AutoGen and Semantic Kernel for historical patterns and migration lessons. AutoGen itself should not be a new Flux dependency.

### 11. Haystack — component validation, cancellation, and tracing donor

**Evidence status:** Haystack 3.1 documentation and source describe directed multigraph pipelines, async streaming, breakpoints, tools, and pluggable tracing. [Haystack pipelines](https://docs.haystack.deepset.ai/docs/pipelines) and [repository](https://github.com/deepset-ai/haystack) (accessed 2026-09-24).

**Capability patterns:**

- Pipelines validate component names, input/output sockets, and types at connection time; serialization allows a graph/configuration to be inspected and reproduced. This is a strong model for provider adapter manifests and conformance fixtures. [Pipelines](https://docs.haystack.deepset.ai/docs/pipelines) (accessed 2026-09-24).
- Async pipelines can stream `StreamingChunk` values, cap concurrency, and cancel/drain sibling tasks when a component fails or a consumer stops early. The docs also warn that synchronous components offloaded to threads cannot be interrupted and may finish side effects in the background. Flux should make cancellation guarantees explicit per adapter and avoid claiming universal cancellation. [Pipelines: async execution and cancellation](https://docs.haystack.deepset.ai/docs/pipelines) (accessed 2026-09-24).
- Tracing is opt-in, supports OpenTelemetry and multiple backends, and has a custom `Tracer` interface. Content tracing is disabled by default. These are directly reusable observability patterns. [Haystack tracing](https://docs.haystack.deepset.ai/docs/tracing) and [custom tracer](https://docs.haystack.deepset.ai/docs/tracing-custom-tracer) (accessed 2026-09-24).
- `PipelineTool` exposes a whole pipeline as a tool with input/output mapping and async invocation, illustrating why tool schemas and workflow execution should remain separate. [PipelineTool](https://docs.haystack.deepset.ai/docs/3.0/pipelinetool) (accessed 2026-09-24).

**Boundary:** Pipelines, loops, RAG, document stores, and agent components are host products. Flux can borrow validation, cancellation, stream chunks, and tracer interfaces only.

**Assessment:** Excellent donor for “explicit contracts, validate before run, content tracing off by default” rather than for a new Flux pipeline layer.

### 12. LlamaIndex — typed event workflows and data boundary donor

**Evidence status:** LlamaIndex Workflows are shipped as a standalone event-driven library and are automatically instrumented when supported integrations are used. [Workflows](https://docs.llamaindex.ai/en/stable/module_guides/workflow) and [repository](https://github.com/run-llama/llama_index) (accessed 2026-09-24).

**Capability patterns:**

- A workflow step receives a typed event and returns a typed event; branches are ordinary conditions, loops return events, and concurrency can be expressed with event lists. The framework validates the event graph before running. [Workflow introduction](https://docs.llamaindex.ai/en/stable/module_guides/workflow) (accessed 2026-09-24).
- `WorkflowHandler.stream_events()` exposes events while the final result remains awaitable, and `Context` separates per-run state from resources such as clients, indexes, and models. This is a useful distinction for Flux's stateless generation facade. [Workflow introduction](https://docs.llamaindex.ai/en/stable/module_guides/workflow) (accessed 2026-09-24).
- Workflow documentation includes branches/loops, error handling/retry steps, human-in-the-loop, durable workflows, testing, observability, and server deployment. These are host orchestration features, not provider-runtime features. [LlamaAgents workflow documentation](https://developers.llamaindex.ai/python/llamaagents/workflows/) (accessed 2026-09-24).
- The agent model supports FunctionAgent, ReAct, and CodeAct variants, while the broader framework is explicitly data/RAG-centric. This reinforces that agent loop and retrieval belong outside Flux. [Building an agent](https://developers.llamaindex.ai/python/framework/understanding/agent/) and [LlamaIndex overview](https://docs.llamaindex.ai/) (accessed 2026-09-24).

**Boundary:** Workflow execution, checkpoints, RAG indexes, document loaders, agents, and deployment server are host concerns. Flux should expose normalized generation events and catalog facts, not the data plane.

**Assessment:** Strong donor for typed event envelopes, validation, and state/resource separation; not a provider-runtime candidate.

### 13. BAML — schema-first structured output donor

**Evidence status:** BAML's core repository, generated clients, streaming structured data, and editor/test tooling are documented. Performance and compatibility claims are vendor claims unless independently reproduced. [BAML documentation](https://docs.boundaryml.com/home) and [repository](https://github.com/BoundaryML/baml) (accessed 2026-09-24).

**Capability patterns:**

- BAML makes prompts and output schemas first-class typed artifacts, with generated language bindings and streaming typed values. [BAML home](https://docs.boundaryml.com/home) (accessed 2026-09-24).
- The project exposes Go and other language clients, plus OpenAPI integration, which suggests a schema descriptor can be more portable than a language-specific response wrapper. [OpenAPI and multi-language announcement](https://boundaryml.com/blog/announcing-openapi-support) and [Go package](https://pkg.go.dev/github.com/boundaryml/baml-go) (accessed 2026-09-24).
- The editor/playground, linting, preview, and test concepts are valuable for Flux conformance fixtures and schema regression tests, even though the editor itself is not an engine responsibility. [BAML home](https://docs.boundaryml.com/home) (accessed 2026-09-24).

**Boundary:** BAML's DSL, prompt files, editor, optimizer, and evaluation application are host/tooling concerns. Flux should expose a schema descriptor and validator/repair interface, not a second prompt language.

**Assessment:** Best focused donor for structured output correctness and schema evolution. It complements rather than replaces the provider/runtime abstraction.

### 14. Mastra — TypeScript workflow, observability, and evaluation donor

**Evidence status:** Mastra's core framework and built-in agent/workflow/observability/eval surfaces are shipped; the repository uses a dual-license model with `ee/` code under the Mastra Enterprise License. [Mastra repository licensing](https://github.com/mastra-ai/mastra/blob/main/README.md) and [Mastra framework](https://mastra.ai/ai-agent-framework) (accessed 2026-09-24).

**Capability patterns:**

- Mastra separates open-ended agents from explicit workflows, with typed steps, branching, parallel execution, loops, suspension/resumption, and storage-backed state. [Mastra agents](https://mastra.ai/docs/agents/overview.md) and [Mastra workflows](https://mastra.ai/docs/workflows/overview) (accessed 2026-09-24).
- Workflow steps can call agents or deterministic tools, and structured output schemas validate handoffs. This is a useful host-side pattern for consuming Flux's normalized responses. [Agents and tools in workflows](https://mastra.ai/docs/workflows/agents-and-tools) (accessed 2026-09-24).
- Built-in scorers, datasets, experiments, traces, metrics, and logs make evaluation a first-class product concern rather than an implicit model-runtime feature. [Mastra framework](https://mastra.ai/ai-agent-framework) (accessed 2026-09-24).
- The repository advertises a unified model router; exact provider counts differ between README and product pages, so only the existence of a router—not a specific count—should be treated as a donor fact. [Mastra repository](https://github.com/mastra-ai/mastra) (accessed 2026-09-24).

**Boundary:** Mastra agents, workflows, Studio, memory, server, and eval product are host concerns. Flux can borrow typed router and lifecycle ideas, but should not import a TypeScript framework or enterprise runtime.

**Assessment:** Good donor for how an ecosystem can separate model calls from workflow/evaluation/UI layers. Use it as a TypeScript boundary reference, not a dependency.

### 15. Strands Agents — minimal model-driven protocol donor

**Evidence status:** Strands has Python and TypeScript SDKs, a model-driven agent loop, provider integrations, MCP, streaming, structured output, hooks, sessions, and OTel. [Strands documentation](https://strandsagents.com/docs/) and [Python repository](https://github.com/strands-agents/sdk-python) (accessed 2026-09-24).

**Capability patterns:**

- The minimal `AgentBase` protocol exposes asynchronous invoke and stream operations, making it easy to see the small surface a provider runtime needs. [Strands AgentBase API](https://strandsagents.com/docs/api/python/strands.agent.base) (accessed 2026-09-24).
- Provider support includes Bedrock, Anthropic, OpenAI, Gemini, Ollama, LiteLLM, and custom providers, with structured output and token/execution metrics. [Strands quickstart/features](https://strandsagents.com/docs/) (accessed 2026-09-24).
- Hooks/plugins cover lifecycle events around model calls and tool calls; sessions are pluggable, and OTel configuration supports tracing and metrics. [Strands repository guidance](https://github.com/strands-agents/sdk-python/blob/main/AGENTS.md) and [telemetry API](https://strandsagents.com/docs/api/python/strands.telemetry.config) (accessed 2026-09-24).
- Graph, Swarm, Workflow, and A2A patterns are explicit multi-agent/host concepts. [Strands multi-agent documentation](https://strandsagents.com/docs/) (accessed 2026-09-24).

**Boundary:** Agent loop, tool execution, sandboxes, sessions, multi-agent patterns, and MCP client belong to the host. Flux can borrow the minimal model protocol and hook vocabulary, but should not add Strands' agent runtime.

**Assessment:** Useful counterweight to large frameworks: a small model-first protocol plus explicit hooks can be enough for a provider engine.

### 16. DSPy — evaluation, optimization, and program-artifact donor

**Evidence status:** DSPy's signatures/modules, adapters, metrics, evaluation, and optimizers are shipped; the project has active 2026 releases and a beta line. [DSPy documentation](https://dspy.ai/), [releases](https://github.com/stanfordnlp/dspy/releases) (accessed 2026-09-24).

**Capability patterns:**

- DSPy programs are expressed as typed signatures and composable modules rather than hand-managed prompts. This is a useful conceptual model for stable test cases and prompt-independent provider conformance. [DSPy overview](https://dspy.ai/) (accessed 2026-09-24).
- Optimizers such as GEPA, MIPRO, SIMBA, and bootstrap approaches optimize instructions, demonstrations, or weights against explicit metrics and datasets. Flux should expose evaluation inputs/outputs and trace data, not own optimization. [DSPy optimizers](https://dspy.ai/3.0.2/learn/optimization/optimizers/) (accessed 2026-09-24).
- Release notes describe active work on typed provider-neutral model systems, tool-aware ReAct, MCP compatibility, and interpreter isolation. These are roadmap/current-release signals, not reasons to make Flux a DSPy host. [DSPy 3.3.1 release](https://github.com/stanfordnlp/dspy/releases) (accessed 2026-09-24).
- A March 2026 issue documents a LiteLLM supply-chain incident and the resulting dependency pin/remediation. This is a concrete reason to keep dependency and lockfile hygiene visible in Flux's conformance/release process, even though it is not a DSPy architecture feature. [DSPy issue 9500](https://github.com/stanfordnlp/dspy/issues/9500) (accessed 2026-09-24).

**Boundary:** Prompt/program optimization, fine-tuning, datasets, and optimizer CLIs are host/tooling concerns. Flux can provide normalized events, usage, and evaluation hooks.

**Assessment:** Essential donor for evaluation and versioned prompt/program artifacts, but explicitly not a runtime framework to embed.

### 17. Agno — SDK/runtime/control-plane separation donor

**Evidence status:** Agno's SDK, AgentOS runtime, storage, control plane, tracing, approvals, scheduling, and UI surfaces are documented. [Agno framework](https://www.agno.com/agent-framework), [repository](https://github.com/agno-agi/agno) (accessed 2026-09-24).

**Capability patterns:**

- The project explicitly separates building agents with the SDK from running them with AgentOS and managing them with a control plane. This is a useful warning against allowing a model SDK to absorb all three layers. [Agno repository](https://github.com/agno-agi/agno) (accessed 2026-09-24).
- Storage owns sessions, memory, knowledge, and traces; the runtime exposes SSE/WebSocket interfaces, approvals, scheduling, and human review. These are host/platform concerns, but their event and usage vocabulary can inform Flux contracts. [Agno framework](https://www.agno.com/agent-framework) (accessed 2026-09-24).
- OTel tracing, audit logs, JWT RBAC, and multi-user isolation are explicitly called out. Flux should borrow privacy-safe trace hooks and safe status projections, not implement an admin control plane. [Agno repository](https://github.com/agno-agi/agno) (accessed 2026-09-24).
- Performance and customer numbers on the product site are vendor claims and were not used as evidence for Flux priorities. [Agno about page](https://www.agno.com/about) (accessed 2026-09-24).

**Boundary:** AgentOS, memory/knowledge, scheduling, approvals, RBAC, UI, and control plane are host concerns. Flux remains the transport/catalog/resilience layer.

**Assessment:** Valuable organizational-boundary donor, less valuable as a direct provider-contract donor.

### 18. CrewAI — event-driven host workflow donor

**Evidence status:** CrewAI's Crews and Flows are shipped in the MIT-licensed repository. Flows provide event-driven state, branching, routing, and config-as-code; Crews provide higher-level autonomous multi-agent behavior. [CrewAI repository](https://github.com/crewAIInc/crewAI) and [Flows](https://docs.crewai.com/concepts/flows) (accessed 2026-09-24).

**Capability patterns:**

- Flows use start/listen/router decorators, persistent state, logical branching (`or_`/`and_`), and asynchronous composition. This is a clear example of host-owned orchestration around ordinary model/tool calls. [Flows](https://docs.crewai.com/concepts/flows) (accessed 2026-09-24).
- Crews and tasks are configurable in YAML/JSON and support streaming, tools, knowledge, and planning. The configuration model is useful for host prompt/version management, not Flux provider selection. [Crews](https://docs.crewai.com/concepts/crews) (accessed 2026-09-24).
- The framework's high-level agent abstraction makes it a poor dependency for a provider-neutral engine; its value is in showing what should remain above the engine boundary. [CrewAI repository](https://github.com/crewAIInc/crewAI) (accessed 2026-09-24).

**Boundary:** Crews, tasks, roles, planning, memory, and flow execution belong to the host. Flux should not expose Crew/Task concepts.

**Assessment:** Useful secondary donor for declarative host workflows, but not a direct Flux runtime candidate.

### 19. Dify — product and prompt-management comparator

**Evidence status:** Dify is an actively maintained application platform for visual agentic workflows, RAG, model/tool integrations, APIs, and self-hosted/cloud deployment. [Dify introduction](https://docs.dify.ai/introduction) and [repository](https://github.com/langgenius/dify) (accessed 2026-09-24).

**Capability patterns:**

- Dify's product surface combines visual workflow design, agent construction, prompt engineering, model management, RAG, monitoring, and published APIs. This is useful for understanding the application layer a host must own around a model engine. [Dify introduction](https://docs.dify.ai/introduction) (accessed 2026-09-24).
- Its documentation and product comparisons emphasize prompt crafting/versioning/testing, provider routing, cost tracking, and deployment modes. These are product/LLMOps requirements, not provider-neutral wire contracts. [Open WebUI comparison page](https://docs.openwebui.com/alternatives/dify) and [Dify repository](https://github.com/langgenius/dify) (accessed 2026-09-24).
- The Dify license is a modified Apache-2.0-based license with conditions around multi-tenant operation and frontend branding; it is not a clean permissive dependency for a reusable engine. [Dify license](https://github.com/langgenius/dify/blob/main/LICENSE) and [Dify license policy](https://docs.dify.ai/policies/open-source) (accessed 2026-09-24).

**Boundary:** UI, visual workflow canvas, prompt IDE, RAG, application publishing, multi-tenant gateway, and commercial licensing stay outside Flux. Flux can expose catalog/preflight/usage primitives that a Dify-like host consumes.

**Assessment:** Include to make the host/product boundary concrete, not to copy code or dependencies.

### 20. Open WebUI — UI, extension, and tool-ownership comparator

**Evidence status:** Open WebUI is an actively maintained, self-hosted AI interface with OpenAI-compatible/Ollama connections, RAG, voice/video, plugins, tools, RBAC, artifacts, and analytics. [Open WebUI documentation](https://docs.openwebui.com/) and [repository](https://github.com/open-webui/open-webui) (accessed 2026-09-24).

**Capability patterns:**

- Open WebUI supports multiple model/API connections, local inference, multi-model conversations, RAG, voice/video, artifact storage, usage analytics, and broad extension points. These are strong examples of host-owned UX and data-plane scope. [Open WebUI repository](https://github.com/open-webui/open-webui) (accessed 2026-09-24).
- Pipelines is a UI-agnostic OpenAI-compatible plugin framework for offloading custom logic. Its documentation warns that pipelines execute arbitrary code and must be trusted. [Pipelines](https://github.com/open-webui/pipelines) and [Pipelines documentation](https://docs.openwebui.com/features/extensibility/pipelines) (accessed 2026-09-24).
- The Pipe documentation gives a crucial tool-loop ownership rule: if the pipeline owns and executes a tool internally, it must not emit `delta.tool_calls`, or Open WebUI will execute the same tool again; if Open WebUI should execute it, the pipeline emits tool calls and terminates with `finish_reason: "tool_calls"`. [Pipe tool-call semantics](https://docs.openwebui.com/features/extensibility/pipelines/pipes) (accessed 2026-09-24).
- The current Open WebUI license adds branding restrictions for larger deployments; older code has different license history. This makes it a product comparator rather than a neutral library dependency. [Open WebUI license](https://docs.openwebui.com/license) and [license file](https://github.com/open-webui/open-webui/blob/main/LICENSE) (accessed 2026-09-24).

**Boundary:** UI, auth, RBAC, RAG, tool execution, artifacts, model conversations, and user-facing analytics are host concerns. Flux should provide explicit tool ownership semantics and safe extension points, not a chat UI.

**Assessment:** Important negative donor: it demonstrates how easily a provider runtime turns into an application/control plane, and how dangerous ambiguous tool-loop ownership is.

### Cross-project synthesis

#### Provider and normalized content patterns

The most portable pattern is a two-layer contract: canonical parts for portable host logic and opaque provider blocks/metadata for lossless replay. LangChain and Vercel expose provider-native blocks; Pydantic uses typed request/response parts; OpenAI Agents and Google ADK expose explicit raw/run-item/partial event categories; Flux already has `ProviderBlock` but should ensure it survives every normalization path. [LangChain messages](https://docs.langchain.com/oss/python/langchain/messages), [Pydantic tools](https://pydantic.dev/docs/ai/tools-toolsets/tools/), [OpenAI streaming](https://openai.github.io/openai-agents-python/streaming), [ADK events](https://google.github.io/adk-docs/events), [Flux DTOs](https://github.com/GrayCodeAI/flux/blob/main/llm/types.go) (accessed 2026-09-24).

#### Stream and tool-loop patterns

All serious projects distinguish text deltas, tool-call deltas, complete tool calls, usage, errors, and terminal state. OpenAI adds the non-obvious rule that a stream is not complete until post-processing ends; Vercel exposes start/delta/end and input lifecycle hooks; ADK exposes partial/turn-complete/interrupted flags; Haystack defines cancellation and terminal stream behavior. Flux should make these distinctions normative in its event contract. [OpenAI streaming](https://openai.github.io/openai-agents-python/streaming), [Vercel tools](https://ai-sdk.dev/docs/ai-sdk-core/tools-and-tool-calling), [ADK events](https://google.github.io/adk-docs/events), [Haystack pipelines](https://docs.haystack.deepset.ai/docs/pipelines) (accessed 2026-09-24).

#### Metadata and capability patterns

Provider abstractions converge on capability declarations rather than a single “supports tools” boolean. LangChain's model profile, any-llm's capability matrix/optional interfaces, Vercel's provider registry, LiteLLM's model pricing/cost data, and Flux's `Model` fields each address a different part of routing. The durable pattern is a typed profile with provenance, freshness, and an explicit `unknown` state. [LangChain model profiles](https://docs.langchain.com/oss/python/langchain/models), [any-llm provider matrix](https://github.com/mozilla-ai/any-llm-go/blob/main/docs/providers.md), [Vercel provider management](https://ai-sdk.dev/docs/ai-sdk-core/provider-management), [LiteLLM models/pricing](https://docs.litellm.ai/docs/completion/input#model-list), [Flux model contract](https://github.com/GrayCodeAI/flux/blob/main/llm/provider.go) (accessed 2026-09-24).

#### Reliability and middleware patterns

Transport retries belong near the provider call, while tool retries and model fallbacks need host policy because tool side effects may not be idempotent. LangChain separates model and tool retry middleware; Microsoft separates chat, function, and agent middleware; Vercel wraps generate and stream independently; LiteLLM distinguishes within-group retries from cross-group fallback. Flux should keep the transport decision precise and expose metadata rather than silently retrying the whole turn. [LangChain middleware](https://docs.langchain.com/oss/python/langchain/middleware/built-in), [Microsoft middleware](https://learn.microsoft.com/en-us/agent-framework/agents/middleware), [Vercel middleware](https://ai-sdk.dev/docs/ai-sdk-core/middleware), [LiteLLM architecture](https://docs.litellm.ai/docs/proxy/architecture) (accessed 2026-09-24).

#### Evaluation, prompt/version, and observability patterns

DSPy, Pydantic Evals, Haystack tracing, OpenAI tracing, Pydantic instrumentation, LiteLLM OTel, and Vercel telemetry all separate runtime behavior from measurement. The shared lesson is that Flux should emit stable events, usage, route, and error metadata; hosts should own datasets, prompt registries, optimizer loops, and dashboards. [DSPy optimizers](https://dspy.ai/3.0.2/learn/optimization/optimizers/), [Pydantic overview](https://pydantic.dev/docs/ai/overview/), [Haystack tracing](https://docs.haystack.deepset.ai/docs/tracing), [OpenAI tracing](https://openai.github.io/openai-agents-python/tracing), [Vercel telemetry](https://ai-sdk.dev/docs/ai-sdk-core/telemetry) (accessed 2026-09-24).

#### UI, deployment, and governance patterns

Dify, Open WebUI, Agno, and Mastra show that UI, Studio, admin, multi-tenancy, storage, scheduling, and deployment are separable products. Their licenses also demonstrate why a provider engine should not copy a product's source-available terms or branding constraints. [Dify license](https://github.com/langgenius/dify/blob/main/LICENSE), [Open WebUI license](https://github.com/open-webui/open-webui/blob/main/LICENSE), [Mastra licensing](https://github.com/mastra-ai/mastra/blob/main/README.md), [Agno repository](https://github.com/agno-agi/agno) (accessed 2026-09-24).

## Flux recommendations and boundary clarifications

### Takeaway

Flux's next capability work should be a **contract-hardening and conformance roadmap**, not an agent-framework roadmap. The highest-value additions are explicit stream phases and terminal invariants, lossless preservation of raw/provider state, typed capability and schema descriptors, provider-neutral structured-output validation, privacy-safe OTel, and a cross-provider conformance suite. Agent loops, tools, sessions, prompts, evaluations, RAG, UI, and durable workflows should remain host contracts.

### Cited Findings

#### P0: make the existing host contract lossless and testable

Flux already defines `ContentPart`, `ProviderBlock`, `ToolCall.RawArguments`, `ToolCall.ProviderMetadata`, typed usage, typed errors, and a pull-based `EventStreamer`. [Flux DTOs](https://github.com/GrayCodeAI/flux/blob/main/llm/types.go) and [Flux tools](https://github.com/GrayCodeAI/flux/blob/main/tools/tool.go) (accessed 2026-09-24). The local normalization path currently copies only tool ID, name, and decoded arguments when mapping a core tool event, while the DTO also carries raw arguments and provider metadata; the continuation path appends assistant text and a generic “Continue.” message. [Flux stream normalization](https://github.com/GrayCodeAI/flux/blob/main/engine/stream.go) and [Flux continuation](https://github.com/GrayCodeAI/flux/blob/main/engine/continuation.go) (accessed 2026-09-24). These are concrete reasons to add conformance tests before adding new framework-like features.

**Recommended contract additions (additive where possible):**

1. Add a stream event envelope with `Sequence`, `CallID`, `TurnID`/`SegmentID`, `Phase` (`start`, `delta`, `complete`, `error`, `cancelled`), `Index` for parallel/tool blocks, and an optional `ProviderBlock`/provider metadata field. Preserve `ToolCall.RawArguments` and `ProviderMetadata` through `engine.normalizeEvent`.
2. Define a terminal invariant: exactly one terminal outcome per call/continuation segment (`done`, `error`, or `cancelled`), with usage and route attached consistently. A stream must not appear complete merely because the provider stopped sending text; this follows the OpenAI completion lesson and is more robust than current implicit channel closure. [OpenAI streaming](https://openai.github.io/openai-agents-python/streaming) (accessed 2026-09-24).
3. Preserve provider-native reasoning/signed blocks and raw tool arguments on continuation requests. Make continuation segments individually addressable and aggregate usage across segments; do not let continuation silently drop provider state. [Flux provider blocks](https://github.com/GrayCodeAI/flux/blob/main/llm/types.go) and [OpenAI usage](https://openai.github.io/openai-agents-python/usage) (accessed 2026-09-24).
4. Add explicit `ToolInputStart`, `ToolInputDelta`, `ToolInputComplete`, `ToolCallComplete`, `ToolError`, and optional `ToolApprovalRequest`/`ToolApprovalResponse` event types, while keeping tool execution in the host. [Vercel tool lifecycle](https://ai-sdk.dev/docs/ai-sdk-core/tools-and-tool-calling) and [OpenAI streaming](https://openai.github.io/openai-agents-python/streaming) (accessed 2026-09-24).
5. Make cancellation observable and testable: `Close` remains idempotent, context cancellation maps to a typed cancellation error, provider HTTP bodies are replayable for transport retries, and tests assert no goroutine/channel leak. Flux's current stream has an explicit close path and cancellation-aware forwarding. [Flux stream](https://github.com/GrayCodeAI/flux/blob/main/engine/stream.go) and [Flux retry](https://github.com/GrayCodeAI/flux/blob/main/provider/core/retry.go) (accessed 2026-09-24).

#### P0: add a provider conformance suite, not an agent evaluation suite

Build a black-box suite run against every adapter using local `httptest` servers and golden fixtures. It should test the **Flux contract**, not an agent loop. At minimum:

| Conformance layer | Required cases |
|---|---|
| Request translation | text, system prompt, image URL/data URI, audio, tools, tool choice, limits, metadata, provider options, and secret redaction. |
| Blocking response | content, reasoning, finish reason, request ID, provider blocks, warnings, usage semantics, route/deployment, and structured output. |
| Streaming | text start/delta/end, reasoning, parallel tool calls, partial JSON arguments, malformed arguments, usage-before/after-finish, done/error/cancel, unknown provider events, and continuation segments. |
| Errors | auth, permission, rate limit, timeout, context exceeded, content filter, invalid request, model not found, 5xx, Retry-After, and cancellation; verify typed code plus retryability without parsing prose. |
| Replay | append normalized assistant message, replay signed/provider blocks, send tool result on the next host turn, and verify no state loss. |
| Media | URL passthrough, explicit conversion, MIME/size limits, unsupported modality, and no accidental secret/URL leakage in traces. |
| Structured output | native schema mode, function-call fallback only when explicitly enabled, JSON mode, validation failure, repair policy, raw output retention, and provider adjustment warning. |
| Reliability | retry budget, Retry-After, fallback scope, idempotency boundary, backoff jitter, and no retry for auth/client errors. |
| Compatibility | unknown event types are ignored safely; additive fields round-trip; provider-specific options require a capability declaration. |

Use a capability matrix so unsupported cases are explicit `skip` results rather than false passes. Add property/fuzz tests for SSE framing, JSON argument fragments, unknown blocks, and oversized events. Haystack's connection validation, LlamaIndex's event-graph validation, Pydantic's offline `TestModel`, and Vercel's provider specification are useful precedents. [Haystack pipelines](https://docs.haystack.deepset.ai/docs/pipelines), [LlamaIndex workflows](https://docs.llamaindex.ai/en/stable/module_guides/workflow), [Pydantic overview](https://pydantic.dev/docs/ai/overview/), [Vercel model specification](https://github.com/vercel/ai/blob/main/packages/provider/src/language-model/v4/language-model-v4.ts) (accessed 2026-09-24).

#### P0/P1: strengthen normalized content, tool, and schema types

Flux's current `ContentPart` is a string-typed union with text, image URL, and base64 audio; `FluxTool.Parameters` is an untyped map; and `ResponseFormat` has only a type and schema string. [Flux content and tool types](https://github.com/GrayCodeAI/flux/blob/main/llm/types.go) (accessed 2026-09-24). Add an additive, versioned model such as:

```text
ContentPart = text | image | audio | video | file | reasoning | tool_call | tool_result | provider_block
```

Each part should carry a stable type discriminator, MIME/format where relevant, source (`url`, `data`, `provider_reference`), and optional opaque provider data. Keep existing `Images` and `Content` fields for compatibility during the transition. This follows the normalized-plus-native pattern in LangChain, Vercel, Pydantic, and ADK. [LangChain messages](https://docs.langchain.com/oss/python/langchain/messages), [Pydantic tools](https://pydantic.dev/docs/ai/tools-toolsets/tools/), [ADK events](https://google.github.io/adk-docs/events) (accessed 2026-09-24).

Add to tool definitions, without executing them:

- `OutputSchema`/structured result descriptor.
- `Strict` and schema dialect/version.
- `ReadOnly`/`SideEffectClass` as advisory metadata for host policy, not an authorization decision.
- Stable tool namespace/version and compatibility metadata.

Add to structured output:

- schema name/version and JSON Schema dialect.
- `Strict` capability requirement.
- output mode (`native_schema`, `tool_call`, `json_mode`, `parse_only`) and the actual mode used.
- raw provider output plus validation error/repair count.
- explicit unsupported/capability-mismatch behavior.

Flux currently maps any nonempty `OutputSchema` to a provider `json_schema` option in `engine/convert.go`; capability negotiation and a warning/error path are safer than assuming every provider accepts the same mode. [Flux conversion](https://github.com/GrayCodeAI/flux/blob/main/engine/convert.go) (accessed 2026-09-24). LangChain's distinction among `json_schema`, `function_calling`, and `json_mode` is a useful donor. [LangChain structured output](https://docs.langchain.com/oss/python/langchain/models) (accessed 2026-09-24).

#### P1: add a narrow model-call middleware layer

Expose a Flux-owned, provider-neutral middleware contract around one model call, not an agent loop. The minimum hooks are:

- `BeforeRequest`/`TransformParams`.
- `WrapGenerate`.
- `WrapStream`.
- `AfterResponse`/`AfterStream`.
- typed context carrying cancellation, correlation IDs, and redacted telemetry fields.

Define ordering, short-circuit behavior, and stream-backpressure guarantees. The middleware must be able to:

- add/adjust normalized messages and provider options.
- apply caching or deterministic transforms.
- attach a guardrail or policy hook that can return a typed error.
- observe tool requests without executing them.
- redact secrets and sensitive content before telemetry.

This follows Vercel's `transformParams`/`wrapGenerate`/`wrapStream` and LangChain's model-call wrapping, while intentionally excluding Microsoft-style agent/function middleware and host tool execution. [Vercel middleware](https://ai-sdk.dev/docs/ai-sdk-core/middleware), [LangChain custom middleware](https://docs.langchain.com/oss/python/langchain/middleware/custom), [Microsoft middleware](https://learn.microsoft.com/en-us/agent-framework/agents/middleware) (accessed 2026-09-24).

#### P1: make model metadata capability-aware and provenance-bearing

Flux already separates `Owner`, `ProviderID`, `GatewayID`, `CanonicalID`, `Source`, and `LiveMetadata`, and has a compiled catalog plus live/public discovery. [Flux model contract](https://github.com/GrayCodeAI/flux/blob/main/llm/provider.go) and [Flux catalog](https://github.com/GrayCodeAI/flux/blob/main/engine/catalog.go) (accessed 2026-09-24). Evolve this into a typed profile with:

- capability values `supported`, `unsupported`, `unknown`, or `provider_defined`.
- input/output modalities, tool/structured-output/reasoning modes, context/output limits, and cache support.
- provider, model, deployment, and gateway provenance.
- pricing source/version/as-of timestamp and confidence.
- observed/live/compiled source and last-refresh time.
- provider option schema/version and passthrough support.
- actual served model/deployment versus requested alias.

Borrow the profile idea from LangChain/models.dev, optional capability interfaces from any-llm, provider registries from Vercel, and pricing/spend separation from LiteLLM. Do not infer a capability merely because a provider accepts an OpenAI-shaped request. [LangChain model profiles](https://docs.langchain.com/oss/python/langchain/models), [any-llm provider matrix](https://github.com/mozilla-ai/any-llm-go/blob/main/docs/providers.md), [Vercel provider management](https://ai-sdk.dev/docs/ai-sdk-core/provider-management), [LiteLLM models/pricing](https://docs.litellm.ai/docs/completion/input#model-list) (accessed 2026-09-24).

#### P1: separate usage, cost, and budget policy

Keep `FluxUsage` as the normalized token fact, but add fields or a companion result for:

- usage source: provider-reported, estimated, partial, or unavailable.
- input, output, reasoning, audio, cache-read, cache-creation, and uncached token detail.
- actual provider model/deployment and request ID.
- cost amount, currency, pricing version/as-of time, and whether cost is estimated.
- cache status and prompt-cache accounting.
- continuation segment and total-call usage.

Aggregate usage across Flux-owned continuation calls, but do not aggregate host-owned agent turns. Never fabricate zero for an omitted provider field: distinguish omitted, unknown, and reported zero as the OpenAI Agents SDK does. [OpenAI usage](https://openai.github.io/openai-agents-python/usage) (accessed 2026-09-24). Emit usage as OTel GenAI attributes with content capture off by default, following LiteLLM and Vercel. [LiteLLM OTel v2](https://docs.litellm.ai/docs/observability/opentelemetry_v2), [Vercel telemetry](https://ai-sdk.dev/docs/ai-sdk-core/telemetry) (accessed 2026-09-24).

#### P1: harden caching as an explicit engine policy

Flux already offers response and semantic caching. The roadmap should make cache behavior inspectable:

- canonical cache key includes normalized messages/parts, system prompt, tool definitions, schema, generation options, selected route/model/deployment, and provider-specific options that affect output.
- credentials, raw secret-bearing headers, and tenant secrets never enter keys or logs.
- cache result emits `hit`, `miss`, `bypass`, `stale`, or `error` metadata.
- streaming cache behavior is explicit; do not claim a cache hit before a complete response is available unless replay is supported.
- semantic similarity threshold and policy remain host-configurable; exact deterministic caching is safer as an engine default.
- prompt-cache tokens are accounted separately from application response-cache hits.

LiteLLM's documentation specifically demonstrates provider-parameter cache-key and route-bypass concerns. [LiteLLM caching](https://docs.litellm.ai/docs/proxy/caching) (accessed 2026-09-24).

#### P1: define safe provider-specific passthrough

Provide two deliberately separate paths:

1. **Normalized options:** allowlisted, versioned, provider-namespaced options that adapters validate and translate.
2. **Raw native passthrough:** an explicit opt-in escape hatch for callers who need a provider-native request/response, with a separate interface and clear warning that it is not portable.

Never put untyped vendor SDK objects into the stable `llm`/`engine` contract. Preserve only opaque provider state needed for replay (`ProviderBlock`, thought signatures, provider metadata) unless a host explicitly requests raw passthrough. This is the safe middle ground between Vercel's provider options, any-llm's provider data, and Open WebUI's OpenAI-compatible plugin boundary. [Vercel provider options](https://ai-sdk.dev/docs/foundations/provider-options), [any-llm provider data](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers), [Open WebUI pipelines](https://docs.openwebui.com/features/extensibility/pipelines) (accessed 2026-09-24).

#### P1: expand multimodal support without making Flux a media product

Keep chat input parts provider-neutral and add file/document/video/audio references with MIME, URI/data source, and size limits. Keep separate, optional media operations for image generation and transcription if Flux already exposes them; do not add a gallery, asset manager, or browser UI. LiteLLM's explicit URL-to-base64 compatibility rule is a useful donor, but URL fetching must be size-limited, opt-in where appropriate, and never silently turn into an SSRF primitive. [LiteLLM image URL handling](https://docs.litellm.ai/docs/proxy/architecture), [Flux media facade](https://github.com/GrayCodeAI/flux/blob/main/engine/media.go) (accessed 2026-09-24).

#### P2: expose optional engine capabilities, not a new framework

If the roadmap later adds embeddings, reranking, batch, moderation, files, or provider-native responses, expose each as a small optional interface with capability negotiation and conformance tests. Any-llm-go's optional `EmbeddingProvider`, `ModelLister`, and `ErrorConverter` are a good Go pattern. [any-llm-go providers](https://pkg.go.dev/github.com/mozilla-ai/any-llm-go/providers) (accessed 2026-09-24). Do not add an agent, graph executor, session store, or UI merely because a framework bundles those features.

### Minimal host-contract primitives Flux should expose

These are the smallest additions that let a host build Rho-like semantics without importing Flux internals:

1. **Generation request/response DTOs:** typed parts, tool schema, tool call/result, provider blocks, schema output, limits, route, warnings, usage, and error metadata.
2. **Event envelope:** call/turn/segment IDs, monotonic sequence, phase, block index, terminal status, raw/provider state, and cancellation state.
3. **Capability profile:** model/deployment/provider capabilities, limits, provenance, freshness, and output modes.
4. **Tool request contract:** schema, call ID, raw arguments, provider metadata, and result/error association; no executor.
5. **Structured-output contract:** schema descriptor, validation result, raw output, repair count, and provider mode.
6. **Extension contract:** versioned model-call middleware and provider adapter registration; no raw SDK types.
7. **Observability contract:** OTel-compatible spans, usage/cost fields, redaction policy, and injectable tracer/sink.
8. **Readiness contract:** preflight, catalog health, credential status, route preview, and live availability without leaking secrets.
9. **Graph vocabulary:** node/edge/event/provenance/idempotency fields only; no scheduler/checkpointer.
10. **Tool versioning vocabulary:** closed namespaces/behavior versions already present in `tools`; no tool policy engine.

These primitives preserve the existing Flux ownership split: Flux owns credentials, catalog, route, transport, resilience, normalized usage, and provider telemetry; the host owns conversation history, permissions, tool execution, sessions, checkpoints, product memory, and user-visible behavior. [Flux boundary](https://github.com/GrayCodeAI/flux/blob/main/docs/architecture/HOST-ENGINE-BOUNDARY.md) and [Flux host port](https://github.com/GrayCodeAI/flux/blob/main/llm/provider.go) (accessed 2026-09-24).

### Explicit capabilities Flux should reject

| Reject from Flux | Reason / host owner |
|---|---|
| Agent classes, planners, handoffs, subagents, autonomous loops | OpenAI Agents, LangGraph, Pydantic AI, Google ADK, CrewAI, and Agno all place this above the model call. |
| Tool execution, permissions, approval policy, sandbox, MCP client | Open WebUI's tool-ownership rule and Vercel/OpenAI approval flows show that execution and policy are host/product semantics. |
| Conversation/session store, memory, checkpoints, replay runtime | LangGraph, Pydantic durable integrations, Letta, Agno, and ADK own durable state and recovery. |
| Graph compiler, scheduler, queue, distributed executor | LangGraph, Haystack, LlamaIndex, Microsoft Agent Framework, and ADK are workflow engines. Flux's `graph` package is explicitly data contracts only. |
| Prompt registry, prompt optimizer, fine-tuning, evaluation datasets | DSPy, Pydantic Evals, Dify, and BAML's editor/test tooling own this lifecycle. |
| RAG/vector/document ingestion | LlamaIndex, Haystack, Dify, and Open WebUI are data/application planes. |
| React/Svelte/Vue hooks, chat UI, generative UI, Studio, admin console | Vercel UI, Dify, Open WebUI, Genkit Dev UI, and Agno AgentOS are host products. |
| Virtual-key team billing/admin/multi-tenant control plane | LiteLLM Proxy, Dify, Agno, and Open WebUI provide platform services beyond a provider engine. |
| Untyped provider SDK leakage | Keep vendor types inside adapters; expose only normalized DTOs, opaque replay blocks, or explicit raw passthrough. |
| Automatic semantic caching or automatic tool-loop execution | Both change product semantics and can duplicate side effects; require explicit host policy. |

### Recommended sequencing

**P0: correctness before breadth**

- Freeze a contract-v2 compatibility test matrix.
- Add stream phases, sequence/call/segment IDs, terminal invariants, typed stream errors, and raw/provider-state preservation.
- Add structured-output capability negotiation and validator/repair result types.
- Add cross-provider golden fixtures and fuzz tests for SSE/JSON/tool arguments.
- Make cancellation and retry scopes explicit in adapter documentation and tests.

**P1: capability depth without framework creep**

- Add typed multimodal parts and optional media capabilities.
- Add capability/provenance/freshness metadata to the catalog.
- Add narrow model-call middleware with streaming-safe ordering and redaction.
- Add usage provenance, cost fields, cache status, and OTel GenAI span hierarchy.
- Add versioned provider-options/passthrough interfaces.

**P2: ecosystem integrations**

- Add optional embeddings, reranking, batch, moderation, and file interfaces only when a host need is demonstrated.
- Add protocol bridges outside the core contract where useful, such as an OpenAI-compatible proxy or host-owned MCP adapter.
- Publish conformance reports and compatibility badges per provider/model route.

**No-go:** no Flux `Agent`, `Runner`, `GraphRuntime`, `SessionStore`, `Memory`, `ToolExecutor`, `PromptStore`, `EvalRunner`, `AdminUI`, or chat frontend in the stable host packages. The existing boundary and contract version should be the guardrail. [Flux boundary](https://github.com/GrayCodeAI/flux/blob/main/docs/architecture/HOST-ENGINE-BOUNDARY.md) (accessed 2026-09-24).

### Inferences

- The most important “capability donor” is not a particular framework; it is the union of small, explicit contracts: model profile, typed parts, event envelope, schema descriptor, usage provenance, and adapter manifest.
- Flux's existing `ProviderBlock`, `RawArguments`, route/attempt metadata, pull stream, and graph-as-data decisions are unusually aligned with the strongest OSS patterns and should be preserved rather than replaced.
- A conformance suite will expose more value to hosts than adding another provider-specific convenience API: hosts can trust the same semantics across 28+ gateways and future adapters.
- Middleware, tracing, caching, and structured output can be engine capabilities only if their policy knobs remain explicit and host-owned where they affect side effects, privacy, or user experience.
- The best long-term competitive position for Flux is “the narrow, auditable provider engine with excellent conformance and lossless normalization,” not “the biggest agent framework.”

### Gaps

- This research did not verify every provider's current wire-level behavior against live credentials. The conformance suite should turn the recommendations into executable facts.
- Exact model capability and pricing data are time-sensitive; `Source`, `LiveMetadata`, freshness, and provenance should be treated as first-class fields rather than static assumptions.
- The local Flux checkout may contain unreleased contract-v2 changes. Before implementation, reconcile these recommendations with the current branch's tests and any sibling Rho pin.
- A final legal review is required before copying code, schemas, generated bindings, or documentation from Apache/MIT projects; Dify, Open WebUI, LiteLLM enterprise, and Mastra enterprise have additional terms.
- Benchmark claims, download counts, star counts, and vendor “production-ready” labels should not be used as roadmap priorities without independent measurements.
