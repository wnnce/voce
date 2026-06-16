from __future__ import annotations

from abc import ABC, abstractmethod

from app.schema import Payload, Signal


class Flow(ABC):
    """Interface used by plugins to emit payloads and signals."""

    @abstractmethod
    async def send_payload(
        self,
        payload: Payload,
        *,
        port: int = 0,
    ) -> None:
        pass

    @abstractmethod
    async def send_signal(
        self,
        signal: Signal,
        *,
        port: int = 0,
    ) -> None:
        pass
