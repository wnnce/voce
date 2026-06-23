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
from app.service.session import PluginSession


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


async def test_session_renews_ack_while_payload_is_processing() -> None:
    release = asyncio.Event()

    class SlowPlugin(AsyncPlugin[EchoConfig]):
        async def on_payload(self, flow: Flow, payload: Payload) -> None:
            await release.wait()

    output: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()
    session = PluginSession(
        "inst_1",
        SlowPlugin(EchoConfig()),
        output,
        ack_interval_sec=0.01,
    )
    message = pb.RuntimeMessage(
        instance_id="inst_1",
        message_id="go_1",
        type="RUNTIME_MESSAGE_TYPE_PAYLOAD",
        payload=pb.PayloadEvent(name="demo", properties=json_bytes({"text": "hello"})),
    )

    task = asyncio.create_task(session._handle_runtime_message(message))
    acks: list[pb.RuntimeMessage] = []
    while len(acks) < 2:
        item = await asyncio.wait_for(output.get(), timeout=0.2)
        assert item is not None
        assert item.type == pb.RUNTIME_MESSAGE_TYPE_ACK
        assert item.correlation_id == "go_1"
        acks.append(item)

    release.set()

    report = None
    for _ in range(5):
        item = await asyncio.wait_for(output.get(), timeout=0.2)
        assert item is not None
        if item.type == pb.RUNTIME_MESSAGE_TYPE_REPORT:
            report = item
            break
        assert item.type == pb.RUNTIME_MESSAGE_TYPE_ACK

    await asyncio.wait_for(task, timeout=0.2)

    assert report is not None
    assert report.correlation_id == "go_1"
    assert report.report.status == pb.REPORT_STATUS_OK


async def test_session_cancel_message_cancels_running_payload() -> None:
    started = asyncio.Event()

    class SlowPlugin(AsyncPlugin[EchoConfig]):
        async def on_payload(self, flow: Flow, payload: Payload) -> None:
            started.set()
            await asyncio.Event().wait()

    output: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()
    session = PluginSession(
        "inst_1",
        SlowPlugin(EchoConfig()),
        output,
        ack_interval_sec=0.01,
    )
    requests: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()

    async def request_iter() -> AsyncIterator[pb.RuntimeMessage]:
        while True:
            item = await requests.get()
            if item is None:
                return
            yield item

    task = asyncio.create_task(session.process_stream(request_iter()))
    await requests.put(
        pb.RuntimeMessage(
            instance_id="inst_1",
            message_id="go_1",
            type="RUNTIME_MESSAGE_TYPE_PAYLOAD",
            payload=pb.PayloadEvent(name="demo", properties=json_bytes({"text": "hello"})),
        )
    )

    ack = await asyncio.wait_for(output.get(), timeout=0.2)
    assert ack is not None
    assert ack.type == pb.RUNTIME_MESSAGE_TYPE_ACK
    assert ack.correlation_id == "go_1"
    await asyncio.wait_for(started.wait(), timeout=0.2)

    await requests.put(
        pb.RuntimeMessage(
            instance_id="inst_1",
            message_id="cancel_1",
            correlation_id="go_1",
            type="RUNTIME_MESSAGE_TYPE_CANCEL",
            cancel=pb.CancelEvent(),
        )
    )

    report = None
    for _ in range(5):
        item = await asyncio.wait_for(output.get(), timeout=0.2)
        assert item is not None
        if item.type == pb.RUNTIME_MESSAGE_TYPE_REPORT:
            report = item
            break
        assert item.type == pb.RUNTIME_MESSAGE_TYPE_ACK

    assert report is not None
    assert report.correlation_id == "go_1"
    assert report.report.status == pb.REPORT_STATUS_CANCELED

    await requests.put(None)
    await asyncio.wait_for(task, timeout=0.2)


async def test_session_concurrent_tasks_keep_emit_correlation_ids() -> None:
    release: dict[str, asyncio.Event] = {
        "go_1": asyncio.Event(),
        "go_2": asyncio.Event(),
    }

    class CorrelationPlugin(AsyncPlugin[EchoConfig]):
        async def on_payload(self, flow: Flow, payload: Payload) -> None:
            key = payload.get_as("key", str, "")
            await release[key].wait()
            await flow.send_payload(Payload("reply", {"key": key}))

    output: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()
    session = PluginSession("inst_1", CorrelationPlugin(EchoConfig()), output)

    first = asyncio.create_task(
        session._handle_runtime_message(
            pb.RuntimeMessage(
                instance_id="inst_1",
                message_id="go_1",
                type="RUNTIME_MESSAGE_TYPE_PAYLOAD",
                payload=pb.PayloadEvent(
                    name="demo",
                    properties=json_bytes({"key": "go_1"}),
                ),
            )
        )
    )
    second = asyncio.create_task(
        session._handle_runtime_message(
            pb.RuntimeMessage(
                instance_id="inst_1",
                message_id="go_2",
                type="RUNTIME_MESSAGE_TYPE_PAYLOAD",
                payload=pb.PayloadEvent(
                    name="demo",
                    properties=json_bytes({"key": "go_2"}),
                ),
            )
        )
    )

    seen_acks: set[str] = set()
    while seen_acks != {"go_1", "go_2"}:
        item = await asyncio.wait_for(output.get(), timeout=0.2)
        assert item is not None
        assert item.type == pb.RUNTIME_MESSAGE_TYPE_ACK
        seen_acks.add(item.correlation_id)

    release["go_2"].set()
    second_emit = await asyncio.wait_for(output.get(), timeout=0.2)
    assert second_emit is not None
    assert second_emit.type == pb.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD
    assert second_emit.correlation_id == "go_2"
    second_report = await asyncio.wait_for(output.get(), timeout=0.2)
    assert second_report is not None
    assert second_report.type == pb.RUNTIME_MESSAGE_TYPE_REPORT
    assert second_report.correlation_id == "go_2"

    release["go_1"].set()
    first_emit = await asyncio.wait_for(output.get(), timeout=0.2)
    assert first_emit is not None
    assert first_emit.type == pb.RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD
    assert first_emit.correlation_id == "go_1"
    first_report = await asyncio.wait_for(output.get(), timeout=0.2)
    assert first_report is not None
    assert first_report.type == pb.RUNTIME_MESSAGE_TYPE_REPORT
    assert first_report.correlation_id == "go_1"

    await asyncio.wait_for(asyncio.gather(first, second), timeout=0.2)


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
