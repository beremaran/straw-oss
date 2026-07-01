REGISTRY ?= ghcr.io
OWNER ?= beremaran
REPO ?= straw

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: build server endpoint test format lint install-tools docker clean dev-up dev-down dev-shell

server:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)" -o bin/relay ./cmd/relay

endpoint:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.Version=$(VERSION)" -o bin/endpoint ./cmd/endpoint

build: server endpoint

test:
	go test -race ./...

format:
	gofmt -w ./

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./...

install-tools:
	@./scripts/install-govulncheck.sh
	@./scripts/install-golangci-lint.sh

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) -t $(OWNER)/$(REPO):base -f docker/base.Dockerfile .
	docker build --build-arg BINARY_NAME=relay --build-arg BASE_IMAGE=$(OWNER)/$(REPO):base -t $(OWNER)/$(REPO):relay -f docker/Dockerfile .
	docker build --build-arg BINARY_NAME=endpoint --build-arg BASE_IMAGE=$(OWNER)/$(REPO):base -t $(OWNER)/$(REPO):endpoint -f docker/Dockerfile .

dev-up:
	docker compose -f docker/docker-compose.dev.yml up -d

dev-down:
	docker compose -f docker/docker-compose.dev.yml down

dev-shell:
	docker compose -f docker/docker-compose.dev.yml run --rm dev sh

clean:
	rm -rf bin/
