# syntax=docker/dockerfile:1

# ─── Stage 1: Build Frontend ──────────────────────────────────────────────────
FROM node:24-bookworm-slim AS frontend-builder
WORKDIR /app
COPY clients/web/package.json clients/web/pnpm-lock.yaml ./clients/web/
RUN npm install -g pnpm@9 && cd clients/web && pnpm install --frozen-lockfile
COPY clients/web/ ./clients/web/
RUN cd clients/web && pnpm build

# ─── Stage 2: Build Backend ───────────────────────────────────────────────────
FROM golang:1.27-bookworm AS backend-builder

# Install build tools and system dependencies (FFmpeg, git-lfs)
RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    libswresample-dev \
    libavutil-dev \
    libc++-dev \
    libc++abi-dev \
    git \
    git-lfs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

# Prepare plugin dependencies (e.g., ten-vad)
COPY . .
RUN git lfs install --system && make deps

# Embed frontend and build the backend binaries
COPY --from=frontend-builder /app/clients/web/dist ./clients/web/dist

# Build Voce (CGO enabled for plugins)
RUN GOOS=linux go build -v -ldflags="-w -s" -o voce ./cmd/voce

# Build Voce Gateway (Static binary for Alpine compatibility)
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o voce-gateway ./cmd/gateway

# ─── Stage 3: Voce Runtime Image ──────────────────────────────────────────────
FROM debian:bookworm-slim AS voce-runtime

# Install runtime shared libraries for audio processing and C++ runtime
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    wget \
    libswresample4 \
    libavutil57 \
    libc++1 \
    libc++abi1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN mkdir -p /etc/voce

# Copy binaries and library files
COPY --from=backend-builder /app/voce /usr/local/bin/voce
COPY --from=backend-builder /app/libs/ten-vad/lib/Linux/x64/libten_vad.so /usr/local/lib/
RUN ldconfig

# Copy required ONNX model file for ten-vad
RUN mkdir -p src/onnx_model
COPY --from=backend-builder /app/libs/ten-vad/src/onnx_model/ten-vad.onnx ./src/onnx_model/

# Environment variables for execution
ENV LD_LIBRARY_PATH=/usr/local/lib

EXPOSE 7001 7002 7003

ENTRYPOINT ["voce"]
CMD ["--config", "/etc/voce/config.yaml"]

# ─── Stage 4: Gateway Runtime Image ───────────────────────────────────────────
FROM alpine:latest AS voce-gateway-runtime

# Install basic runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
RUN mkdir -p /etc/voce

# Copy static gateway binary
COPY --from=backend-builder /app/voce-gateway /usr/local/bin/voce-gateway

EXPOSE 7001

ENTRYPOINT ["voce-gateway"]
CMD ["--config", "/etc/voce/config.yaml"]
