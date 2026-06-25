from __future__ import annotations

from pydantic import BaseModel, Field

from voce.core import AsyncPlugin, Flow
from voce.core.registry import plugin
from voce.schema import Payload, Signal


class PassthroughConfig(BaseModel):
    payload_prefix: str = Field(
        default="",
        title="Payload Prefix",
        description="Prefix added to forwarded payload names.",
    )
    payload_suffix: str = Field(
        default="",
        title="Payload Suffix",
        description="Suffix added to forwarded payload names.",
    )
    forward_signals: bool = Field(
        default=True,
        title="Forward Signals",
        description="Whether incoming signals should be forwarded.",
    )
    output_port: int = Field(
        default=0,
        ge=0,
        title="Output Port",
        description="Port used when emitting forwarded payloads and signals.",
    )


@plugin(
    name="passthrough",
    description="Test plugin that forwards incoming signal and payload events unchanged.",
    config_type=PassthroughConfig,
)
class PassthroughPlugin(AsyncPlugin[PassthroughConfig]):
    async def on_signal(self, flow: Flow, signal: Signal) -> None:
        if not self.config.forward_signals:
            return
        await flow.send_signal(signal, port=self.config.output_port)

    async def on_payload(self, flow: Flow, payload: Payload) -> None:
        name = f"{self.config.payload_prefix}{payload.name}{self.config.payload_suffix}"
        await flow.send_payload(
            Payload(name, payload.properties_dict()),
            port=self.config.output_port,
        )
