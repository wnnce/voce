from __future__ import annotations

from dataclasses import dataclass

from pydantic import BaseModel, ValidationError

from app.schema import Payload, Signal
from app.schema.payload import PAYLOAD_ASR_RESULT
from app.schema.signal import SIGNAL_INTERRUPTER


@dataclass(slots=True)
class ASRResult:
    text: str
    is_final: bool


class Chunk(BaseModel):
    sentence: str
    is_final: bool


def test_payload_get_get_as_and_bind_dataclass() -> None:
    payload = Payload(
        PAYLOAD_ASR_RESULT,
        {
            "text": "hello",
            "is_final": "true",
        },
    )

    assert payload.get("text") == "hello"
    assert payload.get("missing", "fallback") == "fallback"
    assert payload.get_as("is_final", bool) is True
    assert payload.bind(ASRResult) == ASRResult(text="hello", is_final=True)


def test_payload_bind_pydantic_model_and_json_roundtrip() -> None:
    payload = Payload.from_model(
        "llm_chunk",
        Chunk(sentence="hello", is_final=False),
    )
    restored = Payload.from_json_bytes(payload.name, payload.to_json_bytes())

    assert restored.to_dict() == {
        "name": "llm_chunk",
        "properties": {
            "sentence": "hello",
            "is_final": False,
        },
    }
    assert restored.bind(Chunk) == Chunk(sentence="hello", is_final=False)


def test_signal_from_model_and_get_as_default() -> None:
    signal = Signal.from_model(SIGNAL_INTERRUPTER, {"level": "3", "enabled": "bad"})

    assert signal.get_as("level", int) == 3
    assert signal.get_as("missing", str, "fallback") == "fallback"
    assert signal.get_as("enabled", bool, False) is False


def test_get_as_raises_validation_error_without_default() -> None:
    payload = Payload("demo", {"count": "not-an-int"})

    try:
        payload.get_as("count", int)
    except ValidationError:
        pass
    else:
        raise AssertionError("expected pydantic ValidationError")
