REGISTRY ?= ghcr.io
OWNER ?= beremaran
REPO ?= straw

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

.PHONY: build control egress proto test format lint install-tools docker clean dev-up dev-down dev-shell coverage

control:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.Version=$(VERSION)" -o bin/control ./cmd/control

egress:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.Version=$(VERSION)" -o bin/egress ./cmd/egress

build: control egress

proto:
	go tool github.com/bufbuild/buf/cmd/buf generate

test:
	go test -race -tags=integration ./...

format:
	gofmt -w ./

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./...

install-tools:
	@./scripts/install-govulncheck.sh
	@./scripts/install-golangci-lint.sh

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(OWNER)/$(REPO):base -f docker/base.Dockerfile .
	docker build --build-arg BINARY_NAME=control --build-arg BASE_IMAGE=$(OWNER)/$(REPO):base -t $(OWNER)/$(REPO):control -f docker/Dockerfile .
	docker build --build-arg BINARY_NAME=egress --build-arg BASE_IMAGE=$(OWNER)/$(REPO):base -t $(OWNER)/$(REPO):egress -f docker/Dockerfile .

dev-up:
	docker compose -f docker/docker-compose.dev.yml up -d

dev-down:
	docker compose -f docker/docker-compose.dev.yml down

dev-shell:
	docker compose -f docker/docker-compose.dev.yml run --rm dev sh

clean:
	rm -rf bin/

coverage:
	go test -race -tags=integration -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total
