## 1. Evidence identity and persistence

- [ ] 1.1 Extend the existing `kind=result` artifact with stable Result generation/output revision identity at worker completion and bind it to the frozen Queue execution and active ExecutionPlan; verify matching, missing, and replaced-artifact cases with focused tests, including same-Result #69 CandidateIdentity reuse. Do not add a second Result store, parallel result model, or Validation-owned artifact identity.
- [ ] 1.2 Add append-only ValidationRun and override persistence with ruleset/contract versions, read-only references to the existing Result identity, finding references, and atomic QueueJob summary projections; verify immutability, ordering, and projection consistency with migration and persistence tests.
- [ ] 1.3 Preserve legacy QueueJob and embedded Result reports as unbound history, and require a newly bound run before new-policy handoff; verify legacy outcomes remain visible but cannot authorize auto-publication.

## 2. Rule evaluation and outcome policy

- [ ] 2.1 Implement stable rule keys and versioned expected-versus-actual findings for integrity, duration/timeline, video, audio, subtitles, chapters, attachments/fonts, sidecars, and publication readiness; verify each category and `NOT_APPLICABLE`/unverified behavior with table-driven tests.
- [ ] 2.2 Implement deterministic `PASS`, `PASS_WITH_WARNINGS`, `REVIEW_REQUIRED`, and `FAIL` reduction using severity, blocking/advisory disposition, and actionability; verify boundary combinations and that a score cannot override the outcome.
- [ ] 2.3 Reuse only identity-matched #69 perceptual/frame evidence as supplemental input and keep `report.frameFidelity` a sibling report; verify stale evidence is not relabeled and the sibling report persists.

## 3. Queue lifecycle and Publisher handoff

- [ ] 3.1 Run Validation after the worker commits the produced Result and before publisher readiness; verify Validation retries create new runs for the same evidence and never invoke encoding or mutate media.
- [ ] 3.2 Route changed or replaced output through a new Queue execution and Result identity; verify old Validation runs remain immutable and cannot authorize the new artifact.
- [ ] 3.3 Gate automatic advancement and Publisher against the canonical run plus eligible override, defaulting to clean `PASS`; verify actionable warnings, `REVIEW_REQUIRED`, `FAIL`, and unbound legacy statuses block automatic handoff.
- [ ] 3.4 Implement versioned, reason-required interactive operator overrides that preserve the original outcome and reject evidence identity/integrity failures; server-assign `local_operator` for interactive actions and `system` for automatic/system actions, reject or ignore client actor identity, and keep the actor reference extensible for a future authenticated principal without adding auth infrastructure. Verify actor, timestamp, reason, rule scope, and override version are auditable.

## 4. Review Center, History, and bounded evidence access

- [ ] 4.1 Update the Validation API and Review Center to default to `REVIEW_REQUIRED`, `FAIL`, and actionable warnings, with pending work and other outcomes available through explicit views; verify clean `PASS` is omitted from the default list.
- [ ] 4.2 Add lazy detail loading for full findings and large evidence/logs while list requests use batched bounded summaries; verify list rendering and refresh make no ffprobe, scan, perceptual-analysis, or validation calls.
- [ ] 4.3 Update History to retain outcome, Validation run identity, override presence, and publication state after review closes or publication completes; verify all completed lifecycle outcomes remain discoverable.

## 5. Integration and compatibility verification

- [ ] 5.1 Add focused end-to-end coverage for Result-to-Validation-to-Publisher transitions, report sibling compatibility, and retry/requeue boundaries; verify clean, warning, review, failure, and override flows.
- [ ] 5.2 Run the requested focused checks and `./scripts/verify.sh --auto`; verify the canonical verifier reports `ALL CHECKS PASSED` and document any environment limitation separately.
