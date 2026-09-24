## 1. Freeze the shared contracts and current behavior

- [x] 1.1 Extract a backend-owned LAB sampling/window resolver that preserves the current `labFrameSamplingPolicy` defaults, adaptive rules, duration clamping, and configured positions; verify unit cases for short, medium, and long durations plus policy overrides.
- [x] 1.2 Define the bounded primary/operation/window DTOs and material-signature fields, including source revision, normalized interval, effective plan identity, normalization, request/artifact identity, generation token/version, and evidence references; verify serialization contains no unbounded Profile, probe, or log payload.
- [x] 1.3 Add an executable call-accounting seam around canonical Compatible Preview generation, structured probes, specialized probes, and metrics; verify the seam can distinguish request registration from actual generation.

## 2. Reuse Compatible Preview artifacts and evidence

- [x] 2.1 Add a thin primary readiness adapter over the existing registered Compatible Preview behavior, with an integration point for #15 generating/ready/status/lease responses; verify current-main behavior reaches ready without depending on #15 routes.
- [x] 2.2 Refactor Compatible Preview inspect and metrics preparation behind a bounded evidence provider keyed by artifact ID plus generation token/version, with source revision/interval/normalization identity where applicable; verify a replaced generation cannot read old evidence.
- [x] 2.3 Share generic structured probe, measured FPS, frame-structure, frame-reference, and metrics evidence only where the evidence semantics match; verify generic ffprobe evidence does not satisfy a specialized HEVC `trace_headers` request.
- [x] 2.4 Make comparison-frame access a read-only artifact/evidence consumer and preserve source-backed frame generation without creating a processed artifact for Sample A; verify frame requests do not trigger a second Compatible Preview encode.
- [x] 2.5 Protect every bounded ready-artifact evidence read with the existing #1 serving/analysis consumer mechanism or canonical equivalent, releasing it on completion, failure, or cancellation; do not add a second lease, generation consumer, generating state, or indefinite retention; verify cleanup cannot race an active read and later cleanup remains normal.
- [x] 2.6 Preserve and strengthen the source/reference cache signature with source revision, normalized time interval, and normalization while excluding the processed Profile; verify profile-only edits reuse the reference and source/interval/normalization changes invalidate it.

## 3. Implement the bounded backend Fidelity operation

- [x] 3.1 Add the process-local operation store, state/phase transitions, cancellation context, bounded diagnostics, partial window snapshots, and terminal cleanup using the existing bounded operation pattern; verify queued, running, completed, failed, and canceled transitions are race-safe.
- [x] 3.2 Add route handlers and client methods for start/status/cancel and the repository-consistent retry action, resolving the primary request/artifact server-side rather than trusting an unbounded client payload; verify unknown, not-ready, terminal, and retryable responses have stable bounded error semantics.
- [x] 3.3 Freeze the primary descriptor, source revision, effective plan, normalization, sampling policy, and material signature at operation start; verify a mutable Profile edit does not alter an in-flight operation's expected plan.
- [x] 3.4 Add the process-local LAB Fidelity heavy-work capacity guard, initially one slot across concurrent LAB Fidelity operations; verify a second operation reports queued capacity and no two additional heavy windows overlap.
- [x] 3.5 Add operation retention by age and maximum terminal history without deleting Compatible Preview artifacts or #14 records; verify cleanup removes only expired operation metadata.

## 4. Run primary and additional windows deterministically

- [x] 4.1 Build the ordered window plan with the selected interval first and additional policy intervals afterward; verify the selected interval is present even when it is not a configured sampling position.
- [x] 4.2 Implement canonical media-time deduplication after duration clamping and request normalization using source-timebase boundary tolerance; verify an equivalent primary/50% interval produces no duplicate request or generation while a materially different nearby interval remains.
- [x] 4.3 Run primary evidence and each additional window through canonical Compatible Preview request/artifact acquisition rather than raw FFmpeg; verify request identity is stable for the same source revision, interval, plan, and normalization.
- [x] 4.4 Enforce sequential additional-window execution, including encode and heavy evidence phases, with cancellation checks at each boundary; verify barrier/counter tests report maximum concurrency of one without sleeps.
- [x] 4.5 Retain completed windows and their evidence references as partial results, distinguish uncomparable metrics from infrastructure failure, and define retry starting points; verify partial status never emits a final Safe or Reject aggregate.

## 5. Integrate canonical #3 Fidelity and aggregation

