from pydantic import BaseModel

from app.core import AsyncPlugin, PluginMetadata, PluginRegistry


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
