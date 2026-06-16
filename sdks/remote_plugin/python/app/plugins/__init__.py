"""Remote plugin implementations live here."""

import importlib
import inspect
import logging
import pkgutil
from collections.abc import Callable
from typing import Any, cast

from app.core import PluginRegistry

logger = logging.getLogger(__name__)

RegisterFunc = Callable[[PluginRegistry], None]


def register_plugins(registry: PluginRegistry) -> None:
    for module_info in pkgutil.iter_modules(__path__, prefix=f"{__name__}."):
        if not module_info.ispkg:
            continue
        register = _load_register(module_info.name)
        if register is None:
            continue
        register(registry)


def _load_register(package_name: str) -> RegisterFunc | None:
    try:
        module = importlib.import_module(f"{package_name}.plugin")
    except ModuleNotFoundError as exc:
        if exc.name == f"{package_name}.plugin":
            return None
        raise
    register = getattr(module, "register", None)
    if register is None:
        logger.debug("remote plugin package has no register function package=%s", package_name)
        return None
    return _validate_register(package_name, register)


def _validate_register(package_name: str, value: Any) -> RegisterFunc:
    if not callable(value):
        raise TypeError(f"{package_name}.plugin.register must be callable")
    signature = inspect.signature(value)
    if len(signature.parameters) != 1:
        raise TypeError(f"{package_name}.plugin.register must accept exactly one registry argument")
    return cast(RegisterFunc, value)
