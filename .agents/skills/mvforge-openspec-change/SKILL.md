---
name: mvforge-openspec-change
description: Create or refine an MVForge OpenSpec/SDD change before implementation. Use when a feature or refactor needs proposal, design, requirements, tasks, migration, or architecture decisions captured in OpenSpec.
license: MIT
metadata:
  project: MVForge
  version: "1.0"
---

# MVForge OpenSpec Change

Use this skill to define a change before implementation.

This skill creates or updates OpenSpec artifacts. It does not implement production code unless the user separately requests implementation after the change is approved.

## Grounding

1. Read applicable `AGENTS.md`.
2. Read `openspec/config.yaml`.
3. Run:

```bash
openspec list --json
openspec list --specs
```

4. Inspect only the relevant existing specs and source symbols needed to understand current behavior.
5. Use Serena MCP for code investigation whenever available.

Do not invent current behavior from roadmap documents.

## Existing change

If the change already exists:

```bash
openspec status --change "<name>" --json
```

Read the existing artifacts before editing them.

## New change

Create the change through the CLI:

```bash
openspec new change "<name>"
```

Never manually scaffold an `openspec/changes/<name>` directory.

Then inspect status:

```bash
openspec status --change "<name>" --json
```

For each required artifact, use:

```bash
openspec instructions "<artifact-id>" --change "<name>" --json
```

Follow the returned template and instructions.

## MVForge design checks

Capture, when relevant:

- canonical resolver/pipeline reuse;
- backend authority vs frontend presentation;
- SQLite/data migration impact;
- NAS CPU/RAM/storage impact;
- FFmpeg/ffprobe/OCR duplication risk;
- source media/provenance safety;
- cancellation and atomic publication;
- backward compatibility;
- failure behavior;
- observability/provenance;
- configuration inheritance;
- testability.

## Artifact expectations

### Proposal
Define:
- problem;
- goal;
- scope;
- non-goals;
- affected capabilities.

### Specs
Describe observable behavior and scenarios.
Include failure and compatibility cases when relevant.
Avoid implementation details unless required by the contract.

### Design
Explain:
- integration points;
- canonical owner of each decision;
- data flow/state transitions;
- migrations;
- performance/resource implications;
- tradeoffs and rejected alternatives.

### Tasks
- order by dependency;
- keep tasks independently verifiable;
- include focused tests;
- include canonical repository verification;
- avoid vague tasks such as "finish implementation".

## Validation

Validate the change using the project OpenSpec workflow.

Do not implement application code as part of this skill unless the user explicitly transitions to implementation.

## Final report

Report:
- change name;
- artifacts created/updated;
- key decisions;
- unresolved decisions;
- validation status.
