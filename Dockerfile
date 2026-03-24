# Stage 1: Build Go binary
FROM golang:1.26 AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /symphony ./cmd/symphony

# Stage 2: Runtime with Node.js for Claude Code sidecar
FROM node:22-slim
RUN npm install -g tsx

WORKDIR /app
COPY --from=go-builder /symphony /usr/local/bin/symphony
COPY sidecar/ ./sidecar/
RUN cd sidecar/claude && npm install --production 2>/dev/null || true

# Install git for workspace operations
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

EXPOSE 9097
ENTRYPOINT ["symphony"]
