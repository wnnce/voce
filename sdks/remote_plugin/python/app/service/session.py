import asyncio
import logging
import time
import uuid
from contextlib import suppress

from app.core import AsyncPlugin, Flow
from app.proto import plugin_pb2 as pb
from app.schema import Payload, Signal

logger = logging.getLogger(__name__)


class PluginSession(Flow):
    def __init__(
        self,
        instance_id: str,
        plugin: AsyncPlugin,
        output: asyncio.Queue[pb.RuntimeMessage | None],
    ) -> None:
        self.instance_id = instance_id
        self.plugin = plugin
        self.output = output
        self.correlation_id = ""
        self._closed = False
        self._close_lock = asyncio.Lock()

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

    def _new_runtime_message(self, message_type: str) -> pb.RuntimeMessage:
        return pb.RuntimeMessage(
            instance_id=self.instance_id,
            message_id=uuid.uuid4().hex,
            correlation_id=self.correlation_id,
            type=message_type,
        )

    async def process_stream(self, request_iterator) -> None:
        try:
            async for message in request_iterator:
                await self._handle_runtime_message(message)
        except asyncio.CancelledError:
            await self.close()
            raise
        except Exception:
            logger.exception("remote plugin runtime stream failed")
        finally:
            with suppress(Exception):
                await self.close()

    async def close(self) -> None:
        async with self._close_lock:
            if self._closed:
                return
            self._closed = True
            await self.output.put(None)

    async def _handle_runtime_message(self, message: pb.RuntimeMessage) -> None:
        self.correlation_id = message.message_id
        try:
            ack = self._new_runtime_message("RUNTIME_MESSAGE_TYPE_ACK")
            ack.ack.timestamp = time.time_ns() // 1_000_000
            await self.output.put(ack)

            await self._dispatch_message(message)
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
            self.correlation_id = ""

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
