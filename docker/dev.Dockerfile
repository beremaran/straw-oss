FROM golang:1.25-alpine

RUN apk add --no-cache \
    ca-certificates \
    curl \
    docker-cli \
    git \
    make

RUN go install golang.org/x/vuln/cmd/govulncheck@latest
RUN go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
