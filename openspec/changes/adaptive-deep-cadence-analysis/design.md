## Context

See proposal.md for the historical incident and scope. The current interlace scan stores versioned `InterlaceAnalysis` and `CadenceAnalysis` in `ScanResult`; IVTC validation selects three distributed starts and compares `fieldmatch=order=tff|bff,decimate,idet` using `validatedIVTCOrder` and `ivtcWindowsAgree`. The existing snapshot operation already provides asynchronous progress, cancellation, timeouts, per-asset deduplication, and a concurrency limit. Single-item Queue Preflight already owns effective profile resolution, cadence freeze, HEVC planning, candidate revisions, and transactional acceptance. #352's Worker guard remains the last fail-closed boundary.

The production NAS was not accessed. Exact per-window causes behind the historic 1/3 result are unavailable in the checked-in fixture; `tt` is metadata, not a parity decision.

## Goals / Non-Goals

**Goals:**
- Extend the current evidence path and async snapshot operation, without adding a second cadence resolver, analysis store, or job lifecycle.
- Reuse the initial three IVTC comparisons, run at most two additional comparisons, preserve thresholds, and require distributed non-contradictory evidence before automatic order selection.
- Persist versioned evidence with file identity and revision; reject stale writes and stale reads.
- Let the existing Queue Preflight recompute and freeze FPS/HEVC through canonical authorities.

**Non-Goals:**
- Changing thresholds, guessing from `tt`, changing #352, editing profiles globally, or changing historic jobs/plans.
- Continuous deep analysis during every scan, full-length decoding, conversion, NAS diagnostics, or production data access.
- Implementing #285's final-frame-rate/Frame Fidelity smoke pipeline, adding Docker image builds, LLM/VLM, or OpenVINO.

## Decisions

1. **Explicit deep request on the existing snapshot operation.** Add a deep-cadence intent to the current snapshot-operation request. Reuse its async worker, progress, cancellation, timeout, single-asset exclusion, and configured concurrency cap. This keeps ordinary scans lightweight and avoids a new job table or scheduler. A conflicting in-flight scan must not be reported as a completed deep run.

2. **Resume IVTC validation from stored windows.** Load the current `ScanResult` and reuse its three `IVTCValidation.Windows`; select at most two unused positions from the existing canonical sampling plan. Prefer pre-existing high-motion or temporal/telecine evidence roles, then well-separated positions with the strongest existing interlace/repeat evidence, then distributed canonical positions. Record the selection basis for each window. No new detector is introduced solely for window selection.

3. **Keep the existing order authority and add a distribution guard.** Run the same TFF/BFF FFmpeg/idet chain and the existing `validatedIVTCOrder` thresholds. Store every attempted window as valid, inconclusive, or contradictory with bounded reasons. Deep evidence resolves only with at least three distributed valid windows supporting one order and no confident opposite-order window; the aggregate still has to pass the existing confidence/margin and agreement checks. This is conservative and does not lower thresholds.

4. **Store evidence inside the existing ScanResult component.** Extend the existing IVTC evidence JSON with an evidence version, outcome counts, window reasons/selection basis, and source identity. Reuse `ScanResult.InterlaceAnalysis`, cadence recomputation, and snapshot component versioning; no table, migration, or persisted operation lifecycle is added. Bind evidence to canonical path, size/mtime, device/inode when available, ScanResult ID/revision, and source AssetRecord identity when present. Compare the captured and current file identity plus latest snapshot before updating. Cancelled, partial, and stale runs do not overwrite a snapshot.

5. **Queue consumes only current frozen evidence.** Deep completion triggers the existing one-item Batch Preflight re-resolution. That path recalculates cadence FPS and HEVC via canonical preparation, then uses its current revision/source identity and atomic acceptance. Unresolved/stale/cancelled analysis remains `REVIEW_REQUIRED` with no QueueJob, plan, or reservation. Queue Create and claim guards from #352 remain unchanged as defense in depth.

6. **Keep operator controls asset-scoped.** Add start/cancel/progress/evidence review to the existing individual Queue review dialog. Keep explicit TFF/BFF, Preserve, and current LAB/Test Encode links. All choices stay on the candidate asset; no global profile update is issued.

7. **Coordinate smoke coverage with #285.** Reuse the #284 deterministic fixture generator and existing Pipeline Smoke image/workflow. Add a cadence-analysis scenario to the existing smoke execution only if it can run inside the same build; do not create per-scenario image builds or duplicate #285's output fidelity assertions.

## Risks / Trade-offs

- [Two more regions may still not resolve cadence] → Persist their outcomes and retain `REVIEW_REQUIRED`; manual per-asset selection remains available.
- [A source changes during FFmpeg execution] → Verify physical identity and revision before persistence and again through existing Queue acceptance fences.
- [A high-motion role is absent from available evidence] → Prefer temporal/telecine evidence, then distributed canonical positions; record the fallback basis rather than inventing motion scores.
- [Resource pressure on NAS] → Deep work is opt-in, limited to two additional windows, cancellable, timeout-bounded, and subject to existing analysis concurrency controls.
- [Existing scans lack the new evidence fields] → Absence of the nested deep-evidence version keeps them unresolved; physical identity/revision checks mark copied or obsolete deep results stale while old Queue snapshots remain immutable.
- [#285 evolves the shared harness concurrently] → Keep the same generator and smoke job; coordinate only the scenario boundary and avoid extra Docker builds.

## Migration Plan

No SQLite migration and no broad interlace cache-version bump. Version the nested Deep Cadence evidence independently so existing scans remain reusable for their original three-window evidence; snapshots without the nested version remain unresolved. Revalidate physical identity and snapshot revision before saving and before Queue acceptance. Existing QueueJobs and ExecutionPlans are not rewritten. Reuse the #284 hard-telecine fixture generator and sandbox. The full output-fidelity smoke remains #285's boundary and uses the existing image/job so cadence coverage does not add a Docker build. Rollback removes the new deep operation while leaving unknown/new JSON fields ignored by older code.
