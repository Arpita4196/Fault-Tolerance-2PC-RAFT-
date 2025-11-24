# syntax=docker/dockerfile:1

# -----------------------------------------------------------------------------
#  Dockerfile for Raft-enabled IoT Sensor Monitoring node
#
#  This Dockerfile builds a minimal container that contains a single Raft
#  node capable of participating in leader election and log replication.  The
#  node listens for both Raft RPCs and Ingest RPCs on the port specified by
#  the environment variable NODE_PORT.  Each node uses its own Redis
#  instance (configured via REDIS_ADDR) to persist committed sensor readings.
#
#  To build the image:
#
#      docker build -t iot-raft-node .
#
#  The resulting image exposes the binary at /app/raftnode.
# -----------------------------------------------------------------------------

FROM golang:1.24-alpine AS builder

# Set the working directory inside the build container
WORKDIR /src

# Copy go.mod and go.sum first to leverage Docker layer cache
COPY go.mod .

# Download dependencies
RUN go mod download

# Copy the rest of the source code
COPY . ./

# Build the raft node binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/raftnode ./raftnode

# Final runtime image
FROM alpine:3.18

# Install ca-certificates to allow gRPC over TLS if needed in the future
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /bin/raftnode .

# The node listens on port 6000 by default (configurable via NODE_PORT)
EXPOSE 6000

# Set minimal environment variables to satisfy redis client (can be overridden)
ENV NODE_PORT=6000 \
    NODE_ID=node0 \
    PEERS= \
    REDIS_ADDR=redis:6379

# Execute the raftnode binary
ENTRYPOINT ["/app/raftnode"]