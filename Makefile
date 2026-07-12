.PHONY: check commit fmt-check test test-python lint production-deploy-check docs-website dev dev-admin dev-receipts dev-status dev-reset dev-down dev-logs infra-up infra-status infra-reset infra-down infra-clean infra-logs

test:
	go test ./...

test-python:
	uv run --frozen python -m unittest discover integration/python

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"; \
	test -z "$$files" || { echo "$$files"; exit 1; }

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0

production-deploy-check:
	./deploy/production/check-compose.sh

docs-website:
	cd website && npm run build

check: fmt-check test test-python lint

dev:
	@./deploy/local/scripts/dev-up.sh

dev-admin:
	docker compose -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.runtime-admin.yml up -d --build

dev-receipts:
	docker compose -f deploy/local/docker-compose.yml -f deploy/local/docker-compose.object-storage.yml up -d --build

dev-status:
	@./deploy/local/scripts/dev-status.sh

dev-reset:
	@./deploy/local/scripts/dev-reset.sh

dev-down:
	docker compose -f deploy/local/docker-compose.yml down

dev-logs:
	docker compose -f deploy/local/docker-compose.yml logs -f

# Backward-compatible aliases for contributors with older scripts.
infra-up: dev
infra-status: dev-status
infra-reset: dev-reset
infra-down: dev-down
infra-clean: dev-reset
infra-logs: dev-logs

commit:
	opencode run --model llama.cpp/qwen-4b --thinking --title 'Committing changes' --pure --auto 'Commit all changes. If anything fails, stop, and let me know what is wrong'
