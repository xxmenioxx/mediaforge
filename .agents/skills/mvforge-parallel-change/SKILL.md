---
name: mvforge-parallel-change
description: Automatically coordinate a scoped MVForge change with multiple Codex subagents whenever there are two or more independent workstreams that can safely run in parallel without overlapping files, symbols, or unresolved design decisions. Prefer this skill for backend + frontend, implementation + tests, runtime audit + documentation, OpenSpec review, or other clearly separable work. Do not use it for small single-stream tasks or when agents would edit the same files.
license: MIT
metadata:
  project: MVForge
  version: "1.1"
---

# MVForge Parallel Change

Use this skill only when the task can be decomposed into independent workstreams.

The root agent owns scope, decomposition, assignment, integration, conflict prevention, canonical validation, OpenSpec task completion, and commit/push according to `AGENTS.md`.

Subagents are workers, not independent project owners.

## Automatic invocation

Use this skill automatically when all of the following are true:

- the task is an MVForge implementation/review/change task;
- there are at least two meaningful independent workstreams;
- those workstreams can be assigned without overlapping file or symbol ownership;
- no unresolved canonical design decision must be shared by multiple agents first.

Typical automatic triggers:

```text
backend + frontend
implementation + focused tests
runtime audit + documentation
independent read-only investigations
OpenSpec compliance + code ownership mapping
```

Do not require the user to explicitly mention `$mvforge-parallel-change` when these conditions are met.

Do not activate this skill for trivial or single-stream work such as:

```text
rename one label
fix one typo
change one constant
edit one isolated function
```

If parallel safety is uncertain, start with read-only investigation subagents and decide after ownership is mapped.

## Model routing policy

MVForge uses a GPT-6 Luna-first escalation policy.

Prefer increasing GPT-6 Luna reasoning effort before changing model family.
Use only model variants and reasoning levels that are actually available in the
current Codex environment; do not invent unavailable model IDs.

```text
GPT-6 Luna Medium
  ↓
GPT-6 Luna High
  ↓
GPT-6 Luna XHigh
  ↓
GPT-6 Sol only if absolutely necessary and available
```

### GPT-6 Luna Medium
Use for read-only repository exploration, file/symbol mapping, locating tests,
documentation inventory, mechanical edits, simple lint/static cleanup, bounded
evidence collection, focused regression tests, small refactors, deterministic
API/UI wiring, and other straightforward scoped work.

### GPT-6 Luna High
This is the default level for normal MVForge implementation work.

Use for scoped feature implementation, backend/frontend changes, routine
OpenSpec implementation, multi-file changes with clear ownership, lifecycle
reconciliation, moderate DB changes, integration of bounded components, and
non-trivial regression analysis.

### GPT-6 Luna XHigh
Use for architecture-heavy SDD, difficult debugging, FFmpeg/QSV/VAAPI/
VideoToolbox/runtime behavior, migration safety with compatibility risk,
canonical Queue/Worker/Runtime pipeline changes, concurrency-sensitive logic,
high-risk state/ownership decisions, and final review of large or risky
changes.

Do not select XHigh merely because a task uses subagents. Complexity and risk,
not agent count, determine reasoning effort.

### GPT-6 Sol escalation
Do not use GPT-6 Sol by default.

Escalate only when GPT-6 Luna XHigh is demonstrably insufficient, such as:

- conflicting architectural evidence that cannot be reconciled;
- repeated focused failures after the failing workstream has been isolated;
- a materially unsafe or ambiguous canonical decision;
- a cross-cutting regression where Luna XHigh cannot identify a reliable path.

Before escalating, the root agent must state the unresolved limitation or risk.
Escalate only the blocked workstream; do not upgrade every agent.

If GPT-6 Sol or the desired reasoning level is unavailable, remain on the
strongest available GPT-6 Luna level and report the limitation instead of
inventing a model configuration.

### GPT-5 fallback / regression comparison

GPT-5-family models are not part of normal routing.

Use them only when:

- GPT-6 shows a concrete reproducible regression that warrants A/B comparison;
- the user explicitly requests a GPT-5 comparison or fallback.

Do not fall back to GPT-5 simply because a task is difficult; increase GPT-6
Luna reasoning effort first.

### Root defaults

```text
Root agent: GPT-6 Luna High
Small/simple root: GPT-6 Luna Medium
Large/high-risk root: GPT-6 Luna XHigh
Read-only/simple subagents: GPT-6 Luna Medium
Normal implementation subagents: GPT-6 Luna Medium
Complex specialist subagents: GPT-6 Luna High
Critical architecture/reviewer subagent: GPT-6 Luna XHigh
```

Use the cheapest capable GPT-6 Luna level first.

## Ground first

Before spawning subagents:

1. Read applicable `AGENTS.md`.
2. Read the relevant OpenSpec change, if one exists.
3. Identify the concrete issue/task scope.
4. Use Serena MCP to identify likely code boundaries.
5. Determine whether parallel work is actually safe.

