FROM golang:1.25-alpine

RUN apk add --no-cache git make

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=${VERSION}" \
    -o /app/control ./cmd/control

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.Version=${VERSION}" \
    -o /app/egress ./cmd/egress
