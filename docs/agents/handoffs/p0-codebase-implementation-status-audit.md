# Handoff

Task: `claude-p0-codebase-implementation-status-transcripts`

## Changed

- Updated `.claude/skills/straw-task-runner/SKILL.md` with a completion audit that distinguishes library work from
  runtime wiring and requires owned deferrals.
- Updated `AGENTS.md` to allow sub-agent use while keeping one owning agent responsible for final edits and verification.
- Corrected overstated historical records in task 03 and handoffs 05/11.
- Renumbered the P0 test-matrix task from 15 to 25 and added P0 gap tasks 16-24 for live NATS, Postgres, Redis, policy,
  egress execution, and Control dispatch integration.
- Added P1 and P2 task boards and task files covering the remaining planning-doc roadmap.

## Verification

```sh
python3 - <<'PY'
from pathlib import Path
import re, sys
failed=False
for board in [Path('docs/tasks/p0.md'), Path('docs/tasks/p1.md'), Path('docs/tasks/p2.md')]:
    base=board.parent
    missing=[]
    for link in re.findall(r'\]\(([^)]+)\)', board.read_text()):
        if link.startswith(('http','#')):
            continue
        if not (base/link).resolve().exists():
            missing.append(link)
    failed |= bool(missing)
for p in sorted(Path('docs/tasks').glob('p[012]/**/*.md')):
    txt=p.read_text()
    status=re.search(r'^Status: (.+)$', txt, re.M)
    is_open = not status or status.group(1).strip() != 'done'
    checks=[
        'Status:' in txt,
        '- [ ] Read all required planning docs.' in txt or '- [x] Read all required planning docs.' in txt,
        'Run `make check`.' in txt,
        'Write a handoff note.' in txt,
    ]
    if is_open:
        checks.append('Stop if a deferral would have no owning task file.' in txt)
    failed |= not all(checks)
sys.exit(1 if failed else 0)
PY

make check
```

Result:

- Structural board/link check passed.
- `make check` passed: `go test ./...` passed and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`
  reported `0 issues`.
- Completion-audit keyword scan over changed task/handoff files found only intentional documentation of existing
  in-memory/fake/stub/synthetic gaps; no new runtime fake, stub, or TODO implementation was added.

## Reviewer Start Points

- `docs/tasks/p0.md`
- `docs/tasks/p1.md`
- `docs/tasks/p2.md`
- `.claude/skills/straw-task-runner/SKILL.md`

## Remaining Work

- None for this audit/task-writing pass. Implementation work is now represented by the open task files.

## Blockers

- None.
