## 1. Canonical evaluator and policy

- [x] 1.1 Add complete versioned resolved-profile fingerprinting and candidate/evidence identity validation.
- [x] 1.2 Add the pure per-candidate qualification evaluator, intent-specific MVForge VMAF defaults, same-window artifact regression adapter, stable rules, worst-window selection, and #53 outcome aggregation.
- [x] 1.3 Add focused backend policy tests for staleness, source-versus-candidate artifact severity, missing/unsupported evidence, every outcome, and deterministic ordering.

## 2. Immutable persistence and Test Encode integration

- [x] 2.1 Add narrow immutable qualification model, indexes, idempotent migration triggers, and Test Encode profile/source identity snapshots without historical backfill.
- [x] 2.2 Persist qualification from existing Test Encode evidence and expose batched qualification summaries/details through the existing Test Encode APIs.
- [x] 2.3 Add migration, lifecycle, identity, and historical-unqualified backend tests.

## 3. LAB integration

- [x] 3.1 Add outcome and stale/unqualified status to the existing Test Encode inventory in LAB.
- [x] 3.2 Add a focused qualification details view with version, rule explanations, provenance, and worst frozen evidence window using the existing evidence presentation.
- [x] 3.3 Add focused frontend tests for visible rules, provenance, and worst-window navigation without creating a Test Encode.

## 4. Queue Batch Preflight integration

- [x] 4.1 Load only exact current asset/source/profile/policy matches with bounded batch queries and present all outcomes as non-blocking review evidence.
- [x] 4.2 Freeze the exact reviewed qualification record and provenance into the accepted `ExecutionPlan`; keep Worker execution read-only.
- [x] 4.3 Add backend and frontend tests for unqualified warning, explicit outcomes, frozen acceptance provenance, and no Worker recomputation.

## 5. Boundary and validation

- [x] 5.1 Keep #53 Validation independent and test that a qualified candidate cannot bypass a final-output failure.
- [x] 5.2 Run focused backend/frontend checks, `npm run lint`, `npm run build`, `openspec validate profile-qualification-policy --strict`, `./scripts/verify.sh --auto`, and `./scripts/verify.sh --full`.
- [x] 5.3 Review scoped changes, commit, and push `feat/142-profile-qualification`.
