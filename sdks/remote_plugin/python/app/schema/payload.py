from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from app.schema.base import Properties, decode_json_bytes, properties_from_model

PAYLOAD_ASR_RESULT = "asr_result"
PAYLOAD_CAPTION = "caption"
PAYLOAD_LLM_CHUNK = "llm_chunk"


@dataclass(slots=True)
class Payload(Properties):
    name: str
    properties: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_json_bytes(cls, name: str, data: bytes) -> Payload:
        return cls(name=name, properties=decode_json_bytes(data))

    @classmethod
    def from_model(cls, name: str, value: Any) -> Payload:
        return cls(name=name, properties=properties_from_model(value))
