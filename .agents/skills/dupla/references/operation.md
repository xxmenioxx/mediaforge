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

`run` and `resume` include publication after worker supervision. Each
publishable issue gets a normal `git push --set-upstream origin <branch>` and
one PR to `main`, initially Draft. Publication is performed by the parent, not
the implementation workers.

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

Each worker receives one issue only. It reads applicable instructions, stays
in its assigned worktree, implements and tests only that scope, inspects
`git diff --check` and changed files for unexpected changes/secrets, and
returns a JSON summary. Local commits reference that issue. A worker retries
failed validations within scope; if a check still fails, it retains the
evidence and may commit the scoped changes so the parent can publish a Draft
PR that clearly documents the failure. Workers never push or edit GitHub.

Before publication the parent checks branch/worktree identity, issue identity,
commit references, exact changed-file reporting, a clean worktree,
non-empty commits/diff, `git diff --check`, and secret indicators in changed
patch content (including removed lines) and PR text. Any uncommitted/untracked
content, mismatch, potential secret, or empty diff blocks publication without
changing the worktree. A
failed push or `gh` operation is saved in session state; `resume` repeats the
checks and push, queries PRs by exact head branch, and updates the existing
PR's title/body when found instead of creating a duplicate. PR text includes
the implementation summary, `Closes #N`, changed
files, test outcomes, the verifier result, risks, limitations, and pending
work. The parent records the PR URL, Draft/state, and best-effort CI rollup.

Validation failure does not prevent Draft publication when the safety checks
pass. The report remains FAIL/BLOCKED/PARTIAL and the PR body states that the
issue is not complete. Publication success is separate from implementation
success. No empty PR is created.

`review` reports each branch's commits and changed paths and runs a non-checkout
`git merge-tree` conflict analysis. It does not merge or update either branch.
The merge-tree operation may write unreachable Git objects, but does not
change refs, index, or worktree files.

Publication may read PR metadata and CI check rollups with `gh pr list/view`.
It never approves or requests approval, marks a PR ready, enables auto-merge,
merges/squashes, closes an issue, deletes branches/worktrees, or force-pushes.
It uses only the existing `gh` authentication configuration and does not pass
tokens as arguments or to workers.

`cleanup` requires explicit user approval in the conversation and exact
`--confirm-worktree` path arguments. The helper independently refuses active
workers, dirty worktrees, and worktrees containing ignored files. It uses
ordinary `git worktree remove` without force, does not delete branches, and
retains all session logs and summaries.

## Worker summary

Require a final JSON object with `status` (`PASS`, `FAIL`, `BLOCKED`, or
`PARTIAL`), `issue`, `branch`, `worktree`, `summary`, `commits`, `files`,
`tests` (commands and observed outcomes), `verify` (observed result or
`NOT_RUN`), and `pending`. Verify claims against the saved log, output summary,
Git history, and worktree state before reporting them. `files` lists every
changed path so the parent can check it against the committed diff.
