#!/usr/bin/env bash
set -euo pipefail

# prepare-libs.sh - Fetch and prepare project-level external libraries (ten-vad, agora-server-sdk).
# Does NOT require root/sudo permissions.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LIBS_DIR="$REPO_ROOT/libs"

TEN_VAD_DIR="$LIBS_DIR/ten-vad"
TEN_VAD_REPO="https://github.com/TEN-framework/ten-vad.git"

AGORA_SDK_DIR="$LIBS_DIR/agora-server-sdk"
AGORA_SDK_REPO="https://github.com/AgoraIO-Extensions/Agora-Golang-Server-SDK.git"
AGORA_SDK_TAG="v2.6.6"

log_info() {
  echo -e "\033[1;34m[INFO]\033[0m $*"
}

log_error() {
  echo -e "\033[1;31m[ERROR]\033[0m $*" >&2
}

check_prerequisites() {
  for cmd in git curl unzip; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      log_error "Missing required tool: $cmd. Please run ./scripts/install-sys-deps.sh first."
      exit 1
    fi
  done
}

prepare_ten_vad() {
  log_info "Checking ten-vad ($TEN_VAD_DIR)..."
  mkdir -p "$LIBS_DIR"
  if [ ! -d "$TEN_VAD_DIR" ]; then
    log_info "Cloning ten-vad repository..."
    git clone "$TEN_VAD_REPO" "$TEN_VAD_DIR"
  else
    log_info "ten-vad is up to date."
  fi
}

prepare_agora_sdk() {
  log_info "Checking agora-server-sdk ($AGORA_SDK_DIR)..."
  mkdir -p "$LIBS_DIR"
  if [ ! -d "$AGORA_SDK_DIR" ]; then
    log_info "Cloning agora-server-sdk ($AGORA_SDK_TAG)..."
    git clone --depth 1 -b "$AGORA_SDK_TAG" "$AGORA_SDK_REPO" "$AGORA_SDK_DIR"
  else
    log_info "agora-server-sdk is up to date."
  fi

  if [ ! -d "$AGORA_SDK_DIR/agora_sdk" ] && [ ! -d "$AGORA_SDK_DIR/agora_sdk_mac" ]; then
    log_info "Downloading and extracting Agora native C/C++ SDK binaries..."
    (cd "$AGORA_SDK_DIR" && ./scripts/install_agora_sdk.sh)
  else
    log_info "Agora native C/C++ SDK binaries already installed."
  fi
}

main() {
  check_prerequisites
  prepare_ten_vad
  prepare_agora_sdk
  log_info "All external libraries are ready."
}

main "$@"
