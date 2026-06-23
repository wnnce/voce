from __future__ import annotations

import asyncio
import json
import os
import threading
import urllib.error
import urllib.request
from collections.abc import AsyncIterator, Iterator
from typing import Any

from pydantic import BaseModel
from pydantic import Field as PydanticField

from app.core import (
    AsyncPlugin,
    Field,
    Flow,
    MultiTrackConfig,
    PluginMetadata,
    PluginRegistry,
    Property,
    TrackConfig,
)
from app.schema import PAYLOAD_LLM_CHUNK, SIGNAL_INTERRUPTER, Payload

Message = dict[str, Any]

MIN_SENTENCE_LEN = 20
SENTENCE_BOUNDARIES = set("。！？!?.，,；;：:\n\r")
FINAL_BOUNDARIES = set("。！？!?.\n\r")


class OpenAIStreamConfig(BaseModel):
    base_url: str = PydanticField(
        default="https://api.openai.com/v1",
        title="Base URL",
        description="OpenAI-compatible API base URL.",
    )
    api_key: str = PydanticField(
        default="",
        title="API Key",
        description="Bearer token. If empty, OPENAI_API_KEY is used.",
    )
    model: str = PydanticField(
        default="gpt-4o-mini",
        title="Model",
        description="Chat completions model name.",
    )
    prompt: str = PydanticField(
        default="You are a helpful assistant.",
        title="System Prompt",
        description="System prompt prepended to the in-memory conversation.",
    )
    history_limit: int = PydanticField(
        default=10,
        ge=0,
        title="History Limit",
        description="Maximum number of user/assistant turns kept in memory.",
    )
    failed_message: str = PydanticField(
        default="I apologize, I am having trouble connecting right now.",
        title="Failed Message",
        description="Final sentence emitted when the model request fails.",
    )
    request_timeout_sec: float = PydanticField(
        default=60,
        gt=0,
        title="Request Timeout Seconds",
        description="HTTP request timeout for the model stream.",
    )
    min_sentence_len: int = PydanticField(
        default=MIN_SENTENCE_LEN,
        ge=1,
        title="Minimum Sentence Length",
        description="Minimum fragment length before splitting on weak punctuation.",
    )


