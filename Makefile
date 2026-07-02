.PHONY: commit

commit:
	opencode run --model llama.cpp/qwen-35b --thinking --title 'Committing changes' --pure --auto 'Commit all changes.'
