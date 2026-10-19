## Context and evidence authority

The current backend has one canonical Result artifact with generation and output revision, frozen binding and ExecutionPlan fingerprint; ValidationRun is append-only and QueueJob stores only its active run reference/summary. The normal evaluator uses QueueJob.TrackProfileSnapshot for audio/subtitle/attachments, planned sidecars and manifest expectations. Publisher's canonicalRunAllowsPublish validates the active run and output revision, but publisher.go also checks validateRequiredSubtitleArtifacts(job) using the original frozen job. Fixing only the evaluator would leave this second gate inconsistent, or worse, would risk authorizing the wrong package.

The prior read-only incident report identified three historical blocked outputs. Do not commit their private titles, full paths, IDs, media content, raw logs, or fingerprints to this public repository. Input evidence remains unverified for recovery until a separately approved, bounded production preview.

## Task 1.3: modeled authority and evidence gaps

The current schemas bind a historical execution and its output, but they do not independently preserve the original per-asset track intent needed to repair a crossed `TrackProfileSnapshot`:

| Record/field | What it can establish | Authority for corrected track expectations |
| --- | --- | --- |
| `QueueJob.TrackProfileSnapshot` | Frozen resolved track actions, `resolvedForAsset`, and track-plan data consumed by normal Validation/Publisher | Not independent when `resolvedForAsset` identifies another source; this is the contaminated input under review. |
| `QueueJob.ProfileResolution.tracks` | Selection source/key and embedded `profileData` | Not sufficient for the affected pre-fix selected-assets path: `resolveProfileAssignmentsFromEffective` retained a shared mutable `ProfileData` map, and track resolution mutates maps in place before Queue persistence. The frozen Queue fingerprint proves which bytes were persisted, not that they represent this asset's original intent. `TrackProfileKey` alone also has no immutable profile revision. |
| `QueueJob.SubtitleArtifacts` | Planned/observed sidecar artifact IDs, paths and lifecycle status | Can verify artifact presence/readiness, but the plan is downstream of the track snapshot and is not an independent source of the original expected actions. |
| `QueueJob.ProfileSnapshot` | Frozen video profile and source-analysis inputs | Does not encode audio/subtitle/attachment actions. |
| `ExecutionPlan.Constraints`, `DecisionSources`, `Evaluation` | Versioned plan identity/fingerprint and execution/encoder constraints | `ExecutionPlan` has no track-action contract. Its fingerprint binds that plan; it cannot independently recover track intent absent a track contract in the plan. |
| `ResultArtifact.ResultIdentity` / `ExecutionBinding` | Exact artifact generation, output revision, Queue fingerprint and ExecutionPlan fingerprint | Authoritative for identity and stale-output checks, not for expected tracks. |
| `ResultArtifact.OutputProbe`, `Artifacts`, `StreamPlan`, `AssetConversion` | Observed output streams, manifest and execution diagnostics | Output/manifest evidence proves what was produced. `StreamPlan` and track decision report are derived from the same Queue snapshot/As-Is plan; none proves what should have been produced. |
| `ValidationRun` | Immutable result, rule outcomes and exact identity bindings | Its expected track values were produced by the normal evaluator from `QueueJob.TrackProfileSnapshot`; it preserves the old finding but is not an independent corrected expectation. |

Consequently, the three previously identified patterns (audio count mismatch; audio/subtitle actions plus sidecar/manifest mismatch; subtitle/sidecar mismatch with an additional video FPS `UNVERIFIED`) are all `NEEDS_EVIDENCE` from currently modeled execution records. The first two patterns cannot be declared recovered from matching output counts or sidecars. The third also retains its independent video FPS `UNVERIFIED`; track evidence cannot resolve it. Current Asset Defaults, current profile definitions, current scans, and output-derived guesses remain non-authoritative. An independently archived per-asset request/profile revision could change this classification only after its identity, revision, and relationship to the exact Queue execution are verified; no such record is established by these schemas.

### Minimal future append-only audit record (Phase B only)

If independent evidence is later supplied, the minimal persisted `ValidationReconciliation` record must contain: schema version; QueueJob ID and execution number; ExecutionPlan ID/version/fingerprint; ResultArtifact ID, generation token/version, and output revision; prior ValidationRun ID; canonical reconciled expectation contract and fingerprint; per-field evidence references containing source kind, immutable source ID/version/fingerprint and asset identity; server-assigned actor, required reason, creation time, and idempotency key. The record stores the proof and expected contract, not a mutation of the original Queue/Plan/Result/Validation rows. Missing per-field proof prevents commit. Phase A preview creates no such row and adds no migration.

