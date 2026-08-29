# MVForge — Snapshot Trigger Fix Workflow

Base recomendado:

```text
21fde20
```

Objetivo: corregir la semántica de apertura/generación de snapshots sin mezclarlo
con el refactor de Unprocessed Assets.

## Orden

1. `01-explicit-snapshot-triggers.md` — SOL Light
2. revisar commit
3. `02-snapshot-state-ui.md` — SOL Light
4. revisar commit
5. `03-snapshot-loop-regression.md` — SOL Light
6. revisión final

No usar Medium salvo que Codex encuentre una dependencia backend inesperada que
cambie el contrato de snapshot.

Invocación recomendada:

```text
Follow all applicable AGENTS.md files.

Implement:
docs/codex/snapshot-trigger/01-explicit-snapshot-triggers.md

Inspect first, implement, test, and validate.
Do not stop after producing a plan.
```