class OpenAIStreamPlugin(AsyncPlugin[OpenAIStreamConfig]):
    def __init__(self, config: OpenAIStreamConfig) -> None:
        super().__init__(config)
        self._history: list[Message] = []
        self._history_lock = asyncio.Lock()

    async def on_payload(self, flow: Flow, payload: Payload) -> None:
        if not payload.get_as("is_final", bool, False):
            return

        text = payload.get_as("text", str, "")
        if not text:
            return

        messages = await self._build_messages(text)
        fragment: list[str] = []
        assistant_parts: list[str] = []
        stop_event = threading.Event()

        try:
            async for token in self._stream_chat(messages, stop_event):
                assistant_parts.append(token)
                for sentence in self._parse_sentences(fragment, token):
                    await self._send_sentence(flow, sentence, is_final=False)
        except asyncio.CancelledError:
            stop_event.set()
            raise
        except Exception as exc:
            if hasattr(self, "logger"):
                self.logger.exception("openai stream request failed: %s", exc)
            if self.config.failed_message:
                await self._send_sentence(flow, self.config.failed_message, is_final=True)
            return

        final_text = "".join(fragment).strip()
        await self._send_sentence(flow, final_text, is_final=True)
        await self._append_history(text, "".join(assistant_parts))

    async def _build_messages(self, text: str) -> list[Message]:
        async with self._history_lock:
            messages = list(self._history)
        if self.config.prompt:
            messages = [{"role": "system", "content": self.config.prompt}, *messages]
        messages.append({"role": "user", "content": text})
        return messages

    async def _append_history(self, user_text: str, assistant_text: str) -> None:
        if self.config.history_limit <= 0:
            return
        async with self._history_lock:
            self._history.extend(
                [
                    {"role": "user", "content": user_text},
                    {"role": "assistant", "content": assistant_text},
                ]
            )
            max_messages = self.config.history_limit * 2
            if len(self._history) > max_messages:
                self._history = self._history[-max_messages:]

    async def _stream_chat(
        self,
        messages: list[Message],
        stop_event: threading.Event,
    ) -> AsyncIterator[str]:
        queue: asyncio.Queue[tuple[str, str | BaseException | None]] = asyncio.Queue()
        loop = asyncio.get_running_loop()

        def publish(kind: str, value: str | BaseException | None = None) -> None:
            loop.call_soon_threadsafe(queue.put_nowait, (kind, value))

        def worker() -> None:
            try:
                for token in self._stream_chat_sync(messages, stop_event):
                    publish("token", token)
                publish("done")
            except BaseException as exc:  # propagated back to the event loop
                publish("error", exc)

        worker_task = asyncio.create_task(asyncio.to_thread(worker))
        try:
            while True:
                kind, value = await queue.get()
                if kind == "token":
                    yield str(value)
                elif kind == "error":
                    raise (
                        value if isinstance(value, BaseException) else RuntimeError("stream failed")
                    )
                elif kind == "done":
                    break
        finally:
            stop_event.set()
            await worker_task

    def _stream_chat_sync(
        self,
        messages: list[Message],
        stop_event: threading.Event,
    ) -> Iterator[str]:
        body = json.dumps(
            {
                "model": self.config.model,
                "stream": True,
                "messages": messages,
            },
            ensure_ascii=False,
        ).encode()
        request = urllib.request.Request(
            self.config.base_url.rstrip("/") + "/chat/completions",
            data=body,
            method="POST",
            headers=self._request_headers(),
        )

        try:
            with urllib.request.urlopen(
                request, timeout=self.config.request_timeout_sec
            ) as response:
                for raw_line in response:
                    if stop_event.is_set():
                        return
                    token = self._parse_sse_line(raw_line)
                    if token:
                        yield token
        except urllib.error.HTTPError as exc:
            detail = exc.read(4096).decode(errors="replace")
            raise RuntimeError(
                f"request openai failed, status: {exc.code}, error: {detail}"
            ) from exc

    def _request_headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        api_key = self.config.api_key or os.getenv("OPENAI_API_KEY", "")
        if api_key:
            headers["Authorization"] = "Bearer " + api_key
        return headers

    def _parse_sse_line(self, raw_line: bytes) -> str:
        line = raw_line.decode(errors="replace").strip()
        if not line or line.startswith(":") or not line.startswith("data:"):
            return ""

        data = line.removeprefix("data:").strip()
        if data == "[DONE]":
            return ""

        try:
            chunk = json.loads(data)
        except json.JSONDecodeError:
            return ""

        choices = chunk.get("choices") or []
        if not choices:
            return ""
        delta = choices[0].get("delta") or {}
        return str(delta.get("content") or "")

    def _parse_sentences(self, fragment: list[str], content: str) -> list[str]:
        sentences: list[str] = []
        for char in content:
            fragment.append(char)
            if char in SENTENCE_BOUNDARIES:
                sentence = "".join(fragment)
                if self._should_split(sentence, char):
                    sentences.append(sentence)
                    fragment.clear()
        return sentences

    def _should_split(self, sentence: str, last_char: str) -> bool:
        trimmed = sentence.strip()
        if not trimmed or not any(char.isalnum() for char in trimmed):
            return False
        if last_char in FINAL_BOUNDARIES:
            return True
        return len(trimmed) >= self.config.min_sentence_len

    async def _send_sentence(self, flow: Flow, sentence: str, *, is_final: bool) -> None:
        await flow.send_payload(
            Payload(
                PAYLOAD_LLM_CHUNK,
                {
                    "sentence": sentence,
                    "is_final": is_final,
                },
            )
        )


def register(registry: PluginRegistry) -> None:
    registry.register(
        PluginMetadata(
            name="openai_llm_stream",
            description="Minimal OpenAI-compatible streaming LLM example.",
            inputs=[
                Property(
                    "payload",
                    fields=[
                        Field("text", "string", True),
                        Field("is_final", "boolean", True),
                    ],
                ),
            ],
            outputs=[
                Property(
                    "payload",
                    PAYLOAD_LLM_CHUNK,
                    fields=[
                        Field("sentence", "string", True),
                        Field("is_final", "boolean", True),
                    ],
                ),
            ],
            multi_track=MultiTrackConfig(
                enabled=True,
                payload=TrackConfig(
                    enabled=True,
                    buffer_size=128,
                    drop_strategy="block_if_full",
                    interrupt_signals=[SIGNAL_INTERRUPTER],
                ),
            ),
        ),
        OpenAIStreamPlugin,
        OpenAIStreamConfig,
    )
