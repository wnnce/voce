# syntax=docker/dockerfile:1

# ─── Stage 1: Build Frontend ──────────────────────────────────────────────────
FROM node:24-bookworm-slim AS frontend-builder
WORKDIR /app
COPY clients/web/package.json ./clients/web/
RUN npm install -g pnpm && cd clients/web && pnpm install
COPY clients/web/ ./clients/web/
RUN cd clients/web && pnpm build

# ─── Stage 2: Build Backend ───────────────────────────────────────────────────
FROM golang:1.25-bookworm AS backend-builder

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
RUN git lfs install && make deps

# Embed frontend and build the backend binaries
COPY --from=frontend-builder /app/clients/web/dist ./clients/web/dist
RUN GOOS=linux go build -v -ldflags="-w -s" -o voce ./cmd/voce
RUN GOOS=linux go build -v -ldflags="-w -s" -o voce-gateway ./cmd/gateway

# ─── Stage 3: Common Runtime Base ─────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime-base

# Install runtime shared libraries
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    libswresample4 \
    libavutil57 \
    libc++1 \
    libc++abi1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy ten-vad shared library and update system linker cache
COPY --from=backend-builder /app/libs/ten-vad/lib/Linux/x64/libten_vad.so /usr/local/lib/
RUN ldconfig

# Copy required ONNX model file for ten-vad (it expects it in src/onnx_model relative to CWD)
RUN mkdir -p src/onnx_model
COPY --from=backend-builder /app/libs/ten-vad/src/onnx_model/ten-vad.onnx ./src/onnx_model/

# Environment variables for execution
ENV LD_LIBRARY_PATH=/usr/local/lib
COPY config.yaml.example ./config.yaml

# ─── Stage 4: Voce Runtime Image ──────────────────────────────────────────────
FROM runtime-base AS voce-runtime

COPY --from=backend-builder /app/voce ./voce
EXPOSE 7002
EXPOSE 7003

ENTRYPOINT ["./voce"]
CMD ["-c", "/app/config.yaml"]

# ─── Stage 5: Gateway Runtime Image ───────────────────────────────────────────
FROM runtime-base AS gateway-runtime

COPY --from=backend-builder /app/voce-gateway ./voce-gateway
EXPOSE 7001

ENTRYPOINT ["./voce-gateway"]
CMD ["-c", "/app/config.yaml"]
