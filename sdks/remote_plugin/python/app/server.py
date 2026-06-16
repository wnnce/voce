from __future__ import annotations

import argparse
import asyncio
import logging
import signal
from collections.abc import Callable
from dataclasses import dataclass
from types import FrameType

import grpc

from app.core import PluginRegistry
from app.plugins import register_plugins
from app.proto import plugin_pb2_grpc as pbg
from app.service.instances import PluginInstanceService
from app.service.plugin import RemotePluginServiceHandler

logger = logging.getLogger(__name__)


@dataclass(slots=True)
class ServerConfig:
    host: str = "127.0.0.1"
    port: int = 50051
    server_id: str = "python-remote-plugin"
    version: str = "0.1.0"


class RemotePluginServer:
    """asyncio gRPC server skeleton.

    Run `uv run python scripts/generate_proto.py` before wiring the generated protobuf
    service classes into this server.
    """

    def __init__(self, registry: PluginRegistry, config: ServerConfig | None = None) -> None:
        self.registry = registry
        self.config = config or ServerConfig()
        self.instances = PluginInstanceService(registry)

    async def start(self) -> None:
        server = grpc.aio.server()
        pbg.add_RemotePluginServiceServicer_to_server(
            RemotePluginServiceHandler(
                self.instances,
                server_id=self.config.server_id,
                version=self.config.version,
            ),
            server,
        )
        listen_addr = f"{self.config.host}:{self.config.port}"
        try:
            bound_port = server.add_insecure_port(listen_addr)
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
            len(self.registry.list_metadata()),
        )
        await server.start()
        logger.info("remote plugin server started listen_addr=%s", listen_addr)
        stop_event = asyncio.Event()
        self._install_signal_handlers(stop_event)
        try:
            await stop_event.wait()
        finally:
            logger.info("remote plugin server stopping listen_addr=%s", listen_addr)
            await server.stop(grace=5)
            logger.info("remote plugin server stopped listen_addr=%s", listen_addr)

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


async def serve(registry: PluginRegistry, config: ServerConfig | None = None) -> None:
    await RemotePluginServer(registry, config).start()


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    parser = argparse.ArgumentParser(description="Run a Voce remote plugin server")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=50051)
    args = parser.parse_args()

    registry = PluginRegistry()
    register_plugins(registry)
    logger.info(
        "remote plugin registry initialized plugin_count=%d",
        len(registry.list_metadata()),
    )
    asyncio.run(serve(registry, ServerConfig(host=args.host, port=args.port)))


if __name__ == "__main__":
    main()
