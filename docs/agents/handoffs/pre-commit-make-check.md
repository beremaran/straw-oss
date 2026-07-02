# Handoff

Task: `Ensure make check runs on commit, via pre-commit.`

## Changed

- Added [`.pre-commit-config.yaml`](/Users/beremaran/projects/straw/.pre-commit-config.yaml) with a local `make check` hook that runs on `pre-commit`.

## Verification

```sh
make check
```

Result:

- Passed.

## Reviewer Start Points

- [`.pre-commit-config.yaml`](/Users/beremaran/projects/straw/.pre-commit-config.yaml)

## Remaining Work

- None.

## Blockers

- None.
