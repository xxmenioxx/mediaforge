## 1. Freeze the canonical evidence and reference contracts

- [ ] 1.1 Define bounded `PerceptualEvidenceSet`, `ReferenceIdentity`, `CandidateIdentity`, `PerceptualWindowEvidence`, metric result/status, aggregate, capability, warning, and artifact-evidence models; verify JSON round-trips preserve schema/policy versions, identities, nullable values, and all non-numeric statuses.
- [ ] 1.2 Implement canonical evidence identity hashing over source revision, SamplingPlan/Window identities, transform/alignment/normalization, candidate generation/output, capability, and metric/model contracts; verify identical inputs reuse identity while source replacement, window/plan changes, candidate generation replacement, and metric/capability version changes invalidate it.
- [ ] 1.3 Add the Frame Fidelity/reference adapter consuming `fidelityAuthority`, `fidelityExpectedOutput`, frozen effective video decisions, and existing preview normalization; verify cadence/FPS, deinterlace, crop/scale, SAR/DAR, field, and color transforms have one authority and unresolved authority produces `non_comparable`/`unavailable` rather than a guessed raw reference.
- [ ] 1.4 Define and test the versioned source→reference→candidate time/frame alignment contract for unchanged cadence, IVTC/FPS changes, deinterlace, normalized timestamps, and trimmed Test Encode segments; verify ambiguous alignment never pairs frames by index or source time zero.

## 2. Add runtime capability and metric execution seams

- [ ] 2.1 Extend the existing FFmpeg runtime capability boundary and immutable runtime snapshot to detect actual VMAF/libvmaf model, XPSNR, SSIM, MS-SSIM, and PSNR support with executable/filter/model contract identities; verify legacy snapshots remain readable and probes are cached once per runtime/evidence operation rather than once per window.
- [ ] 2.2 Implement injectable, argument-array metric runners behind the existing LAB frame-metric process boundary; verify VMAF, XPSNR, SSIM, MS-SSIM, and diagnostic PSNR parse their own semantics/version/model and use cancellation plus bounded stderr/diagnostics.
- [ ] 2.3 Implement per-metric status isolation (`available`, `unsupported`, `unavailable`, `failed_measurement`, `non_comparable`); verify VMAF absence continues with available XPSNR/SSIM/PSNR, MS-SSIM absence does not invoke an unsupported filter, and one metric failure does not erase successful metrics or become zero.
- [ ] 2.4 Require metric execution to receive a ready candidate artifact/output identity and an already resolved reference/alignment; verify metric requests cannot start an encode, cannot probe a new scene set, and release artifact-use/process resources on success, failure, and cancellation.

## 3. Extend the bounded LAB evidence provider

- [ ] 3.1 Factor the current LAB source/output probe and one-frame SSIM/PSNR path into the shared perceptual provider while preserving the existing `labPreviewEvidence` cache and Compatible Preview artifact lease; verify cache keys include source revision, artifact generation, reference/window/alignment, metric/model, and capability identities.
- [ ] 3.2 Make LAB consume `SamplingPlanForPerceptualQuality` and preserve the frozen primary first, roles/reasons, fallback, plan identity, and window identities; verify no independent hard-scene selector or replacement window can be invoked.
- [ ] 3.3 Attach a bounded `PerceptualEvidenceSet` to the existing LAB Fidelity operation/result and read-only API projection beside size, bitrate, effective encoder, speed, and canonical Frame Fidelity; verify old SSIM/PSNR/Frame Fidelity fields remain readable when the new set is absent.
- [ ] 3.4 Reuse the ready primary Compatible Preview artifact and keep additional #70 work bounded/sequential under existing LAB capacity; verify generation accounting reports no primary re-encode and maximum additional heavy-work concurrency of one.
- [ ] 3.5 Extend LAB cancellation, retryable stale-generation handling, partial progress, and cleanup; verify cancellation leaves the primary preview usable, releases leases/processes/temporary state, and never presents partial evidence as a final verdict.

## 4. Implement deterministic per-window and aggregate evidence

