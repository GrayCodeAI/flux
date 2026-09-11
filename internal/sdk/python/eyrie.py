"""Eyrie Conversation DAG HTTP API client."""

from __future__ import annotations

import json
from typing import Any, AsyncIterator, Optional
from urllib.parse import urljoin

import httpx


class EyrieClient:
    """Synchronous client for the Eyrie conversation DAG HTTP API."""

    def __init__(self, base_url: str, api_key: str = "", timeout: float = 120.0) -> None:
        self.base_url = base_url.rstrip("/")
        headers = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self.client = httpx.Client(base_url=base_url, headers=headers, timeout=timeout)

    def prompt(self, message: str, *, model: str = "", system_prompt: str = "",
               max_tokens: int = 0, tools: list[dict] | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {"message": message}
        if model:
            body["model"] = model
        if system_prompt:
            body["system_prompt"] = system_prompt
        if max_tokens:
            body["max_tokens"] = max_tokens
        if tools:
            body["tools"] = tools
        resp = self.client.post("/prompt", json=body)
        resp.raise_for_status()
        return resp.json()

    def prompt_from(self, node_id: str, message: str, *, model: str = "",
                    system_prompt: str = "", max_tokens: int = 0,
                    tools: list[dict] | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {"message": message}
        if model:
            body["model"] = model
        if system_prompt:
            body["system_prompt"] = system_prompt
        if max_tokens:
            body["max_tokens"] = max_tokens
        if tools:
            body["tools"] = tools
        resp = self.client.post(f"/nodes/{node_id}/prompt", json=body)
        resp.raise_for_status()
        return resp.json()

    def list_conversations(self) -> list[dict[str, Any]]:
        resp = self.client.get("/nodes")
        resp.raise_for_status()
        return resp.json()

    def get_node(self, node_id: str) -> dict[str, Any]:
        resp = self.client.get(f"/nodes/{node_id}")
        resp.raise_for_status()
        return resp.json()

    def get_tree(self, node_id: str) -> list[dict[str, Any]]:
        resp = self.client.get(f"/nodes/{node_id}/tree")
        resp.raise_for_status()
        return resp.json()

    def delete_node(self, node_id: str) -> dict[str, str]:
        resp = self.client.delete(f"/nodes/{node_id}")
        resp.raise_for_status()
        return resp.json()

    def create_alias(self, node_id: str, alias: str) -> dict[str, str]:
        resp = self.client.put(f"/nodes/{node_id}/aliases/{alias}")
        resp.raise_for_status()
        return resp.json()

    def delete_alias(self, alias: str) -> dict[str, str]:
        resp = self.client.delete(f"/aliases/{alias}")
        resp.raise_for_status()
        return resp.json()

    def health(self) -> dict[str, str]:
        resp = self.client.get("/health")
        resp.raise_for_status()
        return resp.json()

    def close(self) -> None:
        self.client.close()


class AsyncEyrieClient:
    """Asynchronous client for the Eyrie conversation DAG HTTP API."""

    def __init__(self, base_url: str, api_key: str = "", timeout: float = 120.0) -> None:
        self.base_url = base_url.rstrip("/")
        headers = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"
        self.client = httpx.AsyncClient(base_url=base_url, headers=headers, timeout=timeout)

    async def prompt(self, message: str, *, model: str = "", system_prompt: str = "",
                     max_tokens: int = 0, tools: list[dict] | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {"message": message}
        if model:
            body["model"] = model
        if system_prompt:
            body["system_prompt"] = system_prompt
        if max_tokens:
            body["max_tokens"] = max_tokens
        if tools:
            body["tools"] = tools
        resp = await self.client.post("/prompt", json=body)
        resp.raise_for_status()
        return resp.json()

    async def stream_prompt(self, message: str, *, model: str = "", system_prompt: str = "",
                            max_tokens: int = 0) -> AsyncIterator[dict[str, Any]]:
        body: dict[str, Any] = {"message": message, "stream": True}
        if model:
            body["model"] = model
        if system_prompt:
            body["system_prompt"] = system_prompt
        if max_tokens:
            body["max_tokens"] = max_tokens
        async with self.client.stream("POST", "/prompt", json=body) as resp:
            resp.raise_for_status()
            async for line in resp.aiter_lines():
                if line.startswith("data: "):
                    data = line[len("data: "):]
                    evt = json.loads(data)
                    yield evt
                    if evt.get("type") in ("done", "error"):
                        break

    async def prompt_from(self, node_id: str, message: str, *, model: str = "",
                          system_prompt: str = "", max_tokens: int = 0,
                          tools: list[dict] | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {"message": message}
        if model:
            body["model"] = model
        if system_prompt:
            body["system_prompt"] = system_prompt
        if max_tokens:
            body["max_tokens"] = max_tokens
        if tools:
            body["tools"] = tools
        resp = await self.client.post(f"/nodes/{node_id}/prompt", json=body)
        resp.raise_for_status()
        return resp.json()

    async def list_conversations(self) -> list[dict[str, Any]]:
        resp = await self.client.get("/nodes")
        resp.raise_for_status()
        return resp.json()

    async def get_node(self, node_id: str) -> dict[str, Any]:
        resp = await self.client.get(f"/nodes/{node_id}")
        resp.raise_for_status()
        return resp.json()

    async def get_tree(self, node_id: str) -> list[dict[str, Any]]:
        resp = await self.client.get(f"/nodes/{node_id}/tree")
        resp.raise_for_status()
        return resp.json()

    async def delete_node(self, node_id: str) -> dict[str, str]:
        resp = await self.client.delete(f"/nodes/{node_id}")
        resp.raise_for_status()
        return resp.json()

    async def create_alias(self, node_id: str, alias: str) -> dict[str, str]:
        resp = await self.client.put(f"/nodes/{node_id}/aliases/{alias}")
        resp.raise_for_status()
        return resp.json()

    async def delete_alias(self, alias: str) -> dict[str, str]:
        resp = await self.client.delete(f"/aliases/{alias}")
        resp.raise_for_status()
        return resp.json()

    async def health(self) -> dict[str, str]:
        resp = await self.client.get("/health")
        resp.raise_for_status()
        return resp.json()

    async def close(self) -> None:
        await self.client.aclose()
