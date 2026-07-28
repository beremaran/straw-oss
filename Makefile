.PHONY: help check clean-room-check conformance protocol-compatibility diagnostic-bundle diagnostic-bundle-check doc-ownership-check docs-check docs-check-external examples-check examples-live fuzz-smoke ha-smoke image-content-check license-check npm-audit profile-smoke public-surface-check quickstart-smoke race release-artifact-check scripts-check security-check state-backup-smoke tls-proxy-check toolchain-check commit dependency-check fmt-check test test-python lint production-deploy-check docs-website dev dev-admin dev-receipts dev-status dev-reset dev-down dev-logs

help: ## List maintained developer commands.
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_.-]+:.*## / {printf "%-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

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

tls-proxy-check: ## Validate the adaptable HAProxy TLS example with an ephemeral certificate.
	./deploy/production/check-tls-proxy.sh

docs-website:
	cd website && npm run build

dependency-check:
	./scripts/verify-dependency-direction.sh

toolchain-check: ## Enforce one exact Go patch across modules, builders, CI, docs, and examples.
	./scripts/verify-toolchain.sh

conformance: ## Validate the versioned conformance fixture contract.
	./scripts/verify-conformance.py

protocol-compatibility: ## Execute previous-tag and current-tag decoder compatibility checks.
	./scripts/verify-protocol-compatibility.sh

public-surface-check: ## Ensure source-declared public surfaces appear in normative docs.
	./scripts/verify-public-surface.py

doc-ownership-check: ## Enforce documentation owners and quarterly review dates.
	./scripts/verify-doc-ownership.py

docs-check: ## Check Markdown structure, terminology, and internal links.
	./scripts/verify-docs.py

docs-check-external: ## Check documentation including live external links.
	./scripts/verify-docs.py --external

examples-check: ## Compile and syntax-check maintained examples.
	go test ./examples/go
	uv run --frozen python -m py_compile examples/python/request.py

scripts-check: ## Syntax-check every maintained shell entry point.
	bash -n scripts/*.sh deploy/local/scripts/*.sh deploy/production/check-tls-proxy.sh deploy/production/ha-smoke.sh examples/*.sh examples/*/*.sh
	sh -n deploy/production/check-compose.sh deploy/production/failure-drill.sh

examples-live: ## Run all maintained examples against an owned disposable stack.
	./examples/live-smoke.sh

race: ## Run the supported Go suite with the race detector.
	go test -race ./...

fuzz-smoke: ## Run bounded parser/state-machine fuzz targets.
	go test ./internal/config -run '^$$' -fuzz FuzzConfigAndSnapshotJSON -fuzztime=3s
	go test ./internal/control -run '^$$' -fuzz FuzzValidateRequest -fuzztime=3s
	go test ./internal/egress -run '^$$' -fuzz FuzzDNSAndDestinationParsers -fuzztime=3s
	go test ./internal/natsx -run '^$$' -fuzz FuzzUnmarshalEnvelope -fuzztime=3s
	go test ./internal/natsx -run '^$$' -fuzz FuzzStreamFrameSequences -fuzztime=3s
	go test ./internal/objectstore -run '^$$' -fuzz FuzzDecodeS3List -fuzztime=3s
	go test ./internal/receipt -run '^$$' -fuzz FuzzDecodeReceiptRecord -fuzztime=3s

security-check: ## Scan the tracked tree for credential-like residue.
	./scripts/verify-secrets.sh

diagnostic-bundle: ## Print an allowlisted, shareable local diagnostic summary.
	./scripts/collect-diagnostics.sh

diagnostic-bundle-check: ## Prove synthetic sensitive values cannot enter the diagnostic summary.
	./scripts/verify-diagnostic-bundle.sh

release-artifact-check: ## Dry-run reproducible binaries, checksums, metadata, and SBOM.
	./scripts/verify-release-artifacts.sh

image-content-check: ## Verify minimal non-root release image metadata and contents.
	./scripts/verify-images.sh

license-check: ## Inventory Go/npm dependency licenses and reject missing notices.
	./scripts/generate-license-inventory.py "$${LICENSE_INVENTORY:-/tmp/straw-dependency-licenses.json}"

npm-audit: ## Reject moderate-or-higher documentation-site dependency vulnerabilities.
	npm --prefix website audit --audit-level=moderate

clean-room-check: ## Prove dependencies resolve without credentials or custom Git configuration.
	./scripts/verify-clean-room.sh

quickstart-smoke: ## Start, call, inspect, and stop the isolated default Compose stack.
	./deploy/local/scripts/quickstart-smoke.sh

profile-smoke: ## Test PROFILE=default|admin|receipts in a namespaced disposable stack.
	./deploy/local/scripts/profile-smoke.sh "$${PROFILE:-default}"

state-backup-smoke: ## Test PROFILE=admin|receipts backup and restore in an owned stack.
	./deploy/local/scripts/state-backup-smoke.sh "$${PROFILE:-admin}"

ha-smoke: ## Test scaling, Control loss, Redis recovery, and graceful worker loss in an owned HA stack.
	./deploy/production/ha-smoke.sh

check: fmt-check test test-python lint dependency-check toolchain-check conformance public-surface-check doc-ownership-check docs-check examples-check scripts-check security-check diagnostic-bundle-check

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

commit:
	opencode run --model llama.cpp/qwen-4b --thinking --title 'Committing changes' --pure --auto 'Commit all changes. If anything fails, stop, and let me know what is wrong'
