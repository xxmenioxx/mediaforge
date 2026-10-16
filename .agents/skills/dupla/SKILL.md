---
name: dupla
description: Implement and validate two MediaForge GitHub Issues in independent worktrees, publish one Draft PR per issue, and safely inspect, resume, or clean up the pair.
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
   `codex exec` processes, each with its own worktree, logs, summary, and
   temporary Go/test directories. Workers test, run `verify.sh --auto`, inspect
   changes, and commit only their issue. The parent then checks the committed
   diff and secrets, pushes each branch normally to `origin`, and creates or
   updates one Draft PR targeting `main`. A failed validation can be published
   as Draft only when its failure is documented; it remains a failed issue.
3. Run `scripts/dupla.py review A B` and inspect both changes and integration
   conflicts. Report issue result, branch, worktree, commits, tests, verifier,
   publication state, and both PR URLs. A clean merge-tree or worker PASS claim
   alone is not a correctness review or validation evidence.

Read [references/operation.md](references/operation.md) for command behavior and
recovery. Read [references/isolation.md](references/isolation.md) before
validation or cleanup. The executable contract and fixture tests are in
`scripts/dupla.py` and `scripts/test_dupla.py`.

`resume` also retries a failed push or PR operation: it queries by exact branch
before creating anything and edits an existing PR instead of duplicating it.
GitHub authentication comes from the user's existing `gh` configuration; do
not pass or copy credentials. Never approve/request approval, mark a PR ready,
enable auto-merge, merge/squash, close an issue, force-push, or delete branches
or worktrees. `cleanup` only previews by default. Ask the user before removal,
then pass the exact approved worktree paths with `--execute --confirm-worktree
PATH`. Dirty worktrees, active workers, branches, logs, and state are preserved.
