from pydantic import BaseModel

from voce.core import (
    AsyncPlugin,
    MultiTrackConfig,
    PluginMetadata,
    PluginRegistry,
    TrackConfig,
)
from voce.proto import plugin_pb2 as pb
from voce.service.converters import plugin_metadata_to_proto


class EchoConfig(BaseModel):
    prefix: str = ""


class EchoPlugin(AsyncPlugin[EchoConfig]):
    pass


def test_registry_create_plugin() -> None:
    registry = PluginRegistry()

    registry.register(
        PluginMetadata(name="echo", description="Echo plugin"),
        EchoPlugin,
        EchoConfig,
    )

    plugin = registry.create("echo", b'{"prefix":"hello"}')

    assert plugin.config == EchoConfig(prefix="hello")
    assert registry.list_metadata()[0].name == "echo"
    assert registry.list_metadata()[0].schema is not None


def test_plugin_metadata_multi_track_config_to_proto() -> None:
    metadata = PluginMetadata(
        name="slow_echo",
        multi_track=MultiTrackConfig(
            enabled=True,
            payload=TrackConfig(
                enabled=True,
                buffer_size=64,
                drop_strategy="drop_oldest",
                interrupt_signals=["interrupter", "barge_in"],
            ),
        ),
    )

    proto = plugin_metadata_to_proto(metadata)

    assert proto.multi_track.enabled is True
    assert proto.multi_track.payload.enabled is True
    assert proto.multi_track.payload.buffer_size == 64
    assert proto.multi_track.payload.drop_strategy == pb.DROP_STRATEGY_DROP_OLDEST
    assert list(proto.multi_track.payload.interrupt_signals) == ["interrupter", "barge_in"]
