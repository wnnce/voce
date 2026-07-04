from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import asdict, is_dataclass
from typing import Any, TypeVar

from pydantic import BaseModel, TypeAdapter, ValidationError

T = TypeVar("T")
_MISSING = object()


class Properties:
    name: str
    properties: dict[str, Any]

    def get(self, key: str, default: Any = None) -> Any:
        return self.properties.get(key, default)

    def get_as(self, key: str, typ: type[T], default: Any = _MISSING) -> T | Any:
        if key not in self.properties:
            if default is _MISSING:
                return None
            return default
        try:
            return TypeAdapter(typ).validate_python(self.properties[key])
        except ValidationError:
            if default is not _MISSING:
                return default
            raise

    def bind(self, typ: type[T]) -> T:
        return TypeAdapter(typ).validate_python(self.properties)

    def bind_key(self, key: str, typ: type[T], default: Any = _MISSING) -> T | Any:
        return self.get_as(key, typ, default)

    def properties_dict(self) -> dict[str, Any]:
        return dict(self.properties)

    def to_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "properties": self.properties_dict(),
        }

    def to_json_bytes(self) -> bytes:
        return encode_json_bytes(self.properties)


def properties_from_model(value: Any) -> dict[str, Any]:
    if isinstance(value, BaseModel):
        return value.model_dump(mode="json")
    if is_dataclass(value) and not isinstance(value, type):
        return asdict(value)
    if isinstance(value, Mapping):
        return dict(value)
    if hasattr(value, "model_dump"):
        return value.model_dump(mode="json")
    if hasattr(value, "__dict__"):
        return dict(value.__dict__)
    raise TypeError(f"unsupported properties source: {type(value)!r}")


def encode_json_bytes(value: Mapping[str, Any]) -> bytes:
    return json.dumps(dict(value), ensure_ascii=False).encode()


def decode_json_bytes(data: bytes) -> dict[str, Any]:
    if not data:
        return {}
    value = json.loads(data.decode())
    if not isinstance(value, dict):
        raise ValueError("schema properties must decode to a JSON object")
    return value
