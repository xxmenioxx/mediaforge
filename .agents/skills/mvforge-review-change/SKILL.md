---
name: mvforge-review-change
description: Review a scoped MVForge implementation, commit, branch, or task for correctness, regressions, scope drift, missing tests, and OpenSpec compliance without performing broad repository exploration. Use when the user asks to review a completed or in-progress MVForge change.
license: MIT
metadata:
  project: MVForge
  version: "1.0"
---

# MVForge Change Review

This is a review workflow, not an implementation workflow unless the user separately asks for fixes.

## Review scope

Start from the concrete change:
- named task;
- changed files;
- commit;
- branch;
- OpenSpec change;
- verifier failure.

Read applicable `AGENTS.md` and relevant OpenSpec artifacts.

Do not inspect unrelated repository areas.

## Navigation

Use Serena MCP first.

Inspect:
1. changed/target symbols;
2. their direct callers/references when needed;
3. relevant tests;
4. canonical resolver/pipeline they are expected to reuse.

Avoid full-file reads and full repository diffs.

For Git diffs, prefer narrow scoped hunks.

## What to review

Prioritize:

### Correctness
- implementation matches approved observable behavior;
- edge/failure paths fail safely;
- state transitions are valid;
- cancellation/cleanup behavior is correct.

### Architecture
- canonical resolver/pipeline reused;
- backend authority not duplicated in frontend;
- no parallel OCR/FFmpeg/subtitle/publication path;
- requested/effective configuration separation preserved.

### Data and provenance
- source media protected;
- persistence/migrations safe;
- audit/provenance remains explainable;
- stale cached/evidence state cannot silently win.

### Performance
- no obvious N+1 API pattern;
- no repeated expensive probe/OCR/FFmpeg work;
- NAS resource use remains bounded.

### Tests
- changed behavior has focused regression coverage;
- tests exercise failure paths when material;
- tests do not merely snapshot implementation details.

### Scope
- no unrelated rewrites;
- OpenSpec task/checklist claims match actual completion.

## Findings format

Report findings in severity order:

- **Blocking**: correctness, data-loss, safety, contract, or required-validation issue.
- **Important**: likely regression, architecture drift, missing material test, or operational problem.
- **Minor**: cleanup/readability/non-blocking issue.

For each finding include:
- affected symbol/file;
- why it matters;
- minimal recommended fix.

If no material findings exist, say so plainly.

## Validation

Do not rerun broad checks unnecessarily.

If the user asks to validate:

```bash
./scripts/verify.sh --auto
```

Review/investigation-only work does not require commit/push.

## Optional implementation prompt

If the user asks for a Codex fix prompt, produce a compact prompt containing:
- skill to use;
- exact issue/task;
- relevant files/symbols;
- blocking finding(s);
- expected behavior;
- focused tests;
- canonical verifier.

Do not restate all repository-global rules already enforced by `AGENTS.md`.
