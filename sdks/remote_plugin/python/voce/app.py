from __future__ import annotations

import asyncio
import logging
import signal
from collections.abc import Callable
from dataclasses import dataclass
from types import FrameType

import grpc

from voce.core.registry import plugin_registry
from voce.proto import plugin_pb2_grpc as plugin_grpc
from voce.service.instances import PluginInstanceService
from voce.service.plugin import RemotePluginServiceHandler

logger = logging.getLogger(__name__)


@dataclass(slots=True)
class Config:
    host: str = "127.0.0.1"
    port: int = 50051
    server_id: str = "python-remote-plugin"
    version: str = "0.1.0"
    stop_grace_period_sec: int | float = 5.0
    ack_interval_sec: float = 10.0
    log_queue_max_size: int = 1000
    log_level: int = logging.INFO


class App:
    """Voce remote plugin application.

    Provides lifecycle management methods (start, stop, serve) for the underlying gRPC server.
    """

    def __init__(self, config: Config | None = None) -> None:
        self.config = config or Config()
        self.plugin_instances = PluginInstanceService(plugin_registry, self.config)
        self._grpc_server: grpc.aio.Server | None = None

    async def start(self) -> None:
        """Starts the remote plugin server without blocking."""
        if self._grpc_server is not None:
            raise RuntimeError("App is already started")

        self._grpc_server = grpc.aio.server()
        plugin_grpc.add_RemotePluginServiceServicer_to_server(
            RemotePluginServiceHandler(
                self.plugin_instances,
                self.config,
            ),
            self._grpc_server,
        )
        listen_addr = f"{self.config.host}:{self.config.port}"
        try:
            bound_port = self._grpc_server.add_insecure_port(listen_addr)
        except RuntimeError:
            logger.exception("remote plugin server bind failed listen_addr=%s", listen_addr)
            raise
        if bound_port == 0:
            raise RuntimeError(f"failed to bind remote plugin server: {listen_addr}")

        logger.info(
            "remote plugin server starting server_id=%s version=%s listen_addr=%s plugin_count=%d",
            self.config.server_id,
            self.config.version,
            listen_addr,
            len(plugin_registry.list_metadata()),
        )
        await self._grpc_server.start()
        logger.info("remote plugin server started listen_addr=%s", listen_addr)

    async def stop(self, grace: int | float | None = None) -> None:
        """Gracefully stops the remote plugin server."""
        if self._grpc_server is None:
            return

        if grace is None:
            grace = self.config.stop_grace_period_sec

        listen_addr = f"{self.config.host}:{self.config.port}"
        logger.info("remote plugin server stopping listen_addr=%s", listen_addr)
        await self._grpc_server.stop(grace=grace)
        logger.info("remote plugin server stopped listen_addr=%s", listen_addr)
        self._grpc_server = None

    async def serve(self) -> None:
        """Starts the server, blocks until a shutdown signal is received, then stops it."""
        await self.start()
        stop_event = asyncio.Event()
        self._install_signal_handlers(stop_event)
        try:
            await stop_event.wait()
        finally:
            await self.stop()

    def _install_signal_handlers(self, stop_event: asyncio.Event) -> None:
        loop = asyncio.get_running_loop()
        for sig in (signal.SIGINT, signal.SIGTERM):
            try:
                loop.add_signal_handler(sig, self._request_stop, stop_event, sig)
            except NotImplementedError:
                signal.signal(sig, self._make_signal_handler(stop_event, sig))

    @staticmethod
    def _request_stop(stop_event: asyncio.Event, sig: signal.Signals) -> None:
        if stop_event.is_set():
            return
        logger.info("remote plugin server received shutdown signal signal=%s", sig.name)
        stop_event.set()

    def _make_signal_handler(
        self,
        stop_event: asyncio.Event,
        sig: signal.Signals,
    ) -> Callable[[int, FrameType | None], None]:
        def handler(_signum: int, _frame: FrameType | None) -> None:
            self._request_stop(stop_event, sig)

        return handler
