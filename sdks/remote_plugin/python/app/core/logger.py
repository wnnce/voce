import asyncio
import logging
from collections.abc import Callable
from typing import TypedDict

from app.proto import plugin_pb2 as pb


class LogMessage(TypedDict):
    level: pb.LogLevel
    message: str


_LOG_LEVEL_MAP: dict[int, pb.LogLevel] = {
    logging.DEBUG: pb.LOG_LEVEL_DEBUG,
    logging.INFO: pb.LOG_LEVEL_INFO,
    logging.WARNING: pb.LOG_LEVEL_WARN,
    logging.ERROR: pb.LOG_LEVEL_ERROR,
    logging.CRITICAL: pb.LOG_LEVEL_ERROR,
}


class RemoteLogHandler(logging.Handler):
    """Handler that queues log records to be sent over a gRPC stream.

    Logs are buffered in an asyncio.Queue to prevent blocking the plugin's fast path.
    If the buffer fills up, new logs are dropped to avoid memory explosion.
    """

    def __init__(self, loop: asyncio.AbstractEventLoop, max_size: int = 1000) -> None:
        super().__init__()
        self.loop = loop
        self.queue: asyncio.Queue[LogMessage] = asyncio.Queue(maxsize=max_size)
        self.setLevel(logging.INFO)

    def _put_message(self, msg: LogMessage) -> None:
        try:
            self.queue.put_nowait(msg)
        except asyncio.QueueFull:
            pass  # Drop log if the stream is too slow to avoid blocking/memory leak

    def emit(self, record: logging.LogRecord) -> None:
        try:
            msg = self.format_to_msg(record)
            self.loop.call_soon_threadsafe(self._put_message, msg)
        except Exception:
            self.handleError(record)

    def format_to_msg(self, record: logging.LogRecord) -> LogMessage:
        """Convert LogRecord to a dictionary compatible with EmitLog."""
        level = _LOG_LEVEL_MAP.get(record.levelno, pb.LOG_LEVEL_UNSPECIFIED)
        message = self.format(record)

        return {
            "level": level,
            "message": message,
        }

    async def read_loop(
        self,
        output: asyncio.Queue[pb.RuntimeMessage | None],
        make_msg_func: Callable[[LogMessage], pb.RuntimeMessage],
    ) -> None:
        """Loop that reads from the internal queue and puts to the gRPC output queue.

        This loop should be run as an asyncio Task when the gRPC stream is established.
        """
        try:
            while True:
                msg = await self.queue.get()
                pb_msg = make_msg_func(msg)
                await output.put(pb_msg)
                self.queue.task_done()
        except asyncio.CancelledError:
            pass
