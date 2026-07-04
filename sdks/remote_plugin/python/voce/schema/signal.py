from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from voce.schema.base import Properties, decode_json_bytes, properties_from_model

SIGNAL_INTERRUPTER = "interrupter"
SIGNAL_AGENT_SPEECH_START = "agent_speech_start"
SIGNAL_AGENT_SPEECH_END = "agent_speech_end"
SIGNAL_USER_SPEECH_START = "user_speech_start"
SIGNAL_USER_SPEECH_END = "user_speech_end"
SIGNAL_VAD_USER_SPEECH_START = "vad_user_speech_start"
SIGNAL_VAD_USER_SPEECH_END = "vad_user_speech_end"


@dataclass(slots=True)
class Signal(Properties):
    name: str
    properties: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json_bytes(cls, name: str, data: bytes) -> Signal:
        return cls(name=name, properties=decode_json_bytes(data))

    @classmethod
    def from_model(cls, name: str, value: Any) -> Signal:
        return cls(name=name, properties=properties_from_model(value))
