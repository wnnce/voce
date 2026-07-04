#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$PROJECT_DIR/../../.." && pwd)"

PROTO_FILE="$REPO_ROOT/api/plugin/v1/plugin.proto"
OUT_DIR="$PROJECT_DIR/src/proto"

mkdir -p "$OUT_DIR"

npx protoc \
  --ts_proto_out="$OUT_DIR" \
  --ts_proto_opt=outputServices=grpc-js \
  --ts_proto_opt=esModuleInterop=true \
  --ts_proto_opt=oneof=unions \
  --ts_proto_opt=env=node \
  --ts_proto_opt=useExactTypes=false \
  --ts_proto_opt=forceLong=number \
  --ts_proto_opt=snakeToCamel=true \
  --proto_path="$REPO_ROOT/api/plugin/v1" \
  "$PROTO_FILE"

echo "Proto generated to $OUT_DIR"
