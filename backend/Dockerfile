# syntax=docker/dockerfile:1

# ---------- Build stage ----------
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache module downloads separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build both binaries. CGO_ENABLED=0 for static linking (runs in distroless/alpine).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# ---------- Runtime stage ----------
FROM alpine:3.21 AS runtime

# Install CA certs (needed for HTTPS calls to LLM providers) and timezone data.
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Non-root user for the application.
RUN addgroup -S agent && adduser -S -G agent agent

# Workspace directory for the filesystem tool.
RUN mkdir -p /app/workspace && chown agent:agent /app/workspace

COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker

USER agent

EXPOSE 8080

# Default entrypoint is the HTTP server.
# Override CMD with ["./worker"] when running the worker.
ENTRYPOINT ["./server"]
