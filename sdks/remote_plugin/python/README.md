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

Generated protobuf files under `app/proto/` are excluded from Black and Ruff.

## Run Server

```bash
uv run python -m app.server --host 127.0.0.1 --port 50051
```

or use the project script:

```bash
uv run voce-remote-plugin --host 127.0.0.1 --port 50051
```

## Package Layout

```text
app/
  server.py    # asyncio gRPC server entrypoint
  proto/      # Generated protobuf modules and protocol imports
  core/       # Plugin base class, registry, metadata, and flow helper
  plugins/    # User plugin implementations
  service/    # gRPC service handlers and server entrypoint
```
