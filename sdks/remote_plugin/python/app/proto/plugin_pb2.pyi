from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class DropStrategy(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DROP_STRATEGY_UNSPECIFIED: _ClassVar[DropStrategy]
    DROP_STRATEGY_BLOCK_IF_FULL: _ClassVar[DropStrategy]
    DROP_STRATEGY_DROP_NEWEST: _ClassVar[DropStrategy]
    DROP_STRATEGY_DROP_OLDEST: _ClassVar[DropStrategy]

class EventType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVENT_TYPE_UNSPECIFIED: _ClassVar[EventType]
    EVENT_TYPE_SIGNAL: _ClassVar[EventType]
    EVENT_TYPE_PAYLOAD: _ClassVar[EventType]
    EVENT_TYPE_AUDIO: _ClassVar[EventType]
    EVENT_TYPE_VIDEO: _ClassVar[EventType]

class ValueType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    VALUE_TYPE_UNSPECIFIED: _ClassVar[ValueType]
    VALUE_TYPE_STRING: _ClassVar[ValueType]
    VALUE_TYPE_NUMBER: _ClassVar[ValueType]
    VALUE_TYPE_INTEGER: _ClassVar[ValueType]
    VALUE_TYPE_BOOLEAN: _ClassVar[ValueType]
    VALUE_TYPE_OBJECT: _ClassVar[ValueType]
    VALUE_TYPE_ARRAY: _ClassVar[ValueType]

class RuntimeMessageType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUNTIME_MESSAGE_TYPE_UNSPECIFIED: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_LIFECYCLE: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_SIGNAL: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_PAYLOAD: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_EMIT_LOG: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_ACK: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_REPORT: _ClassVar[RuntimeMessageType]
    RUNTIME_MESSAGE_TYPE_CANCEL: _ClassVar[RuntimeMessageType]

class LifecycleType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    LIFECYCLE_TYPE_UNSPECIFIED: _ClassVar[LifecycleType]
    LIFECYCLE_TYPE_START: _ClassVar[LifecycleType]
    LIFECYCLE_TYPE_READY: _ClassVar[LifecycleType]
    LIFECYCLE_TYPE_PAUSE: _ClassVar[LifecycleType]
    LIFECYCLE_TYPE_RESUME: _ClassVar[LifecycleType]
    LIFECYCLE_TYPE_STOP: _ClassVar[LifecycleType]

class LogLevel(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    LOG_LEVEL_UNSPECIFIED: _ClassVar[LogLevel]
    LOG_LEVEL_DEBUG: _ClassVar[LogLevel]
    LOG_LEVEL_INFO: _ClassVar[LogLevel]
    LOG_LEVEL_WARN: _ClassVar[LogLevel]
    LOG_LEVEL_ERROR: _ClassVar[LogLevel]

class ReportStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    REPORT_STATUS_UNSPECIFIED: _ClassVar[ReportStatus]
    REPORT_STATUS_OK: _ClassVar[ReportStatus]
    REPORT_STATUS_ERROR: _ClassVar[ReportStatus]
    REPORT_STATUS_CANCELED: _ClassVar[ReportStatus]
DROP_STRATEGY_UNSPECIFIED: DropStrategy
DROP_STRATEGY_BLOCK_IF_FULL: DropStrategy
DROP_STRATEGY_DROP_NEWEST: DropStrategy
DROP_STRATEGY_DROP_OLDEST: DropStrategy
EVENT_TYPE_UNSPECIFIED: EventType
EVENT_TYPE_SIGNAL: EventType
EVENT_TYPE_PAYLOAD: EventType
EVENT_TYPE_AUDIO: EventType
EVENT_TYPE_VIDEO: EventType
VALUE_TYPE_UNSPECIFIED: ValueType
VALUE_TYPE_STRING: ValueType
VALUE_TYPE_NUMBER: ValueType
VALUE_TYPE_INTEGER: ValueType
VALUE_TYPE_BOOLEAN: ValueType
VALUE_TYPE_OBJECT: ValueType
VALUE_TYPE_ARRAY: ValueType
RUNTIME_MESSAGE_TYPE_UNSPECIFIED: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_LIFECYCLE: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_SIGNAL: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_PAYLOAD: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_EMIT_LOG: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_ACK: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_REPORT: RuntimeMessageType
RUNTIME_MESSAGE_TYPE_CANCEL: RuntimeMessageType
LIFECYCLE_TYPE_UNSPECIFIED: LifecycleType
LIFECYCLE_TYPE_START: LifecycleType
LIFECYCLE_TYPE_READY: LifecycleType
LIFECYCLE_TYPE_PAUSE: LifecycleType
LIFECYCLE_TYPE_RESUME: LifecycleType
LIFECYCLE_TYPE_STOP: LifecycleType
LOG_LEVEL_UNSPECIFIED: LogLevel
LOG_LEVEL_DEBUG: LogLevel
LOG_LEVEL_INFO: LogLevel
LOG_LEVEL_WARN: LogLevel
LOG_LEVEL_ERROR: LogLevel
REPORT_STATUS_UNSPECIFIED: ReportStatus
REPORT_STATUS_OK: ReportStatus
REPORT_STATUS_ERROR: ReportStatus
REPORT_STATUS_CANCELED: ReportStatus

class PingRequest(_message.Message):
    __slots__ = ("client_id",)
    CLIENT_ID_FIELD_NUMBER: _ClassVar[int]
    client_id: str
    def __init__(self, client_id: _Optional[str] = ...) -> None: ...

class PingResponse(_message.Message):
    __slots__ = ("server_id", "version")
    SERVER_ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    server_id: str
    version: str
    def __init__(self, server_id: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class ListPluginsRequest(_message.Message):
    __slots__ = ("namespace",)
    NAMESPACE_FIELD_NUMBER: _ClassVar[int]
    namespace: str
    def __init__(self, namespace: _Optional[str] = ...) -> None: ...

class ListPluginsResponse(_message.Message):
    __slots__ = ("plugins",)
    PLUGINS_FIELD_NUMBER: _ClassVar[int]
    plugins: _containers.RepeatedCompositeFieldContainer[PluginMetadata]
    def __init__(self, plugins: _Optional[_Iterable[_Union[PluginMetadata, _Mapping]]] = ...) -> None: ...

class CreateInstanceRequest(_message.Message):
    __slots__ = ("instance_id", "plugin_name", "config", "metadata")
    class MetadataEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    PLUGIN_NAME_FIELD_NUMBER: _ClassVar[int]
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    instance_id: str
    plugin_name: str
    config: bytes
    metadata: _containers.ScalarMap[str, str]
    def __init__(self, instance_id: _Optional[str] = ..., plugin_name: _Optional[str] = ..., config: _Optional[bytes] = ..., metadata: _Optional[_Mapping[str, str]] = ...) -> None: ...

class CreateInstanceResponse(_message.Message):
    __slots__ = ("instance_id",)
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    instance_id: str
    def __init__(self, instance_id: _Optional[str] = ...) -> None: ...

class DestroyInstanceRequest(_message.Message):
    __slots__ = ("instance_id", "reason")
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    instance_id: str
    reason: str
    def __init__(self, instance_id: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class DestroyInstanceResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class PluginMetadata(_message.Message):
    __slots__ = ("name", "description", "schema", "inputs", "outputs", "ports", "multi_track")
    NAME_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    SCHEMA_FIELD_NUMBER: _ClassVar[int]
    INPUTS_FIELD_NUMBER: _ClassVar[int]
    OUTPUTS_FIELD_NUMBER: _ClassVar[int]
    PORTS_FIELD_NUMBER: _ClassVar[int]
    MULTI_TRACK_FIELD_NUMBER: _ClassVar[int]
    name: str
    description: str
    schema: str
    inputs: _containers.RepeatedCompositeFieldContainer[Property]
    outputs: _containers.RepeatedCompositeFieldContainer[Property]
    ports: _containers.RepeatedCompositeFieldContainer[PortMetadata]
    multi_track: MultiTrackConfig
    def __init__(self, name: _Optional[str] = ..., description: _Optional[str] = ..., schema: _Optional[str] = ..., inputs: _Optional[_Iterable[_Union[Property, _Mapping]]] = ..., outputs: _Optional[_Iterable[_Union[Property, _Mapping]]] = ..., ports: _Optional[_Iterable[_Union[PortMetadata, _Mapping]]] = ..., multi_track: _Optional[_Union[MultiTrackConfig, _Mapping]] = ...) -> None: ...

class MultiTrackConfig(_message.Message):
    __slots__ = ("enabled", "payload")
    ENABLED_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    enabled: bool
    payload: TrackConfig
    def __init__(self, enabled: _Optional[bool] = ..., payload: _Optional[_Union[TrackConfig, _Mapping]] = ...) -> None: ...

class TrackConfig(_message.Message):
    __slots__ = ("enabled", "buffer_size", "drop_strategy", "interrupt_signals")
    ENABLED_FIELD_NUMBER: _ClassVar[int]
    BUFFER_SIZE_FIELD_NUMBER: _ClassVar[int]
    DROP_STRATEGY_FIELD_NUMBER: _ClassVar[int]
    INTERRUPT_SIGNALS_FIELD_NUMBER: _ClassVar[int]
    enabled: bool
    buffer_size: int
    drop_strategy: DropStrategy
    interrupt_signals: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, enabled: _Optional[bool] = ..., buffer_size: _Optional[int] = ..., drop_strategy: _Optional[_Union[DropStrategy, str]] = ..., interrupt_signals: _Optional[_Iterable[str]] = ...) -> None: ...

class Property(_message.Message):
    __slots__ = ("type", "name", "fields")
    TYPE_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    FIELDS_FIELD_NUMBER: _ClassVar[int]
    type: EventType
    name: str
    fields: _containers.RepeatedCompositeFieldContainer[Field]
    def __init__(self, type: _Optional[_Union[EventType, str]] = ..., name: _Optional[str] = ..., fields: _Optional[_Iterable[_Union[Field, _Mapping]]] = ...) -> None: ...

class Field(_message.Message):
    __slots__ = ("key", "type", "required")
    KEY_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    key: str
    type: ValueType
    required: bool
    def __init__(self, key: _Optional[str] = ..., type: _Optional[_Union[ValueType, str]] = ..., required: _Optional[bool] = ...) -> None: ...

class PortMetadata(_message.Message):
    __slots__ = ("type", "port", "name", "description")
    TYPE_FIELD_NUMBER: _ClassVar[int]
    PORT_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    type: EventType
    port: int
    name: str
    description: str
    def __init__(self, type: _Optional[_Union[EventType, str]] = ..., port: _Optional[int] = ..., name: _Optional[str] = ..., description: _Optional[str] = ...) -> None: ...

class RuntimeMessage(_message.Message):
    __slots__ = ("instance_id", "message_id", "correlation_id", "type", "metadata", "lifecycle", "signal", "payload", "emit_signal", "emit_payload", "emit_log", "report", "ack", "cancel")
    class MetadataEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_ID_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    LIFECYCLE_FIELD_NUMBER: _ClassVar[int]
    SIGNAL_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    EMIT_SIGNAL_FIELD_NUMBER: _ClassVar[int]
    EMIT_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    EMIT_LOG_FIELD_NUMBER: _ClassVar[int]
    REPORT_FIELD_NUMBER: _ClassVar[int]
    ACK_FIELD_NUMBER: _ClassVar[int]
    CANCEL_FIELD_NUMBER: _ClassVar[int]
    instance_id: str
    message_id: str
    correlation_id: str
    type: RuntimeMessageType
    metadata: _containers.ScalarMap[str, str]
    lifecycle: LifecycleEvent
    signal: SignalEvent
    payload: PayloadEvent
    emit_signal: EmitSignal
    emit_payload: EmitPayload
    emit_log: EmitLog
    report: EventReport
    ack: EventAck
    cancel: CancelEvent
    def __init__(self, instance_id: _Optional[str] = ..., message_id: _Optional[str] = ..., correlation_id: _Optional[str] = ..., type: _Optional[_Union[RuntimeMessageType, str]] = ..., metadata: _Optional[_Mapping[str, str]] = ..., lifecycle: _Optional[_Union[LifecycleEvent, _Mapping]] = ..., signal: _Optional[_Union[SignalEvent, _Mapping]] = ..., payload: _Optional[_Union[PayloadEvent, _Mapping]] = ..., emit_signal: _Optional[_Union[EmitSignal, _Mapping]] = ..., emit_payload: _Optional[_Union[EmitPayload, _Mapping]] = ..., emit_log: _Optional[_Union[EmitLog, _Mapping]] = ..., report: _Optional[_Union[EventReport, _Mapping]] = ..., ack: _Optional[_Union[EventAck, _Mapping]] = ..., cancel: _Optional[_Union[CancelEvent, _Mapping]] = ...) -> None: ...

class LifecycleEvent(_message.Message):
    __slots__ = ("type",)
    TYPE_FIELD_NUMBER: _ClassVar[int]
    type: LifecycleType
    def __init__(self, type: _Optional[_Union[LifecycleType, str]] = ...) -> None: ...

class SignalEvent(_message.Message):
    __slots__ = ("name", "properties")
    NAME_FIELD_NUMBER: _ClassVar[int]
    PROPERTIES_FIELD_NUMBER: _ClassVar[int]
    name: str
    properties: bytes
    def __init__(self, name: _Optional[str] = ..., properties: _Optional[bytes] = ...) -> None: ...

class PayloadEvent(_message.Message):
    __slots__ = ("name", "properties")
    NAME_FIELD_NUMBER: _ClassVar[int]
    PROPERTIES_FIELD_NUMBER: _ClassVar[int]
    name: str
    properties: bytes
    def __init__(self, name: _Optional[str] = ..., properties: _Optional[bytes] = ...) -> None: ...

class EmitSignal(_message.Message):
    __slots__ = ("signal", "port")
    SIGNAL_FIELD_NUMBER: _ClassVar[int]
    PORT_FIELD_NUMBER: _ClassVar[int]
    signal: SignalEvent
    port: int
    def __init__(self, signal: _Optional[_Union[SignalEvent, _Mapping]] = ..., port: _Optional[int] = ...) -> None: ...

class EmitPayload(_message.Message):
    __slots__ = ("payload", "port")
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    PORT_FIELD_NUMBER: _ClassVar[int]
    payload: PayloadEvent
    port: int
    def __init__(self, payload: _Optional[_Union[PayloadEvent, _Mapping]] = ..., port: _Optional[int] = ...) -> None: ...

class EmitLog(_message.Message):
    __slots__ = ("level", "message", "fields")
    class FieldsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    FIELDS_FIELD_NUMBER: _ClassVar[int]
    level: LogLevel
    message: str
    fields: _containers.ScalarMap[str, str]
    def __init__(self, level: _Optional[_Union[LogLevel, str]] = ..., message: _Optional[str] = ..., fields: _Optional[_Mapping[str, str]] = ...) -> None: ...

class EventReport(_message.Message):
    __slots__ = ("status", "error")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    status: ReportStatus
    error: RemoteError
    def __init__(self, status: _Optional[_Union[ReportStatus, str]] = ..., error: _Optional[_Union[RemoteError, _Mapping]] = ...) -> None: ...

class RemoteError(_message.Message):
    __slots__ = ("code", "message", "details")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    DETAILS_FIELD_NUMBER: _ClassVar[int]
    code: str
    message: str
    details: str
    def __init__(self, code: _Optional[str] = ..., message: _Optional[str] = ..., details: _Optional[str] = ...) -> None: ...

class EventAck(_message.Message):
    __slots__ = ("timestamp",)
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    timestamp: int
    def __init__(self, timestamp: _Optional[int] = ...) -> None: ...

class CancelEvent(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...
