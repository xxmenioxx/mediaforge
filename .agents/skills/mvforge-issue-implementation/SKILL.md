---
name: mvforge-issue-implementation
description: Implement a scoped MVForge issue or approved OpenSpec change with minimal code reads, canonical validation, and repo-compliant completion. Use when the user asks Codex to implement a concrete MVForge issue, task, bug fix, or approved change.
license: MIT
metadata:
  project: MVForge
  version: "1.0"
---

# MVForge Issue Implementation

Use this skill for implementation work that has a concrete issue, task, or approved OpenSpec scope.

## Source of truth

Before changing code:

1. Read the applicable `AGENTS.md` files.
2. If an OpenSpec change exists, read its relevant proposal/design/spec/tasks.
3. Treat repository code as the implementation source of truth.
4. Do not treat roadmap or V2 documents as implemented behavior.

Do not duplicate global rules from `AGENTS.md`; follow them as authoritative.

## Investigation

- Use Serena MCP first for code navigation whenever supported.
- Locate the smallest relevant symbols, references, helpers, and tests.
- Avoid whole-file reads and broad repository searches when symbol navigation is sufficient.
- Do not inspect unrelated areas unless a dependency or failing check requires it.
- Preserve existing user changes.

## Implementation

- Stay within the requested issue/change scope.
- Reuse canonical backend resolvers and existing pipelines.
- Do not introduce parallel logic for behavior already owned by a canonical path.
- Preserve source media and provenance.
- Keep parsing, policy, command construction, and execution independently testable.
- Add or update focused tests for changed behavior.
- Preserve backward compatibility unless the approved change explicitly modifies it.

If implementation reveals a requirement outside the approved scope, stop expanding and report it as a follow-up.

## Validation

Run:

```bash
./scripts/verify.sh --auto
```

Use `--full` only when required by `AGENTS.md`, the change scope, or the user.

If verification fails:
- diagnose only the failing check;
- keep diagnostic output bounded;
- fix the scoped cause;
- rerun the canonical verifier.

Completion requires `ALL CHECKS PASSED`.

## OpenSpec tasks

When applicable:
- update only task checkbox(es) completed by this implementation;
- do not mark unrelated or unverified tasks complete.

## Repository completion

Follow the current `AGENTS.md` Git rules exactly.

In particular:
- never commit or push directly to `main`;
- stage only scoped files;
- commit only after required validation passes;
- push the current feature branch when repository rules require it;
- do not create or modify GitHub Issues, PRs, Releases, or settings unless explicitly requested.

## Final report

Keep the completion report compact:

- what changed;
- validation result;
- material limitations/warnings;
- commit SHA/message/branch/push result when a repository modification was completed;
- remaining OpenSpec tasks, if any.
