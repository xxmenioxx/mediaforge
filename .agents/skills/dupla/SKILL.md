---
name: dupla
description: Prepare, run, inspect, resume, review, or safely clean up two MediaForge GitHub Issues in independent Git worktrees.
metadata:
  short-description: Coordinate two isolated MediaForge issues
---

# Dupla

Use for an explicit `$dupla` request involving two GitHub issue numbers. The
workflow supports `#A #B`, `plan #A #B`, `status`, `review #A #B`,
`resume #A #B`, and `cleanup #A #B`.

Resolve the repository from Git. Before creating worktrees or starting workers,
run `scripts/dupla.py plan A B`, read the repository's applicable `AGENTS.md`
files, inspect both issues and relevant code, then check dependencies and
overlapping files. Use Serena for supported code navigation. If either scope is
unknown or the issues depend on each other, do not claim parallel safety:
recommend sequential handling or report the blocker. `plan` is read-only with
respect to Git.

For an execution request, only after that review:

1. Run `scripts/dupla.py prepare A B --parallel-safe`. It uses the same local
   `main` commit for both sibling worktrees and refuses unsafe path/branch
   collisions. It never edits the primary checkout.
2. Run `scripts/dupla.py run A B`. This starts at most two supervised
   `codex exec` processes, each with its own worktree, log, summary file, and
   temporary Go/test directories. Workers may commit locally after their
   checks pass; pushes, PRs, merges, issue changes, and deletion remain
   unauthorized.
3. Run `scripts/dupla.py review A B` and report each worker's observed status,
   branch, worktree, commits, changed files, commands and evidence, plus
   integration conflicts. Inspect the actual changed code and tests against
   both issue scopes; a clean merge-tree or a worker's PASS claim is not a
   correctness review or validation evidence.

Read [references/operation.md](references/operation.md) for command behavior and
recovery. Read [references/isolation.md](references/isolation.md) before
validation or cleanup. The executable contract and fixture tests are in
`scripts/dupla.py` and `scripts/test_dupla.py`.

`cleanup` only previews by default. Ask the user before removal, then pass the
exact approved worktree paths with `--execute --confirm-worktree PATH`. Dirty
worktrees, active workers, branches, logs, and state are preserved.
