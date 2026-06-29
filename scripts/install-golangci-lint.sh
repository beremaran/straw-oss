#!/usr/bin/env bash

if [[ "$OSTYPE" == "darwin"* ]]; then
    brew install golangci-lint
else
    curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.12.2
fi