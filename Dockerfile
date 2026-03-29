# ─── Stage 1: Build Frontend ──────────────────────────────────────────────────
FROM node:24-alpine AS frontend-builder

WORKDIR /app

COPY clients/web/package.json ./clients/web/

RUN npm install -g pnpm && \
    cd clients/web && \
    pnpm install

COPY clients/web/ ./clients/web/
RUN cd clients/web && pnpm build

# ─── Stage 2: Build Backend ───────────────────────────────────────────────────
FROM golang:1.25-alpine AS backend-builder

RUN apk add --no-cache build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY --from=frontend-builder /app/clients/web/dist ./clients/web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o voce ./cmd/voce

# ─── Stage 3: Final Production Image ──────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata mailcap

WORKDIR /app

COPY --from=backend-builder /app/voce .

COPY config.yaml.example ./config.yaml

EXPOSE 7001
EXPOSE 7002

ENTRYPOINT ["./voce"]
CMD ["-c", "/app/config.yaml"]
