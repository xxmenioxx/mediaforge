## 1. Existing operation lifecycle and API

- [ ] 1.1 Extend the existing Asset Info subtitle extraction operation state and its synchronized controller with a `cancelled` terminal outcome, idempotent cancellation, and completion/cancellation race handling; verify focused backend operation-state tests distinguish `cancelled` from `error` and preserve terminal results.
- [ ] 1.2 Add the operation-ID cancellation action to the existing Asset Info API and make running text SRT/ASS conversion use the existing operation model while preserving immediate responses when no new work is required; verify focused handler tests cover running, terminal, duplicate, and retry requests.

## 2. Context and process cancellation

- [ ] 2.1 Thread one operation-owned `context.Context` through canonical text extraction, bitmap segmentation, OCR, normalization, cleanup, and SRT/ASS rendering; verify cancellation at each stage ends as `cancelled` rather than as a generic tool failure.
- [ ] 2.2 Implement bounded process-tree termination for cancellable FFmpeg, SeConv, Tesseract, and other child processes using argument arrays and the supported backend process-group behavior; verify disposable-process tests prove descendants terminate after cancellation.

## 3. Safe canonical artifact execution

- [ ] 3.1 Put new subtitle outputs and OCR intermediates in operation-owned staging, guard the complete canonical output set behind validation and publication, and clean only owned partial files/workspace on cancellation; verify tests prove no partial SRT/ASS sidecar is visible and source media is unchanged.
- [ ] 3.2 Preserve valid existing sidecars and sidecars completed before the cancellation boundary, including canonical shared `both` outputs, while preventing destructive replacement; verify tests cover existing artifacts, cancellation during the second `both` output, and cleanup isolation.
- [ ] 3.3 Keep PGS and VobSub conversion on the canonical OCR path and allow a cancelled request to retry through the same plan without duplicate sidecar discovery or new OCR semantics; verify focused OCR tests cover SRT and ASS retry after cancellation.

## 4. Asset Info frontend lifecycle

- [ ] 4.1 Add the `cancelled` operation state and operation-ID cancel request to the frontend API types/client, then render Cancel only for running SRT, ASS, PGS, and VobSub conversions; verify focused Asset Info component/API tests cover visibility, disabled state, and request mapping.
- [ ] 4.2 Stop progress polling and running UI state cleanly on `cancelled`, keep cancellation separate from error presentation, and expose retry through the existing generation action while preserving existing-sidecar indicators; verify focused frontend tests cover cancellation response, polling termination, duplicate clicks, and retry.

## 5. Regression validation

- [ ] 5.1 Run focused backend subtitle and Asset Info tests covering text SRT/ASS, PGS/VobSub OCR, context propagation, child-process termination, cleanup, existing-sidecar preservation, cancelled-versus-failed status, and retry; verify all targeted tests pass without requiring real media, OCR binaries, hardware, or NAS mounts.
- [ ] 5.2 Run focused frontend Asset Info tests for Cancel, terminal polling, progress cleanup, and retry; verify changed-file lint/build checks remain free of new diagnostics while the known repository lint baseline is reported separately.
- [ ] 5.3 Run `./scripts/verify.sh --auto` from the repository root and verify the output reports `ALL CHECKS PASSED`.
