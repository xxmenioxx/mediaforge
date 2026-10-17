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

Workers do not receive `GH_TOKEN`, `GITHUB_TOKEN`, or
`GH_ENTERPRISE_TOKEN`. The parent uses the user's existing `gh` authentication
for read-only issue/PR queries and the explicitly authorized push and Draft PR
create/update. Credentials are not passed in process arguments, prompts, logs,
or PR bodies. Never invoke approval, ready, auto-merge, merge, issue-close,
branch/worktree-delete, or force-push operations.

For each issue and attempt, the helper gives unique paths for `GOCACHE`,
`GOTMPDIR`, `MVFORGE_GOTMPDIR`, `MVFORGE_OCR_LIFECYCLE_ROOT`,
`MVFORGE_OCR_PERSISTENT_ROOT`, `TMPDIR`, `TMP`, `TEMP`, `npm_config_cache`, and
`MVFORGE_DUPLA_ROOT`. OCR persistent snapshots live under the attempt's
Git-metadata resource directory (`ocr-persistent`), outside checked-out
worktree files. The helper overwrites any inherited shell value for this
variable, so concurrent workers and retries cannot silently share a snapshot
root. Worktree-local build outputs stay in that worktree. Do not point tests at
the same SQLite file or mutable fixture.

OCR tests that need independent state must set `MVFORGE_OCR_PERSISTENT_ROOT` to
a test-owned temporary directory (for table cases, one directory per subtest),
for example with `t.Setenv` and `t.TempDir()`. This keeps those tests
deterministic even when they exercise persistent snapshots and prevents
generated OCR state from falling back to a directory in the source checkout.

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
tests and verifier. Before commit it checks the complete diff, `git diff
--check`, and changed lines for potential secrets. A final PASS requires actual
successful output, not a summary assertion. If checks still fail, preserve logs
and partial work; the parent may publish a clearly marked Draft PR after its
own safety checks, while reporting the issue as failed.

The parent serializes publication per issue. It requires a clean worktree,
issue-tagged commits, a non-empty committed diff, a matching worker file list,
and a clean `git diff --check`. It scans changed patch content and generated PR
text before pushing. Tests for publisher idempotence use a local bare remote and a
fake `gh`; never point these fixtures at GitHub or a real remote.

Review checks the final branch diff sets for overlap and asks Git to calculate
the merge tree without checking it out. Overlap is a review signal, not proof
of a conflict; a clean merge-tree is not proof that behavior composes
correctly.
