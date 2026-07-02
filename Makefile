.PHONY: check commit fmt-check test

test:
	go test ./...

fmt-check:
	@files="$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))"; \
	test -z "$$files" || { echo "$$files"; exit 1; }

check: fmt-check test

commit:
	opencode run --model llama.cpp/qwen-35b --thinking --title 'Committing changes' --pure --auto 'Commit all changes.'
