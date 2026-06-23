import asyncio
import contextvars
import logging
import time
import uuid
from contextlib import suppress

from app.core import AsyncPlugin, Flow
from app.proto import plugin_pb2 as pb
from app.schema import Payload, Signal

logger = logging.getLogger(__name__)

DEFAULT_ACK_INTERVAL_SEC = 10.0

current_correlation_id: contextvars.ContextVar[str] = contextvars.ContextVar(
    "current_correlation_id",
    default="",
)


class PluginSession(Flow):
    def __init__(
        self,
        instance_id: str,
        plugin: AsyncPlugin,
        output: asyncio.Queue[pb.RuntimeMessage | None],
        *,
        ack_interval_sec: float = DEFAULT_ACK_INTERVAL_SEC,
    ) -> None:
        self.instance_id = instance_id
        self.plugin = plugin
        self.output = output
        self.ack_interval_sec = ack_interval_sec
        self._closed = False
        self._close_lock = asyncio.Lock()
        self._tasks: dict[str, asyncio.Task[None]] = {}

    async def send_payload(
        self,
        payload: Payload,
        *,
        port: int = 0,
    ) -> None:
        msg = self._new_runtime_message("RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD")
        msg.emit_payload.payload.name = payload.name
        msg.emit_payload.payload.properties = payload.to_json_bytes()
        msg.emit_payload.port = port
        await self.output.put(msg)

    async def send_signal(
        self,
        signal: Signal,
        *,
        port: int = 0,
    ) -> None:
        msg = self._new_runtime_message("RUNTIME_MESSAGE_TYPE_EMIT_SIGNAL")
        msg.emit_signal.signal.name = signal.name
        msg.emit_signal.signal.properties = signal.to_json_bytes()
        msg.emit_signal.port = port
        await self.output.put(msg)

    def _new_runtime_message(
        self,
        message_type: str,
        *,
        correlation_id: str | None = None,
    ) -> pb.RuntimeMessage:
        return pb.RuntimeMessage(
            instance_id=self.instance_id,
            message_id=uuid.uuid4().hex,
            correlation_id=(
                current_correlation_id.get() if correlation_id is None else correlation_id
            ),
            type=message_type,
        )

    async def process_stream(self, request_iterator) -> None:
        try:
            async for message in request_iterator:
                if message.type == pb.RUNTIME_MESSAGE_TYPE_CANCEL:
                    self._handle_cancel(message)
                    continue
                self._start_message_task(message)
        except asyncio.CancelledError:
            await self.close()
            raise
        except Exception:
            logger.exception("remote plugin runtime stream failed")
        finally:
            await self._drain_running_tasks()
            with suppress(Exception):
                await self.close()

    async def close(self) -> None:
        async with self._close_lock:
            if self._closed:
                return
            self._closed = True
            for task in self._tasks.values():
                task.cancel()
            await self.output.put(None)

    def _start_message_task(self, message: pb.RuntimeMessage) -> None:
        correlation_id = message.message_id
        task = asyncio.create_task(self._handle_runtime_message(message))
        self._tasks[correlation_id] = task

    def _handle_cancel(self, message: pb.RuntimeMessage) -> None:
        correlation_id = message.correlation_id
        if not correlation_id:
            return
        task = self._tasks.get(correlation_id)
        if task is not None and not task.done():
            task.cancel()

    async def _handle_runtime_message(self, message: pb.RuntimeMessage) -> None:
        correlation_id = message.message_id
        token = current_correlation_id.set(correlation_id)
        ack_task: asyncio.Task[None] | None = None
        try:
            await self._send_ack(correlation_id)
            ack_task = asyncio.create_task(self._ack_renew_loop(correlation_id))

            await self._dispatch_message(message)
        except asyncio.CancelledError:
            report = self._new_runtime_message(
                "RUNTIME_MESSAGE_TYPE_REPORT",
                correlation_id=correlation_id,
            )
            report.report.status = pb.REPORT_STATUS_CANCELED
            await self.output.put(report)
        except Exception as exc:
            logger.exception(
                "remote plugin event failed instance_id=%s message_id=%s type=%s",
                message.instance_id,
                message.message_id,
                message.type,
            )
            report = self._new_runtime_message("RUNTIME_MESSAGE_TYPE_REPORT")
            report.report.status = pb.REPORT_STATUS_ERROR
            report.report.error.code = exc.__class__.__name__
            report.report.error.message = str(exc)
            await self.output.put(report)
        else:
            report = self._new_runtime_message("RUNTIME_MESSAGE_TYPE_REPORT")
            report.report.status = pb.REPORT_STATUS_OK
            await self.output.put(report)
        finally:
            if ack_task is not None:
                ack_task.cancel()
                with suppress(asyncio.CancelledError):
                    await ack_task
            current_correlation_id.reset(token)
            self._tasks.pop(correlation_id, None)

    async def _send_ack(self, correlation_id: str) -> None:
        ack = self._new_runtime_message(
            "RUNTIME_MESSAGE_TYPE_ACK",
            correlation_id=correlation_id,
        )
        ack.ack.timestamp = time.time_ns() // 1_000_000
        await self.output.put(ack)

    async def _ack_renew_loop(self, correlation_id: str) -> None:
        while True:
            await asyncio.sleep(self.ack_interval_sec)
            await self._send_ack(correlation_id)

    async def _dispatch_message(self, message: pb.RuntimeMessage) -> None:
        body = message.WhichOneof("body")
        match body:
            case "lifecycle":
                await self._dispatch_lifecycle(message.lifecycle.type)
            case "signal":
                await self.plugin.on_signal(
                    self,
                    Signal.from_json_bytes(message.signal.name, message.signal.properties),
                )
            case "payload":
                await self.plugin.on_payload(
                    self,
                    Payload.from_json_bytes(message.payload.name, message.payload.properties),
                )
            case _:
                raise ValueError(f"unsupported runtime message body: {body or '<empty>'}")

    async def _dispatch_lifecycle(self, lifecycle_type: int) -> None:
        match lifecycle_type:
            case pb.LIFECYCLE_TYPE_START:
                await self.plugin.on_start(self)
            case pb.LIFECYCLE_TYPE_READY:
                await self.plugin.on_ready(self)
            case pb.LIFECYCLE_TYPE_PAUSE:
                await self.plugin.on_pause()
            case pb.LIFECYCLE_TYPE_RESUME:
                await self.plugin.on_resume(self)
            case pb.LIFECYCLE_TYPE_STOP:
                await self.plugin.on_stop()
            case _:
                raise ValueError(f"unsupported lifecycle type: {lifecycle_type}")

    async def _drain_running_tasks(self) -> None:
        tasks = list(self._tasks.values())
        if not tasks:
            return
        await asyncio.gather(*tasks, return_exceptions=True)
        self._tasks.clear()
