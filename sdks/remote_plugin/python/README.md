# Voce Remote Plugin Python SDK

Python SDK skeleton for implementing Voce Remote Plugin servers.

## Requirements

- Python 3.14+
- uv

## Setup

```bash
uv sync
```

## Generate gRPC Code

The SDK uses the repository proto definition at `api/plugin/v1/plugin.proto`.

```bash
uv run python scripts/generate_proto.py
```

Generated files are written to:

```text
app/proto/
```

## Development

```bash
uv run black .
uv run ruff check .
uv run pytest
```

Generated protobuf files under `voce/proto/` are excluded from Black and Ruff.

## Run Example

```bash
cd ../examples/python
uv run python main.py
```

## Package Layout

```text
voce/
  app.py      # SDK App lifecycle
  proto/      # Generated protobuf modules and protocol imports
  core/       # Plugin base class, registry, metadata, and flow helper
  service/    # gRPC service handlers
```
