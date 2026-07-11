.PHONY: check commit fmt-check test test-python postgres-migrations-check lint load-smoke production-deploy-check docs-website dev dev-status dev-reset dev-down dev-logs infra-up infra-status infra-reset infra-down infra-clean infra-logs

test:
	go test ./...

test-python:
	uv run --all-packages --frozen python -m unittest discover python/tests

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"; \
	test -z "$$files" || { echo "$$files"; exit 1; }

lint:
	golangci-lint run --max-issues-per-linter 0 --max-same-issues 0

load-smoke:
	go test ./internal/loadtest
	go test ./internal/control -run 'TestDispatcher(ControlNATSEgressRoundTrip|EgressPhaseTiming|RateLimitRetryAfter)|TestRateLimiter(MemoryGuardrailFallback|RedisFailurePolicy)|TestQuotaAdmissionRedisFailurePolicy'
	go test ./internal/egress -run 'Test(EvaluateAssignmentPrecedence|WorkerRejectsAssignmentAtCapacity|WorkerCreditExhaustionAbortsWithoutPublishing|WorkerDownloadCreditGatesResponseData)'
	go test ./internal/natsx -run 'TestStreamValidator'

production-deploy-check:
	./deploy/production/check-compose.sh

docs-website:
	cd website && npm run build

check: fmt-check test test-python lint

postgres-migrations-check:
	./scripts/check-postgres-migrations.sh

dev:
	@./deploy/local/scripts/dev-up.sh

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
