from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator

import grpc

from app.core.logger import LogMessage
from app.proto import plugin_pb2 as pb
from app.proto import plugin_pb2_grpc as pbg
from app.service.converters import plugin_metadata_to_proto
from app.service.instances import PluginInstanceService

logger = logging.getLogger(__name__)


class RemotePluginServiceHandler(pbg.RemotePluginServiceServicer):
    def __init__(
        self,
        instances: PluginInstanceService,
        *,
        server_id: str,
        version: str,
    ) -> None:
        self.instances = instances
        self.server_id = server_id
        self.version = version

    async def Ping(self, request: pb.PingRequest, context: grpc.aio.ServicerContext):
        logger.debug("remote plugin ping client_id=%s", request.client_id)
        return pb.PingResponse(server_id=self.server_id, version=self.version)

    async def ListPlugins(
        self,
        request: pb.ListPluginsRequest,
        context: grpc.aio.ServicerContext,
    ) -> pb.ListPluginsResponse:
        plugins = [plugin_metadata_to_proto(item) for item in self.instances.list_metadata()]
        logger.info(
            "remote plugin list plugins namespace=%s plugin_count=%d",
            request.namespace,
            len(plugins),
        )
        return pb.ListPluginsResponse(plugins=plugins)

    async def CreateInstance(
        self,
        request: pb.CreateInstanceRequest,
        context: grpc.aio.ServicerContext,
    ) -> pb.CreateInstanceResponse:
        try:
            self.instances.create_instance(
                request.instance_id,
                request.plugin_name,
                request.config,
            )
        except KeyError as exc:
            await context.abort(grpc.StatusCode.NOT_FOUND, str(exc))
        except ValueError as exc:
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        logger.info(
            "remote plugin instance created instance_id=%s plugin=%s",
            request.instance_id,
            request.plugin_name,
        )
        return pb.CreateInstanceResponse(instance_id=request.instance_id)

    async def DestroyInstance(
        self,
        request: pb.DestroyInstanceRequest,
        context: grpc.aio.ServicerContext,
    ) -> pb.DestroyInstanceResponse:
        await self.instances.destroy_instance(request.instance_id)
        logger.info(
            "remote plugin instance destroyed instance_id=%s reason=%s",
            request.instance_id,
            request.reason,
        )
        return pb.DestroyInstanceResponse()

    async def RunInstance(
        self,
        request_iterator: AsyncIterator[pb.RuntimeMessage],
        context: grpc.aio.ServicerContext,
    ) -> AsyncIterator[pb.RuntimeMessage]:
        metadata = dict(context.invocation_metadata() or {})
        metadata_instance_id = metadata.get("instance-id")
        if not isinstance(metadata_instance_id, str) or not metadata_instance_id:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "instance-id metadata is required"
            )
            raise RuntimeError("unreachable")
        instance_id = metadata_instance_id

        try:
            plugin, log_handler = self.instances.get_instance_with_handler(instance_id)
        except KeyError as exc:
            await context.abort(grpc.StatusCode.NOT_FOUND, str(exc))

        output: asyncio.Queue[pb.RuntimeMessage | None] = asyncio.Queue()

        log_task = asyncio.create_task(
            log_handler.read_loop(output, self._make_log_msg_func(instance_id))
        )

        from app.service.session import PluginSession

        session = PluginSession(instance_id, plugin, output)
        try:
            self.instances.attach_session(instance_id, session)
        except ValueError as exc:
            await context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(exc))
        except KeyError as exc:
            await context.abort(grpc.StatusCode.NOT_FOUND, str(exc))

        worker = asyncio.create_task(session.process_stream(request_iterator))
        try:
            while True:
                message = await output.get()
                if message is None:
                    break
                yield message
        except asyncio.CancelledError:
            logger.info("remote plugin runtime stream canceled")
            raise
        finally:
            await session.close()
            self.instances.detach_session(instance_id, session)
            worker.cancel()
            log_task.cancel()
            await asyncio.gather(worker, log_task, return_exceptions=True)

    def _make_log_msg_func(self, instance_id: str):
        import uuid

        def make_msg(data: LogMessage) -> pb.RuntimeMessage:
            message = pb.RuntimeMessage(
                instance_id=instance_id,
                message_id=uuid.uuid4().hex,
                correlation_id="",
                type="RUNTIME_MESSAGE_TYPE_EMIT_LOG",
            )
            message.emit_log.level = data["level"]
            message.emit_log.message = data["message"]
            return message

        return make_msg
