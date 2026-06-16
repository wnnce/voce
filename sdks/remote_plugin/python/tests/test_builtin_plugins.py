from __future__ import annotations

from app.core import PluginRegistry
from app.plugins import register_plugins
from app.plugins.passthrough import PassthroughPlugin
from app.schema import Payload


class CapturingFlow:
    def __init__(self) -> None:
        self.payloads: list[Payload] = []

    async def send_payload(self, payload: Payload, *, port: int = 0) -> None:
        self.payloads.append(payload)


def test_register_builtin_passthrough_plugin() -> None:
    registry = PluginRegistry()

    register_plugins(registry)

    plugins = registry.list_metadata()
    assert len(plugins) == 1
    assert plugins[0].name == "passthrough"


async def test_passthrough_plugin_forwards_payload() -> None:
    plugin = PassthroughPlugin(config=registry_config())
    flow = CapturingFlow()
    payload = Payload("demo", {"text": "hello"})

    await plugin.on_payload(flow, payload)

    assert flow.payloads == [payload]


def registry_config():
    registry = PluginRegistry()
    register_plugins(registry)
    return registry.create("passthrough", b"{}").config
