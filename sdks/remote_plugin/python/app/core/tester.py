from __future__ import annotations

import asyncio
from collections.abc import Callable
from typing import Any

from app.core.flow import Flow
from app.core.plugin import AsyncPlugin
from app.schema.payload import Payload
from app.schema.signal import Signal

ACTIVITY_BUFFER_SIZE = 1024


class MockFlow(Flow):
    """MockFlow implements the Flow interface hooks for testing purposes."""

    def __init__(self, ping_activity: Callable[[], None] | None = None) -> None:
        self._ping_activity = ping_activity
        self.on_signal_hook: Callable[[int, Signal], None] | None = None
        self.on_payload_hook: Callable[[int, Payload], None] | None = None

    def ping(self) -> None:
        if self._ping_activity:
            self._ping_activity()

    async def send_payload(
        self,
        payload: Payload,
        *,
        port: int = 0,
    ) -> None:
        self.ping()
        if self.on_payload_hook:
            self.on_payload_hook(port, payload)

    async def send_signal(
        self,
        signal: Signal,
        *,
        port: int = 0,
    ) -> None:
        self.ping()
        if self.on_signal_hook:
            self.on_signal_hook(port, signal)


class PluginTester:
    """PluginTester provides a harness for testing individual plugins in isolation.

    It uses a MockFlow to capture outputs and track plugin activity.
    """

    def __init__(self, plugin: AsyncPlugin[Any]) -> None:
        self.plugin = plugin
        self.mock = MockFlow(self._ping_activity)
        self._activity: asyncio.Queue[None] = asyncio.Queue(maxsize=ACTIVITY_BUFFER_SIZE)

    def _ping_activity(self) -> None:
        try:
            self._activity.put_nowait(None)
        except asyncio.QueueFull:
            pass

    async def start(self) -> PluginTester:
        await self.plugin.on_start(self.mock)
        await self.plugin.on_ready(self.mock)
        return self

    async def stop(self) -> None:
        await self.plugin.on_stop()

    async def wait(self, timeout_sec: float = 0.1) -> None:
        """Wait blocks until the plugin stops emitting activity for the specified duration.

        This is useful for testing asynchronous plugins that process data in the background.
        """
        while True:
            try:
                await asyncio.wait_for(self._activity.get(), timeout=timeout_sec)
                self._activity.task_done()
            except TimeoutError:
                # No activity for `timeout` seconds, we consider it done.
                return

    async def inject_signal(self, signal: Signal) -> PluginTester:
        self._ping_activity()
        await self.plugin.on_signal(self.mock, signal)
        return self

    async def inject_payload(self, payload: Payload) -> PluginTester:
        self._ping_activity()
        await self.plugin.on_payload(self.mock, payload)
        return self

    def on_signal(self, cb: Callable[[int, Signal], None]) -> PluginTester:
        self.mock.on_signal_hook = cb
        return self

    def on_payload(self, cb: Callable[[int, Payload], None]) -> PluginTester:
        self.mock.on_payload_hook = cb
        return self
