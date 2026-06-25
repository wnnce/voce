from __future__ import annotations

from dataclasses import replace

from pydantic import BaseModel, TypeAdapter

from voce.core.plugin import (
    AsyncPlugin,
    MultiTrackConfig,
    PluginMetadata,
    PortMetadata,
    Property,
)


class EmptyPluginConfig(BaseModel):
    pass


type AsyncPluginClass = type[AsyncPlugin]
type PluginConfigClass = type[BaseModel]


class PluginRegistry:
    def __init__(self) -> None:
        self._plugins: dict[str, AsyncPluginClass] = {}
        self._configs: dict[str, PluginConfigClass] = {}
        self._metadata: dict[str, PluginMetadata] = {}

    def register(
        self,
        metadata: PluginMetadata,
        plugin_cls: AsyncPluginClass,
        config_cls: PluginConfigClass = EmptyPluginConfig,
    ) -> None:
        if metadata.name in self._plugins:
            raise ValueError(f"plugin already registered: {metadata.name}")
        self._plugins[metadata.name] = plugin_cls
        self._configs[metadata.name] = config_cls
        self._metadata[metadata.name] = self._metadata_with_schema(metadata, config_cls)

    def list_metadata(self) -> list[PluginMetadata]:
        return list(self._metadata.values())

    def create(self, name: str, config: bytes) -> AsyncPlugin:
        try:
            plugin_cls = self._plugins[name]
            config_cls = self._configs[name]
        except KeyError as exc:
            raise KeyError(f"plugin not found: {name}") from exc
        plugin_config = self._decode_config(config_cls, config)
        return plugin_cls(plugin_config)

    def _decode_config(self, config_cls: PluginConfigClass, config: bytes) -> BaseModel:
        if config:
            return TypeAdapter(config_cls).validate_json(config)
        return config_cls()

    def _metadata_with_schema(
        self,
        metadata: PluginMetadata,
        config_cls: PluginConfigClass,
    ) -> PluginMetadata:
        if metadata.schema is not None:
            return metadata
        return replace(metadata, schema=config_cls.model_json_schema())


plugin_registry = PluginRegistry()


def plugin(
    name: str,
    description: str = "",
    config_type: PluginConfigClass = EmptyPluginConfig,
    inputs: list[Property] | None = None,
    outputs: list[Property] | None = None,
    ports: list[PortMetadata] | None = None,
    multi_track: MultiTrackConfig | None = None,
):
    """Decorator to register a plugin class to the global plugin_registry."""

    def decorator(cls: AsyncPluginClass) -> AsyncPluginClass:
        metadata = PluginMetadata(
            name=name,
            description=description,
            inputs=inputs or [],
            outputs=outputs or [],
            ports=ports or [],
            multi_track=multi_track,
        )
        plugin_registry.register(metadata, cls, config_type)
        return cls

    return decorator
