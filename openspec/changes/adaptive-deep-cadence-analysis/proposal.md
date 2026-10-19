## Why

The historical #2191 snapshot for *Sin Limite* records hard telecine, 30000/1001 source FPS, container field-order metadata `tt`, and only one validated IVTC window out of three. With no validated `SelectedOrder`, #352 correctly leaves effective FPS unknown and HEVC unresolved. The remaining gap is a bounded way to gather more evidence and feed the canonical Queue review before a new job is persisted.

The historical fixture is the available evidence for this change; this work does not collect new NAS measurements or alter Job #2191.

## What Changes

- Add an explicit asynchronous, cancellable Deep Cadence Analysis mode to the existing snapshot-operation flow.
- Reuse the existing three IVTC windows and analyze at most two additional, distributed regions with existing FFmpeg/idet comparisons for TFF and BFF.
- Record attempted, valid, inconclusive, and contradictory window evidence with a version and physical-source identity; discard partial or stale results.
- Reuse canonical cadence/HEVC resolution in the existing single-item Batch Preflight and Queue acceptance path. Continue returning `REVIEW_REQUIRED` when evidence remains ambiguous.
- Expose analysis progress, cancellation, evidence review, per-asset TFF/BFF and Preserve choices, plus existing LAB/Test Encode actions in the individual Queue review flow.
- Reuse the #284 synthetic harness and existing Pipeline Smoke build; do not add an image build or implement #285's full output-fidelity scenario here.

## Capabilities

### New Capabilities
- `adaptive-deep-cadence-analysis`: bounded deep IVTC evidence, its identity and lifecycle, and its use by individual Queue review.

### Modified Capabilities
- None. The Queue single-asset behavior introduced in the existing `single-queue-cadence-review` change is extended by this new capability in the same Draft PR.

## Impact

Backend interlace/cadence analysis, snapshot-operation request and persistence, Queue Preflight, and their tests; frontend API types and individual Queue review; existing synthetic Pipeline Smoke harness integration. Evidence remains JSON in the existing ScanResult snapshot, so no SQLite migration or new persisted lifecycle state is introduced. The Worker claim-time fail-closed guard and #352 HEVC authority remain unchanged. No LLM/VLM, OpenVINO, NAS access, production media, or production configuration changes are included.
