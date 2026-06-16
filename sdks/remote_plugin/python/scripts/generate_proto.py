from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def main() -> None:
    root = Path(__file__).resolve().parents[4]
    sdk_root = Path(__file__).resolve().parents[1]
    proto_dir = root / "api" / "plugin" / "v1"
    proto = proto_dir / "plugin.proto"
    out = sdk_root / "app" / "proto"
    out.mkdir(parents=True, exist_ok=True)
    (out / "__init__.py").touch()

    subprocess.run(
        [
            sys.executable,
            "-m",
            "grpc_tools.protoc",
            f"--proto_path={proto_dir}",
            f"--python_out={out}",
            f"--pyi_out={out}",
            f"--grpc_python_out={out}",
            proto.name,
        ],
        check=True,
    )
    grpc_file = out / "plugin_pb2_grpc.py"
    if grpc_file.exists():
        grpc_file.write_text(
            grpc_file.read_text().replace(
                "import plugin_pb2 as plugin__pb2",
                "from app.proto import plugin_pb2 as plugin__pb2",
            )
        )


if __name__ == "__main__":
    main()
