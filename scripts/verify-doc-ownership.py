#!/usr/bin/env python3
import datetime as dt
import json
from pathlib import Path

root = Path(__file__).resolve().parent.parent
docs = root / "docs/public"
manifest = json.loads((docs / "owners.json").read_text())
listed = set(manifest["pages"])
present = {str(path.relative_to(docs)) for path in docs.rglob("*.md")}
if listed != present:
    raise SystemExit(f"documentation ownership mismatch; unowned={sorted(present-listed)}, missing={sorted(listed-present)}")
today = dt.date.today()
cycle = dt.timedelta(days=int(manifest["review_cycle_days"]))
feedback_url = manifest.get("feedback_url", "")
if not feedback_url.startswith("https://github.com/beremaran/straw-oss/issues/new?"):
    raise SystemExit("documentation feedback_url must use the repository's contextual new-issue route")
for page, record in manifest["pages"].items():
    owner = record.get("owner", "")
    reviewed = record.get("reviewed", "")
    tested_commands = record.get("tested_commands", [])
    if not owner:
        raise SystemExit(f"documentation owner is empty: {page}")
    if not tested_commands or any(not isinstance(command, str) or not command.strip() for command in tested_commands):
        raise SystemExit(f"documentation tested-command evidence is empty: {page}")
    if today - dt.date.fromisoformat(reviewed) > cycle:
        raise SystemExit(f"documentation review is stale: {page} ({reviewed})")