- [x] 5.1 Factor or adapt the #3 canonical evaluator to accept a frozen Compatible Preview authority without changing Queue authority precedence or introducing a LAB resolver; verify source/expected/actual/authority/reason field semantics for geometry, FPS, field structure, pixel semantics, color, preserve, and HEVC cases.
- [x] 5.2 Pass shared structured and specialized evidence into the canonical evaluator for every completed window; verify high SSIM plus an actual FPS mismatch remains a canonical mismatch and is not an infrastructure error.
- [x] 5.3 Preserve `fidelityMetricVerdict`, average/minimum SSIM, average/minimum PSNR, comparable count, worst region, and existing thresholds while separating visual aggregate from canonical status; verify all-comparable metric fixtures retain existing Safe/Review/Reject behavior.
- [x] 5.4 Define the combined presentation so canonical mismatch forces the authoritative failure presentation, canonical unverified prevents Safe, and incomplete/failed operation state is not converted into a final verdict; verify each combination in focused aggregation tests.

## 6. Rebuild Process Video around Preview Ready

- [x] 6.1 Split `Process Video` into primary request/readiness orchestration and non-blocking Fidelity operation start, retaining request/material identity checks and aborting superseded work; verify Sample B readiness is emitted while an injected Fidelity barrier remains blocked.
- [x] 6.2 Replace the single frontend multi-window React promise chain with status polling over the backend operation at a practical non-sub-second cadence; verify polling stops on terminal/not-found state and does not start generation or acquire hidden ownership.
- [x] 6.3 Bind Sample B playback to the ready primary Compatible Preview request through the existing transport abstraction; preserve current transport on pre-#15 main and consume #15 native ready-media behavior when available; verify #19 adds no Blob/native transport, credentials/Cookie, Range/seek, or readiness-race policy.
- [x] 6.4 Render independent Preview and Fidelity states, incremental completed-window results, bounded current-window progress, cancel, retry, and failure messaging; verify cancel/retry controls do not discard a ready primary preview.
- [x] 6.5 Gate result application on operation ID, primary artifact/generation identity, and current material signature; verify a late operation A result cannot replace current signature B state after a Profile or Start/Seconds edit.
- [x] 6.6 Make Previews, Technical, Frames, Characteristics, and Warnings tabs read-only consumers of collected operation/evidence data; verify tab changes produce no generation, probe, or metrics-start calls.

## 7. Add deterministic regression coverage

- [x] 7.1 Add backend tests for preview-first readiness, primary generation count exactly one, primary request/artifact reuse, and inspect/metrics reuse; verify playback, inspect, metrics, Fidelity, and frame consumers share the primary generation counter.
- [x] 7.2 Add backend runner tests for sequential-window barriers, global capacity serialization, deterministic primary deduplication, partial progress, and no-final-verdict behavior; verify all concurrency assertions use channels/counters rather than arbitrary sleeps.
- [x] 7.3 Add evidence-provider tests for one structured probe across consumers, source/reference material-signature reuse and invalidation, generation-token invalidation, frame-reference reuse, and separate HEVC trace/header evidence.
- [x] 7.4 Add deterministic artifact-protection tests for primary evidence versus cleanup, cancellation releasing LAB analysis protection without forced deletion, and independent playback/Fidelity protections on one ready artifact.
- [ ] 7.5 Add cancellation, retry, background-failure, and shared-consumer tests; verify cancel preserves a ready primary, retry makes no primary generation call, and additional infrastructure failure is retryable without changing Preview Ready.
- [ ] 7.6 Add restart-boundary tests for process-local operation loss and #14 primary artifact reuse; verify a new operation can retry the primary without re-encoding it.
- [x] 7.7 Add canonical #3 adapter and aggregation tests for intentional transformations, unverified evidence, high-SSIM canonical mismatch, and visual-threshold compatibility.
- [ ] 7.8 Add focused frontend tests for independent state rendering, polling lifecycle, stale-signature protection, cancel/retry controls, the pre-#15 existing-transport/post-#15 adapter boundary, and no hidden tab-triggered work.

## 8. Validate and hand off the implementation plan

- [x] 8.1 Run focused backend/frontend tests for the changed operation and evidence seams, then run `./scripts/verify.sh --auto`; verify the canonical verifier reports `ALL CHECKS PASSED` during implementation.
- [x] 8.2 Review the final implementation diff against the expensive-work accounting table and ownership boundaries; verify no second preview lease/cache or artifact-use guard system, Queue job, general scheduler, raw Fidelity FFmpeg path, threshold retune, #14 persistence engine, or #19-owned Blob/native transport policy was introduced.