Do not spawn agents merely because the feature is large.

## Parallelization test

Parallelize only when workstreams have clear ownership.

Good examples:

```text
Backend implementation
Frontend implementation
Focused regression tests
OpenSpec compliance review
Docker/runtime dependency audit
Documentation
```

Bad examples:

```text
Agent A edits AssetsPage.tsx
Agent B also edits AssetsPage.tsx

Agent A refactors canonical resolver
Agent B implements against the old resolver
```

If two workstreams require the same files/symbols or depend on an unresolved design decision, keep them sequential.

## Assign ownership explicitly

Every subagent assignment must include:

- objective;
- exact scope;
- owned files/symbols or domain boundary;
- files/symbols it must not modify;
- relevant OpenSpec requirement/task;
- expected output;
- validation expected from that subagent;
- selected GPT-6 Luna reasoning level and why that level is sufficient.

Example:

```text
Subagent A — Backend

Model:
GPT-6 Luna Medium

Reason:
Scoped backend implementation with known ownership and focused tests.

Objective:
Implement runtime snapshot retention.

Own:
- backend/internal/runtimeinfo/*
- backend/internal/models/runtime_snapshot.go
- focused backend tests

Do not modify:
- frontend/*
- .github/*
- backend/Dockerfile

Return:
- changed symbols/files
- focused test result
- assumptions or blockers
```

## Prefer investigation parallelism before edit parallelism

When boundaries are uncertain, first use subagents for read-only investigation:

```text
Root
├── Agent A: map backend ownership
├── Agent B: map frontend ownership
└── Agent C: identify regression tests / OpenSpec gaps
```

Then the root decides whether editing can safely proceed in parallel.

## Suggested roles

### Implementer
Makes scoped changes inside an owned boundary.

### Reviewer
Reviews existing or newly produced changes. Normally read-only unless the root explicitly assigns fixes.

### Test specialist
Adds focused regression coverage in test files that do not conflict with implementation ownership.

### OpenSpec reviewer
Checks implementation against approved proposal/design/spec/tasks.

### Runtime/media specialist
Use with `$mvforge-runtime-media-change`.

### Subtitle specialist
Use with `$mvforge-subtitle-change`.

## Skill composition

Example:

```text
Root:
$mvforge-parallel-change
$mvforge-issue-implementation

Backend media subagent:
$mvforge-runtime-media-change

Subtitle subagent:
$mvforge-subtitle-change

Review subagent:
$mvforge-review-change
```

Do not repeat all repository-global rules in every assignment. `AGENTS.md` remains authoritative.

## Editing rules

- No two agents may intentionally edit the same file concurrently.
- No two agents may own the same canonical decision.
- Shared interfaces/contracts are changed by one owner first.
- Dependent agents wait until that contract is stable.
- Preserve unrelated user changes.
- Avoid broad formatting/refactors.
- Do not let subagents perform GitHub Issue/PR/Release operations unless explicitly requested.
- Follow current `AGENTS.md` rules for Git operations.

## Integration

The root agent reviews all subagent outputs before final validation.

Checklist:

```text
[ ] scopes did not overlap
[ ] no duplicated resolver/pipeline introduced
[ ] shared contracts agree
[ ] tests reflect final integrated behavior
[ ] OpenSpec requirements remain satisfied
[ ] no subagent silently expanded scope
[ ] no unrelated files changed
```

If subagents produced conflicting approaches, choose one canonical path. Do not merge both just to preserve work.

## Validation

Subagents may run focused checks for their owned work.

The root performs final validation after integration:

```bash
./scripts/verify.sh --auto
```

Use `--full` only when required by `AGENTS.md` or the change scope.

Completion requires:

```text
ALL CHECKS PASSED
```

Separate green subagent checks do not replace integrated verification.

## Commit and push

For one issue/change:

- integrate subagent work first;
- create one scoped task commit unless the approved plan explicitly requires multiple commits;
- push the current feature branch according to `AGENTS.md`;
- never commit/push directly to `main`.

Subagents should not create competing commits unless the root explicitly designed a multi-commit workflow.

## Failure handling

If one subagent fails:

- preserve successful independent work;
- identify whether the failure blocks integration;
- retry only the failed scoped work;
- do not restart unrelated successful agents;
- increase GPT-6 Luna reasoning effort only for the failed/uncertain workstream;
- do not escalate every agent merely because one subagent struggled;
- use GPT-6 Sol only after GPT-6 Luna XHigh is demonstrably insufficient and only when GPT-6 Sol is available.

If a subagent discovers a new requirement outside scope, report it to the root and do not implement it automatically.

## Final report

The root reports:

- workstreams used;
- GPT-6 Luna reasoning levels used for each workstream;
- any escalation and why it was necessary;
- integrated result;
- canonical validation result;
- material warnings/limitations;
- commit SHA/message/branch/push result when applicable;
- remaining tasks/follow-ups.

Do not dump full subagent transcripts.
