# Docker registry settings
REGISTRY ?= ghcr.io
OWNER ?= beremaran
REPO ?= straw

# Versioning from Git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date +'%Y-%m-%dT%H:%M:%SZ')

.PHONY: docker server endpoint web build all test load-test security format lint lint-autofix clean docs docs-serve install-tools

docker:
	# Build base image
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(OWNER)/$(REPO):base \
		-f .docker/base.Dockerfile .
	# Tag base image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO):base
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO)/base:$(VERSION)
	docker tag $(OWNER)/$(REPO):base $(REGISTRY)/$(OWNER)/$(REPO)/base:latest

	# Build relay image using the local base image
	docker build \
		--build-arg BINARY_NAME=relay \
		--build-arg BASE_IMAGE=$(OWNER)/$(REPO):base \
		-t $(OWNER)/$(REPO):relay \
		-f .docker/Dockerfile .
	# Tag relay image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO):relay
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO)/relay:$(VERSION)
	docker tag $(OWNER)/$(REPO):relay $(REGISTRY)/$(OWNER)/$(REPO)/relay:latest

	# Build endpoint image using the local base image
	docker build \
		--build-arg BINARY_NAME=endpoint \
		--build-arg BASE_IMAGE=$(OWNER)/$(REPO):base \
		-t $(OWNER)/$(REPO):endpoint \
		-f .docker/Dockerfile .
	# Tag endpoint image for local and GHCR formats
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO):endpoint
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO)/endpoint:$(VERSION)
	docker tag $(OWNER)/$(REPO):endpoint $(REGISTRY)/$(OWNER)/$(REPO)/endpoint:latest

	# Build web image
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(OWNER)/$(REPO):web \
		-f web/management/Dockerfile web/management
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
	go test -race ./...

load-test: docker
	@./scripts/run-load-test.sh

security:
	$$(go env GOPATH)/bin/govulncheck ./...

format:
	gofmt -w ./

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./...

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
	cd web/management && npm install

web-dev:
	cd web/management && npm run dev

web-build:
	cd web/management && npm run build

web-test:
	cd web/management && npm run test

clean:
	rm -rf bin/ site/ docs/openapi.yaml docs/api-reference.html client/ web/management/dist/

