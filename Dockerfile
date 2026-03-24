# Stage 1: Build Go binary
FROM golang:1.26 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /symphony ./cmd/symphony

# Stage 2: Minimal runtime with git
FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /symphony /usr/local/bin/symphony

# Claude CLI must be installed separately in the runtime environment
# or mounted from the host. Symphony uses `claude -p --output-format json`.

EXPOSE 9097
ENTRYPOINT ["symphony"]