## Goals / Non-goals

**Goals:** independent plan-based reconstruction; no media mutation; append-only correction evidence; identity-bound new ValidationRun; consistent effective expectations across Validation and manual Publisher; fail-closed incomplete evidence; opt-in operator approval; bounded NAS I/O.

**Non-goals:** requeue/encode, source or Result repair, modifying any original frozen snapshot or historical ValidationRun, new global policy exceptions, applying current profile defaults to historical files, automatically publishing, or claiming visual fidelity from track-count matches.

## Decisions

### 1. Independent expected-state reconstruction

Provide a pure reconciliation resolver taking a frozen QueueJob, its exact active ExecutionPlan, canonical ResultArtifact, original per-asset selection/provenance and independently frozen execution-time track/sidecar contract (where present). It MUST prove the expected stream indexes, actions, languages, extraction/embedded dispositions, artifact IDs and output paths were intended for that specific asset.

A Result's observed audio/subtitle stream counts and packaged sidecars cannot establish expectation on their own: doing so would validate an arbitrary defective output. The contaminated TrackProfileSnapshot cannot be treated as an authority for either a passing expectation or a repaired expectation. Do not replay current Asset Defaults, current profiles or current ScanResults as substitutes for execution-time choices. If the immutable historical records cannot prove a particular expected field, return NEEDS_EVIDENCE for that field and block commit. A plan that embeds the same contaminated data is not independent proof.

Keep the normal validation of video properties, timestamps, source/Result integrity, attachments, chapters, identity and Publisher requirements. Explicitly report unrelated UNVERIFIED video FPS; never silently resolve it through repaired track evidence.

### 2. Two-stage API, explicit operator intent

Phase A: a read-only preview/prepare operation for one job and exact Result revision. It MUST have no DB writes, no Queue/Publisher mutation and no new media work. It returns SAFE only when independently versioned per-asset intent and exact source/plan/Result identity are proven; otherwise it returns BLOCKED or NEEDS_EVIDENCE with original vs proposed per-asset expectations, identity metadata, blocking reasons, per-rule impact, and additional evidence required. The currently modeled historical records contain no independent track-intent source, so the three known patterns cannot currently produce SAFE. The preview is advisory and MUST NOT grant any durable authority. Bounded SELECT and existing compact Result artifacts are preferred. A detailed preview remains local/operator-visible and not published as a GitHub log.

Phase B: a commit request names the exact previously previewed immutable identities/revisions and requires an operator-provided nonempty reason. The server assigns the actor, e.g., local_operator in V1; client-provided actors are ignored. Under the existing asset/Validation locking and DB transaction boundaries, re-read and verify:
- active execution, original frozen Queue fingerprint and ExecutionPlan fingerprint
- canonical Result artifact generation, output revision and output file identity
- independently reconstructed expectation source and immutable input fingerprints
- required sidecar identities, manifests and current file revisions
- no stale candidate, conflicting active reconciliation or concurrent lifecycle/publish change.

Reject stale/cancelled/replaced/unbound inputs without side effects. Commit may perform bounded read-only probes and explicit append-only DB writes; do not perform FFmpeg conversion or filesystem mutation.

### 3. New reconciliation authority, not a mutable replacement snapshot

Propose a small append-only ValidationReconciliation record (or reuse a proven immutable existing audit model if sufficient). Its versioned contents include: job/execution/plan identity, exact Result identity/output revision, original active ValidationRun, old TrackProfileSnapshot fingerprint, separately constructed expected-track/sidecar contract and fingerprint, evidence/provenance references, per-rule rationale, server actor/reason, status, timestamps and idempotency key.

Store a separate reconciliation candidate/commit ID linked from the *new* ValidationRun's versioned report/provenance. Historical Queue profile/track snapshots, ExecutionPlan, ResultArtifact and prior ValidationRuns remain unmodified.

The existing QueueJob **validation summary and active ValidationRun pointer are mutable projections** and may be updated transactionally to reference the new run; this narrowly scoped metadata transition is not permission to rewrite job intent, frozen snapshots, job result identity or execution history. If that projection cannot be changed safely without violating an immutable lifecycle boundary, the implementation must explicitly model an alternative active-run selector before enabling Publisher.

