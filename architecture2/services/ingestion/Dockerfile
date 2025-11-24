# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod/go.sum FIRST (better build caching)
COPY go.mod ./

RUN go mod download

# Copy rest of project
COPY . .

# Build the Raft-enabled ingestion service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ingestion ./services/ingestion

# ---------------------
# Runtime image
# ---------------------
FROM alpine:3.18

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy built binary
COPY --from=builder /bin/ingestion .

# Raft node always listens on NODE_PORT (default 50051)
EXPOSE 50051

# Default env vars (overridable by docker-compose)
ENV NODE_ID=node1 \
    NODE_PORT=50051 \
    PEERS="" \
    REDIS_ADDR=redis1:6379

ENTRYPOINT ["/app/ingestion"]
