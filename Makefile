.PHONY: check commit fmt-check test test-python postgres-migrations-check clickhouse-migrations-check lint load-smoke production-deploy-check docs-website infra-up infra-status infra-reset infra-down infra-clean infra-logs

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

clickhouse-migrations-check:
	./deploy/local/scripts/check-clickhouse-migrations.sh

infra-up:
	@if [ ! -f deploy/local/.dev/mitm-ca/ca.pem ] || [ ! -f deploy/local/.dev/mitm-ca/ca-key.pem ]; then \
		./deploy/local/dev-mitm-ca.sh; \
	fi
	@./deploy/local/scripts/dev-up.sh

infra-status:
	@./deploy/local/scripts/dev-status.sh

infra-reset:
	@./deploy/local/scripts/dev-reset.sh

infra-down:
	docker compose -f deploy/local/docker-compose.yml down

infra-clean:
	docker compose -f deploy/local/docker-compose.yml down -v

infra-logs:
	docker compose -f deploy/local/docker-compose.yml logs -f

commit:
	opencode run --model llama.cpp/qwen-4b --thinking --title 'Committing changes' --pure --auto 'Commit all changes. If anything fails, stop, and let me know what is wrong'
