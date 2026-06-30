FROM golang:1.25-alpine AS base

# Install system dependencies
RUN apk add --no-cache \
    git \
    make \
    curl \
    ca-certificates \
    nodejs \
    npm \
    py3-pip \
    python3 \
    build-base \
    docker-cli

# Install Node.js tools
RUN npm install -g \
    @redocly/cli@latest \
    @openapitools/openapi-generator-cli@latest

# Install Python docs tools
RUN pip install --no-cache-dir --break-system-packages \
    mkdocs-material \
    mkdocs-minify-plugin

# Install Go tools
RUN go install golang.org/x/vuln/cmd/govulncheck@latest
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

# Install uv for running Python tools
RUN pip install --no-cache-dir --break-system-packages uv

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
