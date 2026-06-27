FROM golang:1.25-alpine

RUN apk add --no-cache git make

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/relay ./cmd/relay-server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/endpoint ./cmd/endpoint