- [ ] 4.1 Emit per-window evidence with frozen interval, primary/roles/reasons, reference/candidate/alignment identities, metric descriptors/results, structured artifact-evidence boundary, and bounded reasons; verify missing/unavailable/non-comparable values are explicit and never numeric zeroes.
- [ ] 4.2 Implement versioned metric aggregation using available values only, deterministic median, nearest-rank p10, direction-aware worst selection, unavailable/non-comparable/failed counts, and degraded-percentile provenance; verify map/order/goroutine-independent results and stable worst-window identity on ties.
- [ ] 4.3 Keep structured blocking/blur/detail, banding, ringing/halo, and temporal-instability evidence separate from metric aggregates; verify an available matching deterministic signal attaches independently and an absent analyzer does not block metrics.
- [ ] 4.4 Keep canonical evidence advisory and threshold-free; verify no new VMAF/SSIM/XPSNR/PSNR universal pass/fail rule is applied, while the legacy LAB visual projection remains compatible and cannot override a canonical Frame Fidelity mismatch.

## 5. Reuse the contract for Test Encode

- [ ] 5.1 Freeze or reference the canonical SamplingPlan and effective video authority at Test Encode preparation and persist a candidate identity containing Test Encode ID, output revision, configuration hash, and segment mapping; verify changing the mutable Profile after freeze cannot change the evidence input.
- [ ] 5.2 Expose the shared PerceptualEvidenceSet through Test Encode's existing validation-report boundary for a compatible completed output; verify Test Encode does not create a metric-specific schema or encode another sample solely for measurement.
- [ ] 5.3 Add Test Encode identity/alignment regressions for non-zero segment starts, output generation replacement, same-path replacement, and exact evidence reuse; verify stale evidence is rejected and equivalent source/reference/candidate identities reuse bounded results.

## 6. Reuse the contract for final Validation

- [ ] 6.1 Adapt final Validation to read the frozen Queue `samplingPlan` and effective authority from the immutable ProfileSnapshot, evaluate the final output with the shared reference/candidate provider, and persist bounded evidence alongside Frame Fidelity; verify current scanner/Profile/Asset Override state cannot select or reinterpret windows.
- [ ] 6.2 Preserve legacy Queue behavior when a snapshot has no usable plan/capability/evidence identity; verify Validation reports perceptual evidence unavailable without failing solely for optional metric absence or performing a new scene search.
- [ ] 6.3 Add final-output identity fencing and reuse tests for verified execution artifacts, output replacement, candidate generation changes, VMAF-unavailable fallback, and shared LAB/Test Encode evidence identities.

## 7. Add read-only LAB/Test Encode/Validation presentation

- [ ] 7.1 Add bounded API/frontend types and projections for the shared evidence set, per-window statuses, metric semantics, aggregates, warnings, capability availability, and worst-window links; verify frontend rendering does not infer cadence, parity, quality thresholds, or window selection.
- [ ] 7.2 Place perceptual evidence beside existing LAB Fidelity information without replacing native media playback or ready-artifact behavior; verify switching tabs or reopening read-only details causes no encode, metric-start, capability probe per window, or selector call.
- [ ] 7.3 Add UI/API compatibility coverage for partial windows, cancellation, stale evidence, unavailable VMAF, non-comparable reference, and legacy reports; verify users can distinguish measured evidence from legacy visual projection and structural Frame Fidelity.

## 8. Complete deterministic regression coverage and repository validation

- [ ] 8.1 Add fake capability/process/reference seams and unit tests for canonical transform identity, raw-source rejection when transforms differ, unchanged reference reuse, source replacement invalidation, and exact artifact-generation fencing; verify tests use temporary fixtures only and do not require NAS, network, optional libvmaf, or host hardware.
- [ ] 8.2 Add SamplingPlan integration tests proving only frozen #70 windows are consumed, primary is first and reused without re-encode, roles/reasons/source revision/plan identity/fallback survive, maximum windows are enforced, and no independent scene reselection occurs.
- [ ] 8.3 Add capability matrix tests for VMAF available/unavailable, XPSNR/SSIM/PSNR combinations, optional MS-SSIM, metric failure isolation, bounded capability probing, and capability identity changes.
- [ ] 8.4 Add aggregation tests for median, degraded lower percentile, unavailable-window exclusion, deterministic ties, metric direction/semantics, worst-window identity, and absence of arbitrary universal thresholds.
- [ ] 8.5 Add lifecycle tests for LAB primary reuse, sequential bounded additional work, cancellation/resource cleanup, stale generation rejection, Test Encode and Validation contract parity, persisted bounded reports, legacy LAB compatibility, and no hidden recomputation.
- [ ] 8.6 Run `openspec validate perceptual-quality-evidence --type change --strict`, review the final SDD diff/status against the #70-based main, and run the repository's canonical verification command after implementation; verify this planning phase creates no production-code changes.
