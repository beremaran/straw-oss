# Docker registry settings
REGISTRY ?= ghcr.io
OWNER ?= beremaran
REPO ?= straw

# Versioning from Git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date +'%Y-%m-%dT%H:%M:%SZ')
GO_PACKAGES = $(shell go list -f '{{.Dir}}' ./... | sed 's|^$(CURDIR)|.|' | grep -v '^./web/management/node_modules/')

# Docker compose dev file (host-side only)
DC = docker compose -f docker/docker-compose.dev.yml

# Detect if running inside dev container (no docker compose available)
# Inside the dev container, docker-cli exists but docker compose plugin does not
ifeq ($(shell command -v docker 2>/dev/null && docker compose version >/dev/null 2>&1; echo $$?),0)
  DC_RUN = $(DC) run --rm dev
else
  DC_RUN =
endif

.PHONY: dev-up dev-down dev-shell docker server endpoint web build all test load-test security format lint lint-autofix clean docs docs-serve install-tools web-lint web-format dev-build dev-test dev-lint dev-docs

# ── Docker Compose Dev Environment (host-side) ───────────────────────────────

dev-up:
	$(DC) up -d postgres nats redis

dev-down:
	$(DC) down

dev-shell:
	$(DC) run --rm dev sh

# ── Dev Targets (auto-detect host vs container) ──────────────────────────────

dev-build:
ifeq ($(DC_RUN),)
	go build -ldflags "-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)" -o bin/relay ./cmd/relay
	go build -ldflags "-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)" -o bin/endpoint ./cmd/endpoint
else
	$(DC_RUN) go build -ldflags "-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)" -o bin/relay ./cmd/relay
	$(DC_RUN) go build -ldflags "-w -s -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)" -o bin/endpoint ./cmd/endpoint
endif

dev-test:
ifeq ($(DC_RUN),)
	go test -race -shuffle=on $(GO_PACKAGES)
else
	$(DC_RUN) go test -race -shuffle=on $(GO_PACKAGES)
endif

dev-lint:
ifeq ($(DC_RUN),)
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 $(GO_PACKAGES)
else
	$(DC_RUN) golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 $(GO_PACKAGES)
endif

dev-docs:
ifeq ($(DC_RUN),)
	npx -y @redocly/cli lint api/openapi.yaml
	cp api/openapi.yaml docs/openapi.yaml
	npx -y @redocly/cli build-docs api/openapi.yaml -o docs/api-reference.html
	uvx --with mkdocs-material mkdocs build --strict
else
	$(DC_RUN) npx -y @redocly/cli lint api/openapi.yaml
	$(DC_RUN) cp api/openapi.yaml docs/openapi.yaml
	$(DC_RUN) npx -y @redocly/cli build-docs api/openapi.yaml -o docs/api-reference.html
	$(DC_RUN) uvx --with mkdocs-material mkdocs build --strict
endif

# ── Local Docker Builds (Production) ─────────────────────────────────────────

docker:
	# Build base image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(OWNER)/$(REPO):base \
		-f docker/base.Dockerfile .
	# Tag base image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO):base
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO)/base:$(VERSION)
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO)/base:latest

	# Build relay image using the local base image
	docker build \
		--build-arg BINARY_NAME=relay \
		--build-arg BASE_IMAGE=$(OWNER)/$(REPO):base \
		-t $(OWNER)/$(REPO):relay \
		-f docker/Dockerfile .
	# Tag relay image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO):relay
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO)/relay:$(VERSION)
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO)/relay:latest

	# Build endpoint image using the local base image
	docker build \
		--build-arg BINARY_NAME=endpoint \
		--build-arg BASE_IMAGE=$(OWNER)/$(REPO):base \
		-t $(OWNER)/$(REPO):endpoint \
		-f docker/Dockerfile .
	# Tag endpoint image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO):endpoint
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO)/endpoint:$(VERSION)
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO)/endpoint:latest

	# Build web image
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(OWNER)/$(REPO):web \
		-f web/management/docker/Dockerfile web/management
	# Tag web image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):web $(REGISTRY)/$(OWNER)/$(REPO):web
	docker tag $(OWNER)/$(REPO):web $(REGISTRY)/$(OWNER)/$(REPO)/web:$(VERSION)
	docker tag $(OWNER)/$(REPO):web $(REGISTRY)/$(OWNER)/$(REPO)/web:latest


server:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o bin/relay ./cmd/relay

endpoint:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o bin/endpoint ./cmd/endpoint

build: server endpoint

all: build test docker

test:
	go test -race $(GO_PACKAGES)

load-test: docker
	@./scripts/run-load-test.sh

security:
	$$(go env GOPATH)/bin/govulncheck $(GO_PACKAGES)

format:
	gofmt -w ./

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 $(GO_PACKAGES)

lint-autofix:
	@./scripts/lint-fix-loop.sh

install-tools:
	@./scripts/install-govulncheck.sh
	@./scripts/install-golangci-lint.sh

docs:
	npx -y @redocly/cli lint api/openapi.yaml
	cp api/openapi.yaml docs/openapi.yaml
	npx -y @redocly/cli build-docs api/openapi.yaml -o docs/api-reference.html
	uvx --with mkdocs-material mkdocs build --strict

docs-serve:
	npx -y @redocly/cli build-docs api/openapi.yaml -o docs/api-reference.html
	uvx --with mkdocs-material mkdocs serve

generate-clients:
	npx -y @openapitools/openapi-generator-cli generate -i api/openapi.yaml -g typescript-fetch -o client/typescript
	npx -y @openapitools/openapi-generator-cli generate -i api/openapi.yaml -g go -o client/go

web-install:
	cd web/management && yarn install

web-dev:
	cd web/management && yarn dev

web-build:
	cd web/management && yarn build

web-test:
	cd web/management && yarn test

web-lint:
	cd web/management && yarn lint

web-format:
	cd web/management && yarn format

clean:
	rm -rf bin/ site/ docs/openapi.yaml docs/api-reference.html client/ web/management/dist/
