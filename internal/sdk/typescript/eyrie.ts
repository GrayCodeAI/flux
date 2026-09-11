interface ToolDef {
  name: string;
  description?: string;
  input_schema?: unknown;
}

interface PromptOptions {
  model?: string;
  system_prompt?: string;
  max_tokens?: number;
  tools?: ToolDef[];
  signal?: AbortSignal;
}

interface PromptResponse {
  content: string;
  node_id: string;
}

interface StreamEvent {
  type: string;
  data: unknown;
}

interface AliasResult {
  alias: string;
  node_id: string;
}

interface Node {
  id: string;
  parent_id?: string;
  root_id?: string;
  sequence: number;
  node_type: string;
  content: string;
  model?: string;
  title?: string;
  created_at: string;
}

class APIError extends Error {
  statusCode: number;
  body: string;

  constructor(statusCode: number, body: string) {
    super(`eyrie: ${statusCode} ${body}`);
    this.statusCode = statusCode;
    this.body = body;
  }
}

class EyrieClient {
  private baseURL: string;
  private headers: Record<string, string>;

  constructor(baseURL: string, apiKey: string = "") {
    this.baseURL = baseURL.replace(/\/+$/, "");
    this.headers = {
      "Content-Type": "application/json",
    };
    if (apiKey) {
      this.headers["Authorization"] = `Bearer ${apiKey}`;
    }
  }

  async prompt(message: string, options?: PromptOptions): Promise<PromptResponse> {
    const body: Record<string, unknown> = { message };
    if (options?.model) body.model = options.model;
    if (options?.system_prompt) body.system_prompt = options.system_prompt;
    if (options?.max_tokens) body.max_tokens = options.max_tokens;
    if (options?.tools) body.tools = options.tools;
    const res = await fetch(`${this.baseURL}/prompt`, {
      method: "POST",
      headers: this.headers,
      body: JSON.stringify(body),
      signal: options?.signal,
    });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async *streamPrompt(message: string, options?: PromptOptions): AsyncGenerator<StreamEvent> {
    const body: Record<string, unknown> = { message, stream: true };
    if (options?.model) body.model = options.model;
    if (options?.system_prompt) body.system_prompt = options.system_prompt;
    if (options?.max_tokens) body.max_tokens = options.max_tokens;
    if (options?.tools) body.tools = options.tools;
    const res = await fetch(`${this.baseURL}/prompt`, {
      method: "POST",
      headers: this.headers,
      body: JSON.stringify(body),
      signal: options?.signal,
    });
    if (!res.ok) throw new APIError(res.status, await res.text());
    const reader = res.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        if (line.startsWith("data: ")) {
          const data = line.slice(6);
          const evt: StreamEvent = JSON.parse(data);
          yield evt;
          if (evt.type === "done" || evt.type === "error") return;
        }
      }
    }
  }

  async promptFrom(nodeId: string, message: string, options?: PromptOptions): Promise<PromptResponse> {
    const body: Record<string, unknown> = { message };
    if (options?.model) body.model = options.model;
    if (options?.system_prompt) body.system_prompt = options.system_prompt;
    if (options?.max_tokens) body.max_tokens = options.max_tokens;
    if (options?.tools) body.tools = options.tools;
    const res = await fetch(`${this.baseURL}/nodes/${nodeId}/prompt`, {
      method: "POST",
      headers: this.headers,
      body: JSON.stringify(body),
      signal: options?.signal,
    });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async listConversations(): Promise<Node[]> {
    const res = await fetch(`${this.baseURL}/nodes`, { headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async getNode(nodeId: string): Promise<Node> {
    const res = await fetch(`${this.baseURL}/nodes/${nodeId}`, { headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async getTree(nodeId: string): Promise<Node[]> {
    const res = await fetch(`${this.baseURL}/nodes/${nodeId}/tree`, { headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async deleteNode(nodeId: string): Promise<void> {
    const res = await fetch(`${this.baseURL}/nodes/${nodeId}`, { method: "DELETE", headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
  }

  async createAlias(nodeId: string, alias: string): Promise<AliasResult> {
    const res = await fetch(`${this.baseURL}/nodes/${nodeId}/aliases/${alias}`, {
      method: "PUT",
      headers: this.headers,
    });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async deleteAlias(alias: string): Promise<AliasResult> {
    const res = await fetch(`${this.baseURL}/aliases/${alias}`, { method: "DELETE", headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }

  async health(): Promise<{ status: string }> {
    const res = await fetch(`${this.baseURL}/health`, { headers: this.headers });
    if (!res.ok) throw new APIError(res.status, await res.text());
    return res.json();
  }
}
