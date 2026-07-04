import asyncio
import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from voce.core import AsyncPlugin, PluginRegistry
from voce.core.logger import RemoteLogHandler

if TYPE_CHECKING:
    from voce.app import Config
    from voce.service.session import PluginSession


@dataclass(slots=True)
class PluginInstance:
    plugin: AsyncPlugin
    log_handler: RemoteLogHandler
    session: PluginSession | None = None


@dataclass(slots=True)
class InstanceStore:
    instances: dict[str, PluginInstance] = field(default_factory=dict)


class PluginInstanceService:
    def __init__(self, registry: PluginRegistry, config: Config) -> None:
        self.registry = registry
        self.config = config
        self.store = InstanceStore()

    def create_instance(self, instance_id: str, plugin_name: str, config: bytes) -> None:
        if not instance_id:
            raise ValueError("instance_id is required")
        if instance_id in self.store.instances:
            raise ValueError(f"plugin instance already exists: {instance_id}")

        plugin = self.registry.create(plugin_name, config)

        loop = asyncio.get_running_loop()
        log_handler = RemoteLogHandler(loop=loop, max_size=self.config.log_queue_max_size)
        log_handler.setFormatter(logging.Formatter("%(message)s"))
        log_handler.setLevel(self.config.log_level)

        logger = logging.getLogger(f"plugin.{plugin_name}.{instance_id}")
        logger.setLevel(self.config.log_level)
        logger.addHandler(log_handler)

        plugin.logger = logger
        self.store.instances[instance_id] = PluginInstance(plugin, log_handler)

    async def destroy_instance(self, instance_id: str) -> None:
        instance = self.store.instances.get(instance_id)
        if instance is None:
            return

        try:
            if instance.session is not None:
                await instance.session.close()

            plugin_logger = getattr(instance.plugin, "logger", None)
            if isinstance(plugin_logger, logging.Logger):
                plugin_logger.removeHandler(instance.log_handler)
            instance.log_handler.close()
        finally:
            self.store.instances.pop(instance_id, None)

    def get_instance(self, instance_id: str) -> AsyncPlugin:
        try:
            return self.store.instances[instance_id].plugin
        except KeyError as exc:
            raise KeyError(f"plugin instance not found: {instance_id}") from exc

    def get_instance_with_handler(self, instance_id: str) -> tuple[AsyncPlugin, RemoteLogHandler]:
        try:
            instance = self.store.instances[instance_id]
        except KeyError as exc:
            raise KeyError(f"plugin instance not found: {instance_id}") from exc
        return instance.plugin, instance.log_handler

    def attach_session(self, instance_id: str, session: PluginSession) -> None:
        try:
            instance = self.store.instances[instance_id]
        except KeyError as exc:
            raise KeyError(f"plugin instance not found: {instance_id}") from exc
        if instance.session is not None and instance.session is not session:
            raise ValueError(f"plugin instance already has an active stream: {instance_id}")
        instance.session = session

    def detach_session(self, instance_id: str, session: PluginSession) -> None:
        instance = self.store.instances.get(instance_id)
        if instance is not None and instance.session is session:
            instance.session = None

    def list_metadata(self):
        return self.registry.list_metadata()