### 4. Reuse the canonical evaluator with a narrowly scoped immutable overlay

Do not duplicate Validation rules or weaken mandatory findings. The canonical evaluator accepts an explicit verified expectation authority for only the contaminated per-asset track/sidecar dimensions, while continuing to use the original job for frozen execution binding and non-track requirements. Run all rules with actual Result evidence and current output revision as before.

A corrected track plan is NOT simply a new TrackProfileSnapshot on the historical job. Any in-memory effective-job view must be ephemeral, carry the reconciliation ID and expectation hash, and only be used after the original frozen Queue/Result binding is verified. Manifest checks must compare required artifact IDs with actual Result-manifest entries and confirm physical sidecar readiness under the reconciled contract. No missing artifacts may be fabricated. The resulting ValidationRun retains original and corrected expected-value references and has explicit reconciliation provenance.

All non-track mandatory findings, including unknown video FPS when it is blocking, remain UNVERIFIED/REVIEW_REQUIRED or FAIL. Never relabel them PASS because other track findings improved.

### 5. Publisher shares the same authority, and is always manual

Because Publisher currently computes sidecar readiness from the original QueueJob, a reconciled ValidationRun cannot authorize publication unless Publisher's *entire* eligibility, package selection, and final filesystem sidecar checks consume the exact same identity-bound reconciliation contract.

Modify only the manual, explicitly approved handoff for reconciled jobs. canonicalRunAllowsPublish must revalidate active run, original binding, Result revision, exact reconciliation record and independent eligibility of all blocking findings. Publisher must build a temporary, reconciled package/sidecar view for readiness and materialization while retaining the original job for frozen identity and unrelated checks. Any unsupported Publisher path or contradictory sidecar contract remains blocked. Require separate explicit Publish confirmation; no auto-publish triggered by successful reconciliation, even for PASS.

All current destination, overwrite, archive and original-file protections remain. Publisher must check source and destination file revisions again immediately before irreversible operations.

### 6. Idempotency, access and rollback

A retry with the same operator-approved idempotency key and identical Result/plan/expected-contract fingerprints returns the same persisted outcome; changed inputs result in a conflict. A new revision or different reconciliation requires a new approval. Accepted audit rows never mutate; cancelled/failed previews do not create authority. A transaction publishes the new run and active-summary reference together; restore to the original active run only through a separate explicit audited action if implemented.

Expose reconciliation preview in Review Center only on explicitly selected completed, unpublished, review-blocked canonical Results. Do not support bulk commit. Backend determines eligibility; the UI may not infer SAFE from a green status or send trusted actor IDs.

Deployment defaults recovery OFF until the operator explicitly enables/uses it. Rollback disables new reconciliation/Publisher activation but keeps append-only records and original failures available. Existing normal Validation and Publisher paths remain unchanged when no reconciliation exists.

## Risks / Trade-offs

- **Not enough independent historical intent:** Reject with NEEDS_EVIDENCE, even if Result happens to match an inferred stream count. Do not promise to recover all three outputs.
- **Existing publisher uses corrupted frozen job state:** Do not merge a Validation-only recovery. Test the whole manual publication package path using the effective reconciled view.
- **Output/media files can change between preview and commit:** Revalidate canonical Result revision and sidecar/file identity at commit and publish; no path-only comparisons.
- **Publisher activation needs a QueueJob projection change:** Narrow transaction updates to active run/summary only; leave historical frozen JSON and prior evidence untouched.
- **Runtime/NAS cost:** Reuse Result probes and sidecar manifests, keep file checks bounded, no full output re-encode or automatic FFmpeg work.
- **Concurrent validation or publication:** Enforce per-job exclusivity and atomic active run/approval revision checks.

## Migration Plan

1. Phase A: OpenSpec approval and synthetic pure preview prototype; no DB schema change or remote NAS reads. Decide which immutable historical record fields can serve as independent expected-state evidence.
2. Phase B: additive audit storage (only if necessary), explicit commit API, canonical evaluator overlay, Publisher-consistent manual handoff, Review Center UX and synthetic E2E coverage. Keep feature disabled for production until approved.
3. Phase C, separately authorized: deploy, bounded production read-only preview for each affected job, review expected/actual and missing evidence, then separately authorize any append-only reconciliation and new ValidationRun per exact Result. No publication without another explicit approval.
