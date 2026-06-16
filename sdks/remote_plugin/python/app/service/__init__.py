"""gRPC service implementation for the Voce remote plugin server."""

from app.server import RemotePluginServer, ServerConfig, main, serve

__all__ = ["RemotePluginServer", "ServerConfig", "main", "serve"]
