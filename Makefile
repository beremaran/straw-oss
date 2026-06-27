.PHONY: docker server endpoint build all test load-test security format lint clean

docker:
	docker build -t beremaran/straw:base -f .docker/base.Dockerfile .
	docker build -t beremaran/straw:relay -f .docker/Dockerfile --build-arg BINARY_NAME=relay .
	docker build -t beremaran/straw:endpoint -f .docker/Dockerfile --build-arg BINARY_NAME=endpoint .

server:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o bin/relay ./cmd/relay-server

endpoint:
	CGO_ENABLED=0 go build -ldflags "-w -s" -o bin/endpoint ./cmd/endpoint

build: server endpoint

all: build test docker

test:
	go test -race ./...

load-test: docker
	@./scripts/run-load-test.sh

security:
	@./scripts/install-govulncheck.sh
	govulncheck ./...

format:
	gofmt -w ./

lint:
	@./scripts/install-golangci-lint.sh
	golangci-lint run ./...

clean:
	rm -rf bin/