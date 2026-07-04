from __future__ import annotations

import threading
from collections.abc import AsyncIterator

import pytest

from voce.core import PluginTester
from voce.plugins.openai_llm_stream.plugin import OpenAIStreamConfig, OpenAIStreamPlugin
from voce.schema import PAYLOAD_LLM_CHUNK, Payload


class FakeOpenAIStreamPlugin(OpenAIStreamPlugin):
    def __init__(self, config: OpenAIStreamConfig, chunks: list[str]) -> None:
        super().__init__(config)
        self.chunks = chunks
        self.seen_messages = []

    async def _stream_chat(
        self,
        messages: list[dict],
        stop_event: threading.Event,
    ) -> AsyncIterator[str]:
        self.seen_messages = messages
        for chunk in self.chunks:
            yield chunk


@pytest.mark.asyncio
async def test_openai_stream_plugin_splits_streaming_tokens() -> None:
    plugin = FakeOpenAIStreamPlugin(
        OpenAIStreamConfig(prompt="Be brief.", min_sentence_len=8),
        ["Hello", " world.", " Next", " part"],
    )
    received: list[Payload] = []
    tester = PluginTester(plugin).on_payload(lambda _port, payload: received.append(payload))

    await tester.start()
    await tester.inject_payload(Payload("asr_result", {"text": "hi", "is_final": True}))
    await tester.wait(0.1)

    assert [payload.name for payload in received] == [PAYLOAD_LLM_CHUNK, PAYLOAD_LLM_CHUNK]
    assert received[0].properties == {"sentence": "Hello world.", "is_final": False}
    assert received[1].properties == {"sentence": "Next part", "is_final": True}
    assert plugin.seen_messages[0] == {"role": "system", "content": "Be brief."}
    assert plugin.seen_messages[-1] == {"role": "user", "content": "hi"}

    await tester.stop()


@pytest.mark.asyncio
async def test_openai_stream_plugin_ignores_partial_payload() -> None:
    plugin = FakeOpenAIStreamPlugin(OpenAIStreamConfig(), ["ignored"])
    received: list[Payload] = []
    tester = PluginTester(plugin).on_payload(lambda _port, payload: received.append(payload))

    await tester.start()
    await tester.inject_payload(Payload("asr_result", {"text": "hi", "is_final": False}))
    await tester.wait(0.1)

    assert received == []
    assert plugin.seen_messages == []

    await tester.stop()
