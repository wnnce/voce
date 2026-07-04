"""Remote plugin implementations live here."""

# Import built-in plugins so they register themselves via decorators
from plugins.openai_llm_stream import plugin as _openai_plugin
from plugins.passthrough import plugin as _passthrough_plugin

__all__ = ["_openai_plugin", "_passthrough_plugin"]
