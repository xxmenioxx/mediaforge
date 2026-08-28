# MVForge Codex compact workflow

## Repository placement

Copy:

```text
AGENTS.md
```

to the repository root, replacing the current root file with this merged
version after reviewing the diff.

Place task files under:

```text
docs/codex/unprocessed-assets/
```

Recommended order:

1. `01-scope-configuration.md` — SOL Light
2. `02-selection-missing.md` — SOL Light
3. `03-batch-effective-config.md` — SOL Light
4. `04-queue-selected-plan-only.md` — Medium, PLAN ONLY
5. `04-queue-selected-implement.md` — SOL Light with approved plan pasted in
6. `05-configure-selected-batch.md` — SOL Light

After each implementation task:
- review the commit before starting the next task;
- fix only concrete review findings;
- keep one concern per commit.

Suggested Codex invocation:

```text
Follow all applicable AGENTS.md files.

Implement:
docs/codex/unprocessed-assets/01-scope-configuration.md

Inspect first, implement, test, and validate.
Do not stop after producing a plan.
```

For Task 4, use the PLAN ONLY file with Medium first, review the plan, then paste
the approved plan into the implementation file and run that with SOL Light.
