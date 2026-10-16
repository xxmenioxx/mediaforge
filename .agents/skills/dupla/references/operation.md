# Operation and safety contract

## Commands

Run the helper with Python 3 and pass issue numbers without `#`:

```text
python3 .agents/skills/dupla/scripts/dupla.py plan 323 324
python3 .agents/skills/dupla/scripts/dupla.py prepare 323 324 --parallel-safe
python3 .agents/skills/dupla/scripts/dupla.py run 323 324
python3 .agents/skills/dupla/scripts/dupla.py status
python3 .agents/skills/dupla/scripts/dupla.py review 323 324
python3 .agents/skills/dupla/scripts/dupla.py resume 323 324
python3 .agents/skills/dupla/scripts/dupla.py cleanup 323 324
```

`plan` resolves the checkout through Git, reads both Issues with `gh issue
view`, and captures status, branches, worktrees, and the local `main` SHA. It
does not create state, branches, worktrees, or other Git changes. The calling
Codex agent must then inspect the actual requirements and relevant code; the
script does not guess compatibility from issue titles.

`prepare` requires the caller to have completed that compatibility analysis
and pass `--parallel-safe`. It rechecks the base ref and every target before
creating either worktree. Worktrees are named `<repo>-issue-<number>` beside the
primary checkout. Branches use the repo's `fix/<number>-...` or
`feat/<number>-...` style based on issue labels. Existing worktrees are reused
only when their Git common directory, issue-number branch association, and
path are unambiguous. A collision or partial creation is reported and kept for
recovery; nothing is rolled back automatically.

`run` persists its session and per-worker process/log/summary paths under the
Git common directory's `dupla/` metadata folder. It starts both Codex processes
before waiting, records exit codes, continues to collect the second result if
the first fails, and does not detach them. `resume` reuses their exact
worktrees, keeps dirty and uncommitted files, leaves a provably live worker
alone, and starts a new supervised Codex process only for unfinished workers.
After a parent interruption, use `status` to check process identity; if the
process cannot be verified, report its state as unknown and do not claim it is
running.

Each worker receives one issue only. It must read applicable instructions,
stay in its assigned worktree, implement and test only that scope, and return a
JSON summary. It may make a local commit after required checks pass. It must
not push, open a PR, merge, alter issues, or remove worktrees/branches.

`review` reports each branch's commits and changed paths and runs a non-checkout
`git merge-tree` conflict analysis. It does not merge or update either branch.
The merge-tree operation may write unreachable Git objects, but does not
change refs, index, or worktree files.

`cleanup` requires explicit user approval in the conversation and exact
`--confirm-worktree` path arguments. The helper independently refuses active
workers, dirty worktrees, and worktrees containing ignored files. It uses
ordinary `git worktree remove` without force, does not delete branches, and
retains all session logs and summaries.

## Worker summary

Require a final JSON object with `status` (`PASS`, `FAIL`, `BLOCKED`, or
`PARTIAL`), `issue`, `branch`, `worktree`, `commits`, `files`,
`tests` (commands and observed outcomes), `verify` (observed result or
`NOT_RUN`), and `pending`. Verify claims against the saved log, output summary,
Git history, and worktree state before reporting them.
