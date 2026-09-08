# Codex Prompt — SDD Pilot: Canonical Subtitle ASS

Implement the feature described by:

- `docs/specs/001-canonical-subtitle-ass/spec.md`
- `docs/specs/001-canonical-subtitle-ass/plan.md`
- `docs/specs/001-canonical-subtitle-ass/tasks.md`

Follow `AGENTS.md`.

Treat `spec.md` as the behavioral source of truth. `plan.md` is architectural
guidance, not permission to modify every listed file.

For exploration:

- use Serena MCP semantic tools first;
- do not inspect Codex memory;
- do not use Serena CLI;
- do not perform broad raw reads;
- read only the symbols required to resolve the current task.

Before editing, report only:

1. root cause/current gap;
2. smallest safe implementation;
3. exact symbols/files to modify.

Then implement Tasks 1–8 in order. Keep SRT as the default converted format and
do not create a separate Asset Info ASS conversion path.

Validate only with:

```bash
./scripts/verify.sh --auto
```

If validation fails, follow the repository failure workflow from `AGENTS.md`.

Before finishing, converge the implementation against R1–R10 in `spec.md`.

Keep the final response concise.
