# Build stage
FROM golang:1.25-alpine AS builder

# Install git and make (sometimes needed for dependencies or build metadata)
RUN apk add --no-cache git make

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build arguments
ARG TARGET_BINARY=relay-server
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build the binary
# We use -w -s to strip debug information and reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/binary ./cmd/${TARGET_BINARY}

# Runtime stage
# distroless/static is minimal and contains only CA certificates and timezone data
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/binary /app/server

# Run as non-root user (distroless provides a 'nonroot' user)
USER nonroot:nonroot

# Entrypoint
ENTRYPOINT ["/app/server"]
