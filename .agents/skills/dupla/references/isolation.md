# Isolation and validation

The orchestrator uses two separate `codex exec --cd <worktree>` processes.
This uses flags supported by the installed Codex CLI; it does not use
`--worktree`, `--dangerously-bypass-approvals-and-sandbox`, or
`--approve-for-me`. Each process is supervised by the parent, has a separate
JSONL log and last-message file, and is launched with `--sandbox workspace-write`.
The only additional writable path is the repository's shared Git common
directory, needed for commits on each assigned branch. Workers must not switch
branches or modify other worktrees; Git's own per-worktree indexes and locking
protect concurrent commits to the two distinct refs.

For each issue, the helper gives unique paths for `GOCACHE`, `GOTMPDIR`,
`MVFORGE_GOTMPDIR`, `MVFORGE_OCR_LIFECYCLE_ROOT`, `TMPDIR`, `TMP`, `TEMP`,
`npm_config_cache`, and `MVFORGE_DUPLA_ROOT`. Worktree-local build outputs stay
in that worktree. Do not point tests at the same SQLite file or mutable fixture.

Tests that bind shared ports, start services, use Docker/FFmpeg integration
fixtures, or mutate shared volumes must run through the serialized lock:

```text
python3 <dupla-helper-path> lock --scope shared-validation -- ./scripts/verify.sh --auto
```

Use the same wrapper for other checks that touch shared services or resources.
The lock is cross-process and repository-wide. Keep isolated unit tests
parallel where their fixtures and temp paths are independent. Do not start two
copies of a shared service or test container just to preserve parallel speed.

Each worker runs focused tests for its changed behavior and the canonical
`./scripts/verify.sh --auto` through the shared lock. If a check fails, it
records the output, fixes only issue-scoped causes, and reruns the relevant
check. A final PASS requires actual successful output, not a summary assertion.
Keep failed logs and partial work for resume.

Review checks the final branch diff sets for overlap and asks Git to calculate
the merge tree without checking it out. Overlap is a review signal, not proof
of a conflict; a clean merge-tree is not proof that behavior composes
correctly.
