#!/usr/bin/env bash
set -euo pipefail

# install-sys-deps.sh - Install system build tools and C/C++ dependencies for Voce.
# Supported operating systems: macOS, Ubuntu, Debian, Arch Linux.

log_info() {
  echo -e "\033[1;34m[INFO]\033[0m $*"
}

log_error() {
  echo -e "\033[1;31m[ERROR]\033[0m $*" >&2
}

detect_os() {
  case "$(uname -s)" in
    Darwin*)
      echo "macos"
      ;;
    Linux*)
      if [ -f /etc/os-release ]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        case "${ID:-}" in
          ubuntu)
            echo "ubuntu"
            ;;
          debian)
            echo "debian"
            ;;
          arch|manjaro|endeavouros)
            echo "arch"
            ;;
          *)
            if [[ "${ID_LIKE:-}" =~ (ubuntu|debian) ]]; then
              echo "debian"
            elif [[ "${ID_LIKE:-}" =~ arch ]]; then
              echo "arch"
            else
              echo "unknown"
            fi
            ;;
        esac
      else
        echo "unknown"
      fi
      ;;
    *)
      echo "unknown"
      ;;
  esac
}

get_sudo() {
  if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
      echo "sudo"
    else
      log_error "Running as non-root user and 'sudo' is not available. Cannot install system packages."
      exit 1
    fi
  else
    echo ""
  fi
}

install_macos() {
  log_info "Detected OS: macOS"
  if ! command -v brew >/dev/null 2>&1; then
    log_error "Homebrew not found. Please install Homebrew first: https://brew.sh"
    exit 1
  fi

  log_info "Installing dependencies via Homebrew (pkg-config, ffmpeg, opus, opusfile, curl, unzip, git)..."
  brew update
  brew install pkg-config ffmpeg opus opusfile curl unzip git
  log_info "macOS system dependencies installed successfully."
}

install_debian_ubuntu() {
  local os_name="$1"
  log_info "Detected OS: $os_name"
  if ! command -v apt-get >/dev/null 2>&1; then
    log_error "Package manager 'apt-get' not found."
    exit 1
  fi

  local SUDO
  SUDO="$(get_sudo)"

  log_info "Updating package lists..."
  $SUDO apt-get update -y

  log_info "Installing system packages (build-essential, FFmpeg dev libs, libopus-dev, libopusfile-dev, libc++, pkg-config, curl, unzip, git)..."
  $SUDO apt-get install -y \
    build-essential \
    pkg-config \
    libavformat-dev \
    libavcodec-dev \
    libswresample-dev \
    libavutil-dev \
    libopus-dev \
    libopusfile-dev \
    libc++-dev \
    libc++abi-dev \
    curl \
    unzip \
    git

  log_info "$os_name system dependencies installed successfully."
}

install_arch() {
  log_info "Detected OS: Arch Linux"
  if ! command -v pacman >/dev/null 2>&1; then
    log_error "Package manager 'pacman' not found."
    exit 1
  fi

  local SUDO
  SUDO="$(get_sudo)"

  log_info "Installing packages via pacman (base-devel, pkgconf, ffmpeg, opus, opusfile, libc++, curl, unzip, git)..."
  $SUDO pacman -Sy --noconfirm --needed \
    base-devel \
    pkgconf \
    ffmpeg \
    opus \
    opusfile \
    libc++ \
    curl \
    unzip \
    git

  log_info "Arch Linux system dependencies installed successfully."
}

main() {
  local os
  os="$(detect_os)"

  case "$os" in
    macos)
      install_macos
      ;;
    ubuntu|debian)
      install_debian_ubuntu "$os"
      ;;
    arch)
      install_arch
      ;;
    *)
      log_error "Unsupported operating system. Supported systems: macOS, Ubuntu, Debian, Arch Linux."
      exit 1
      ;;
  esac
}

main "$@"
