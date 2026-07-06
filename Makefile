.PHONY: check commit fmt-check test postgres-migrations-check lint load-smoke

test:
	go test ./...

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

check: fmt-check test lint

postgres-migrations-check:
	./scripts/check-postgres-migrations.sh

commit:
	opencode run --model llama.cpp/qwen-4b --thinking --title 'Committing changes' --pure --auto 'Commit all changes. If anything fails, stop, and let me know what is wrong'
