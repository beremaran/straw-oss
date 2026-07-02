.PHONY: check commit fmt-check test postgres-migrations-check

test:
	go test ./...

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"; \
	test -z "$$files" || { echo "$$files"; exit 1; }

check: fmt-check test

postgres-migrations-check:
	./scripts/check-postgres-migrations.sh

commit:
	opencode run --model llama.cpp/qwen-35b --thinking --title 'Committing changes' --pure --auto 'Commit all changes.'
