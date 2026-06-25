from __future__ import annotations

import json
from typing import Any

from voce.core import (
    Field,
    MultiTrackConfig,
    PluginMetadata,
    PortMetadata,
    Property,
    TrackConfig,
)
from voce.proto import plugin_pb2 as pb

EVENT_TYPES = {
    "signal": "EVENT_TYPE_SIGNAL",
    "payload": "EVENT_TYPE_PAYLOAD",
    "audio": "EVENT_TYPE_AUDIO",
    "video": "EVENT_TYPE_VIDEO",
}

VALUE_TYPES = {
    "string": "VALUE_TYPE_STRING",
    "number": "VALUE_TYPE_NUMBER",
    "float": "VALUE_TYPE_NUMBER",
    "integer": "VALUE_TYPE_INTEGER",
    "int": "VALUE_TYPE_INTEGER",
    "boolean": "VALUE_TYPE_BOOLEAN",
    "bool": "VALUE_TYPE_BOOLEAN",
    "object": "VALUE_TYPE_OBJECT",
    "array": "VALUE_TYPE_ARRAY",
}

DROP_STRATEGIES = {
    "block_if_full": "DROP_STRATEGY_BLOCK_IF_FULL",
    "block": "DROP_STRATEGY_BLOCK_IF_FULL",
    "drop_newest": "DROP_STRATEGY_DROP_NEWEST",
    "drop_oldest": "DROP_STRATEGY_DROP_OLDEST",
}


def plugin_metadata_to_proto(metadata: PluginMetadata) -> pb.PluginMetadata:
    result = pb.PluginMetadata(
        name=metadata.name,
        description=metadata.description,
        schema=json_string(metadata.schema or {}),
        inputs=[property_to_proto(item) for item in metadata.inputs],
        outputs=[property_to_proto(item) for item in metadata.outputs],
        ports=[port_metadata_to_proto(item) for item in metadata.ports],
    )
    if metadata.multi_track is not None:
        result.multi_track.CopyFrom(multi_track_config_to_proto(metadata.multi_track))
    return result


def multi_track_config_to_proto(config: MultiTrackConfig) -> pb.MultiTrackConfig:
    result = pb.MultiTrackConfig(enabled=config.enabled)
    if config.payload is not None:
        result.payload.CopyFrom(track_config_to_proto(config.payload))
    return result


def track_config_to_proto(config: TrackConfig) -> pb.TrackConfig:
    return pb.TrackConfig(
        enabled=config.enabled,
        buffer_size=config.buffer_size,
        drop_strategy=drop_strategy_to_proto(config.drop_strategy),
        interrupt_signals=list(config.interrupt_signals),
    )


def property_to_proto(prop: Property) -> pb.Property:
    return pb.Property(
        type=event_type_to_proto(prop.type),
        name=prop.name,
        fields=[field_to_proto(item) for item in prop.fields],
    )


def field_to_proto(field: Field) -> pb.Field:
    return pb.Field(
        key=field.key,
        type=value_type_to_proto(field.type),
        required=field.required,
    )


def port_metadata_to_proto(port: PortMetadata) -> pb.PortMetadata:
    return pb.PortMetadata(
        type=event_type_to_proto(port.type),
        port=port.port,
        name=port.name,
        description=port.description,
    )


def event_type_to_proto(value: str) -> str:
    return EVENT_TYPES.get(value.lower(), "EVENT_TYPE_UNSPECIFIED")


def value_type_to_proto(value: str) -> str:
    return VALUE_TYPES.get(value.lower(), "VALUE_TYPE_UNSPECIFIED")


def drop_strategy_to_proto(value: str) -> str:
    return DROP_STRATEGIES.get(value.lower(), "DROP_STRATEGY_UNSPECIFIED")


def json_bytes(value: dict[str, Any]) -> bytes:
    return json.dumps(value, ensure_ascii=False).encode()


def json_string(value: dict[str, Any]) -> str:
    return json.dumps(value, ensure_ascii=False)


def json_from_bytes(data: bytes) -> dict[str, Any]:
    if not data:
        return {}
    value = json.loads(data.decode())
    if not isinstance(value, dict):
        raise ValueError("properties must decode to a JSON object")
    return value
