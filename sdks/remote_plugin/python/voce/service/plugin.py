from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator
from typing import TYPE_CHECKING

import grpc

from voce.core.logger import LogMessage
from voce.proto import plugin_pb2 as plugin_proto
from voce.proto import plugin_pb2_grpc as plugin_grpc
from voce.service.converters import plugin_metadata_to_proto
from voce.service.instances import PluginInstanceService

if TYPE_CHECKING:
    from voce.app import Config

logger = logging.getLogger(__name__)


class RemotePluginServiceHandler(plugin_grpc.RemotePluginServiceServicer):
    def __init__(
        self,
        plugin_instances: PluginInstanceService,
        config: Config,
    ) -> None:
        self.plugin_instances = plugin_instances
        self.config = config
        self.server_id = config.server_id
        self.version = config.version

    async def Ping(self, request: plugin_proto.PingRequest, context: grpc.aio.ServicerContext):
        logger.debug("remote plugin ping client_id=%s", request.client_id)
        return plugin_proto.PingResponse(server_id=self.server_id, version=self.version)

    async def ListPlugins(
        self,
        request: plugin_proto.ListPluginsRequest,
        context: grpc.aio.ServicerContext,
    ) -> plugin_proto.ListPluginsResponse:
        plugins = [plugin_metadata_to_proto(item) for item in self.plugin_instances.list_metadata()]
        logger.info(
            "remote plugin list plugins namespace=%s plugin_count=%d",
            request.namespace,
            len(plugins),
        )
        return plugin_proto.ListPluginsResponse(plugins=plugins)

    async def CreateInstance(
        self,
        request: plugin_proto.CreateInstanceRequest,
        context: grpc.aio.ServicerContext,
    ) -> plugin_proto.CreateInstanceResponse:
        try:
            self.plugin_instances.create_instance(
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
        return plugin_proto.CreateInstanceResponse(instance_id=request.instance_id)

    async def DestroyInstance(
        self,
        request: plugin_proto.DestroyInstanceRequest,
        context: grpc.aio.ServicerContext,
    ) -> plugin_proto.DestroyInstanceResponse:
        await self.plugin_instances.destroy_instance(request.instance_id)
        logger.info(
            "remote plugin instance destroyed instance_id=%s reason=%s",
            request.instance_id,
            request.reason,
        )
        return plugin_proto.DestroyInstanceResponse()

    async def RunInstance(
        self,
        request_iterator: AsyncIterator[plugin_proto.RuntimeMessage],
        context: grpc.aio.ServicerContext,
    ) -> AsyncIterator[plugin_proto.RuntimeMessage]:
        metadata = dict(context.invocation_metadata() or {})
        metadata_instance_id = metadata.get("instance-id")
        if not isinstance(metadata_instance_id, str) or not metadata_instance_id:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "instance-id metadata is required"
            )
            raise RuntimeError("unreachable")
        instance_id = metadata_instance_id

        try:
            plugin, log_handler = self.plugin_instances.get_instance_with_handler(instance_id)
        except KeyError as exc:
            await context.abort(grpc.StatusCode.NOT_FOUND, str(exc))

        outgoing_messages: asyncio.Queue[plugin_proto.RuntimeMessage | None] = asyncio.Queue()

        log_task = asyncio.create_task(
            log_handler.read_loop(outgoing_messages, self._create_log_message_builder(instance_id))
        )

        from voce.service.session import PluginSession

        session = PluginSession(
            instance_id,
            plugin,
            outgoing_messages,
            ack_interval_sec=self.config.ack_interval_sec,
        )
        try:
            self.plugin_instances.attach_session(instance_id, session)
        except ValueError as exc:
            await context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(exc))
        except KeyError as exc:
            await context.abort(grpc.StatusCode.NOT_FOUND, str(exc))

        process_stream_task = asyncio.create_task(session.process_stream(request_iterator))
        try:
            while True:
                message = await outgoing_messages.get()
                if message is None:
                    break
                yield message
        except asyncio.CancelledError:
            logger.info("remote plugin runtime stream canceled instance_id=%s", instance_id)
            raise
        except Exception:
            logger.exception("remote plugin runtime stream failed instance_id=%s", instance_id)
            raise
        finally:
            await session.close()
            self.plugin_instances.detach_session(instance_id, session)
            process_stream_task.cancel()
            log_task.cancel()
            await asyncio.gather(process_stream_task, log_task, return_exceptions=True)

    def _create_log_message_builder(self, instance_id: str):
        import uuid

        def make_msg(data: LogMessage) -> plugin_proto.RuntimeMessage:
            message = plugin_proto.RuntimeMessage(
                instance_id=instance_id,
                message_id=uuid.uuid4().hex,
                correlation_id="",
                type="RUNTIME_MESSAGE_TYPE_EMIT_LOG",
            )
            message.emit_log.level = data["level"]
            message.emit_log.message = data["message"]
            return message

        return make_msg
