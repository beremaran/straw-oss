# Handoff and Review

Every task handoff should state:

- task file;
- summary of code and docs changed;
- verification command and result;
- known gaps or blocked follow-up;
- files reviewers should inspect first.

Reviewers should check:

- the task stayed inside P0 scope;
- behavior matches the named planning docs;
- tests prove the behavior, not just package compilation;
- secrets, auth material, target URLs, and headers are redacted where required;
- `make check` was run after the final edit.
