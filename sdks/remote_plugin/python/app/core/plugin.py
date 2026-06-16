from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from pydantic import BaseModel

from app.schema import Payload, Signal

if TYPE_CHECKING:
    from app.core.flow import Flow


@dataclass(slots=True)
class Field:
    key: str
    type: str
    required: bool = False


@dataclass(slots=True)
class Property:
    type: str
    name: str = ""
    fields: list[Field] = field(default_factory=list)


@dataclass(slots=True)
class PortMetadata:
    type: str
    port: int
    name: str = ""
    description: str = ""


@dataclass(slots=True)
class PluginMetadata:
    name: str
    description: str = ""
    schema: dict[str, Any] | None = None
    inputs: list[Property] = field(default_factory=list)
    outputs: list[Property] = field(default_factory=list)
    ports: list[PortMetadata] = field(default_factory=list)
    multi_wrapper: bool = False


class AsyncPlugin[ConfigT: BaseModel]:
    """Base class for remote plugins.

    Subclasses can override lifecycle methods and implement OnSignal / OnPayload behavior.
    The first remote plugin version intentionally does not expose audio or video callbacks.
    """

    metadata: PluginMetadata
    logger: logging.Logger
    config: ConfigT

    def __init__(self, config: ConfigT) -> None:
        self.config = config

    async def on_start(self, flow: Flow) -> None:
        pass

    async def on_ready(self, flow: Flow) -> None:
        pass

    async def on_pause(self) -> None:
        pass

    async def on_resume(self, flow: Flow) -> None:
        pass

    async def on_stop(self) -> None:
        pass

    async def on_signal(
        self,
        flow: Flow,
        signal: Signal,
    ) -> None:
        pass

    async def on_payload(
        self,
        flow: Flow,
        payload: Payload,
    ) -> None:
        pass
