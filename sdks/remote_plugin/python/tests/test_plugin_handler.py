from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator

from pydantic import BaseModel

from app.core import AsyncPlugin, Flow, PluginMetadata, PluginRegistry
from app.proto import plugin_pb2 as pb
from app.schema import Payload
from app.service.converters import json_bytes, json_string
from app.service.instances import PluginInstanceService
from app.service.plugin import RemotePluginServiceHandler


class EchoConfig(BaseModel):
    suffix: str = ".echo"


class EchoPlugin(AsyncPlugin[EchoConfig]):
    def __init__(self, config: EchoConfig) -> None:
        super().__init__(config)
        self.stop_count = 0

    async def on_payload(self, flow: Flow, payload: Payload) -> None:
        await flow.send_payload(
            Payload(f"{payload.name}{self.config.suffix}", payload.properties_dict())
        )

    async def on_stop(self) -> None:
        self.stop_count += 1


def build_handler() -> RemotePluginServiceHandler:
    registry = PluginRegistry()
    registry.register(
        PluginMetadata(
            name="echo",
            description="Echo plugin",
            schema={"type": "object"},
        ),
        EchoPlugin,
        EchoConfig,
    )
    return RemotePluginServiceHandler(
        PluginInstanceService(registry),
        server_id="test-server",
        version="0.1.0",
    )


async def test_unary_plugin_service_methods() -> None:
    handler = build_handler()

    ping = await handler.Ping(pb.PingRequest(client_id="go-runtime"), None)
    plugins = await handler.ListPlugins(pb.ListPluginsRequest(), None)
    created = await handler.CreateInstance(
        pb.CreateInstanceRequest(
            instance_id="inst_1",
            plugin_name="echo",
            config=json_bytes({"prefix": "hello"}),
        ),
        None,
    )
    plugin = handler.instances.get_instance("inst_1")
    destroyed = await handler.DestroyInstance(
        pb.DestroyInstanceRequest(instance_id="inst_1", reason="test"),
        None,
    )

    assert ping.server_id == "test-server"
    assert plugins.plugins[0].name == "echo"
    assert plugins.plugins[0].schema == json_string({"type": "object"})
    assert created.instance_id == "inst_1"
    assert isinstance(destroyed, pb.DestroyInstanceResponse)
    assert isinstance(plugin, EchoPlugin)
    assert plugin.stop_count == 0


async def test_run_instance_dispatches_payload_and_reports() -> None:
    handler = build_handler()
    await handler.CreateInstance(
        pb.CreateInstanceRequest(instance_id="inst_1", plugin_name="echo"),
        None,
    )

    async def requests() -> AsyncIterator[pb.RuntimeMessage]:
        yield pb.RuntimeMessage(
            instance_id="inst_1",
            message_id="go_1",
            type="RUNTIME_MESSAGE_TYPE_PAYLOAD",
            payload=pb.PayloadEvent(name="demo", properties=json_bytes({"text": "hello"})),
        )

    class MockContext:
        def invocation_metadata(self):
            return (("instance-id", "inst_1"),)

        async def abort(self, code, details):
            raise Exception(f"Abort {code}: {details}")

    replies = [item async for item in handler.RunInstance(requests(), MockContext())]

    assert len(replies) == 3
    assert replies[0].type == pb.RUNTIME_MESSAGE_TYPE_ACK
    assert replies[0].correlation_id == "go_1"
    assert replies[1].type == pb.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD
    assert replies[1].correlation_id == "go_1"
    assert replies[1].emit_payload.payload.name == "demo.echo"
    assert replies[1].emit_payload.payload.properties == json_bytes({"text": "hello"})
    assert replies[2].type == pb.RUNTIME_MESSAGE_TYPE_REPORT
    assert replies[2].correlation_id == "go_1"
    assert replies[2].report.status == pb.REPORT_STATUS_OK


async def test_destroy_instance_closes_running_stream() -> None:
    handler = build_handler()
    await handler.CreateInstance(
        pb.CreateInstanceRequest(instance_id="inst_1", plugin_name="echo"),
        None,
    )
    plugin = handler.instances.get_instance("inst_1")
    assert isinstance(plugin, EchoPlugin)

    requests_queue: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()

    async def requests() -> AsyncIterator[pb.RuntimeMessage]:
        while True:
            message = await requests_queue.get()
            if message is None:
                return
            yield message

    class MockContext:
        def invocation_metadata(self):
            return (("instance-id", "inst_1"),)

        async def abort(self, code, details):
            raise Exception(f"Abort {code}: {details}")

    async def collect_replies() -> list[pb.RuntimeMessage]:
        return [item async for item in handler.RunInstance(requests(), MockContext())]

    collect_task = asyncio.create_task(collect_replies())
    for _ in range(10):
        if handler.instances.store.instances["inst_1"].session is not None:
            break
        await asyncio.sleep(0)
    assert handler.instances.store.instances["inst_1"].session is not None

    await handler.DestroyInstance(
        pb.DestroyInstanceRequest(instance_id="inst_1", reason="test"),
        None,
    )

    replies = await asyncio.wait_for(collect_task, timeout=1)

    assert replies == []
    assert plugin.stop_count == 0


async def test_destroy_instance_does_not_call_on_stop_after_lifecycle_stop() -> None:
    handler = build_handler()
    await handler.CreateInstance(
        pb.CreateInstanceRequest(instance_id="inst_1", plugin_name="echo"),
        None,
    )
    plugin = handler.instances.get_instance("inst_1")
    assert isinstance(plugin, EchoPlugin)

    async def requests() -> AsyncIterator[pb.RuntimeMessage]:
        yield pb.RuntimeMessage(
            instance_id="inst_1",
            message_id="go_stop",
            type="RUNTIME_MESSAGE_TYPE_LIFECYCLE",
            lifecycle=pb.LifecycleEvent(type=pb.LIFECYCLE_TYPE_STOP),
        )

    class MockContext:
        def invocation_metadata(self):
            return (("instance-id", "inst_1"),)

        async def abort(self, code, details):
            raise Exception(f"Abort {code}: {details}")

    replies = [item async for item in handler.RunInstance(requests(), MockContext())]
    destroyed = await handler.DestroyInstance(
        pb.DestroyInstanceRequest(instance_id="inst_1", reason="test"),
        None,
    )

    assert len(replies) == 2
    assert replies[0].type == pb.RUNTIME_MESSAGE_TYPE_ACK
    assert replies[1].type == pb.RUNTIME_MESSAGE_TYPE_REPORT
    assert isinstance(destroyed, pb.DestroyInstanceResponse)
    assert plugin.stop_count == 1
