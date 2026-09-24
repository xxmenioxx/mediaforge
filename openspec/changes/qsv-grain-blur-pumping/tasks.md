## 1. Freeze canonical sampling and experiment identity

- [ ] 1.1 Define the versioned `QSVTemporalExperiment` and candidate identity contract with source revision, #70 SamplingPlan/Window identities, baseline/variant identities, declared variable, requested/effective settings, runtime/filter/effective-plan identities, evidence references, classification, and warnings; verify repeated equivalent inputs produce stable identities.
- [ ] 1.2 Add the QSV consumer adapter over `SamplingPlanForQSVPumping()` for exactly `high_motion`, `high_texture_or_grain`, and `worst_predicted`; verify tests consume only frozen returned windows and preserve plan/policy provenance.
- [ ] 1.3 Record unavailable #70 roles without reselection or a second hard-scene analyzer; verify missing-role tests show explicit unavailability and no full-file or replacement scan.
- [ ] 1.4 Preserve the user-selected LAB primary/repro window, deduplicate only through the canonical interval identity/tolerance, and freeze source revision, policy, and window identities before variants start; verify a primary outside the role subset remains first and a stale source/plan is retryable rather than mixed.

## 2. Resolve effective QSV candidates and enforce isolation

- [ ] 2.1 Build baseline and variant candidates through the existing canonical profile, capability, effective-plan, and Compatible Preview resolution; capture requested versus effective QSV quality/rate-control, GOP, B-frame, supported feature controls, encoder/runtime identity, and fallback reasons without adding undocumented flags; verify capability fixtures cover supported and unsupported controls.
- [ ] 2.2 Implement canonical effective-setting delta validation with one declared changed variable and versioned derived fields only; verify an unexpected second effective change yields `invalid/non-isolated` and cannot produce causal attribution.
- [ ] 2.3 Freeze and compare restoration filters, scaling, normalization, audio/subtitle handling, and effective media plan; verify hqdn3d/chromanr/denoise/sharpening/grain/scale presence alone is not labeled as the cause of pumping.
- [ ] 2.4 Add isolated quality-only A/B coverage for `global_quality` and effective QSV rate-control behavior; verify the test compares effective settings and records requested/effective fallback state.
- [ ] 2.5 Add isolated GOP-only A/B coverage for resolved GOP length and source-relative versus recommended GOP semantics; verify cadence, rate control, B-frame behavior, and filters remain frozen.
- [ ] 2.6 Add isolated BF-only A/B coverage for `-bf`/maximum B depth and adaptive-B behavior as separate variables; verify an incidental GOP or rate-control change invalidates the comparison.

## 3. Add bounded temporal and optional perceptual evidence

- [ ] 3.1 Add an injectable temporal-evidence seam over adjacent frames or short segments with bounded frame observations, deterministic normalization/region identity, detail/grain energy, blur/detail variation, loss/recovery summaries, and evidence references; verify deterministic fixtures produce stable measurements without a single-frame shortcut.
- [ ] 3.2 Separate measured temporal evidence from a versioned interpretation policy; verify loss-then-recovery fixtures can yield `likely pumping`, adequate no-signal fixtures yield `not observed`, and insufficient/ambiguous fixtures yield `inconclusive` rather than a universal-detector claim.
- [ ] 3.3 Add an optional #69 `PerceptualEvidence` adapter keyed by source/plan/window/generation identity; verify matching evidence is attached by bounded reference and absent, stale, or failing #69 evidence leaves the experiment runnable with an explicit warning.
- [ ] 3.4 Reuse existing Frame Fidelity, QSV bitstream/frame-structure, size, bitrate, and speed evidence through injectable providers; verify no second VMAF/SSIM pipeline or unbounded raw-frame/log persistence is introduced.

## 4. Extend the existing LAB/Compatible Preview lifecycle

- [ ] 4.1 Extend `LabFidelityOperation` with explicit QSV experiment orchestration while retaining its existing states, cancellation, stale-run checks, and heavy-work slot; verify normal LAB/Fidelity runs without opt-in perform no variant work.
- [ ] 4.2 Reuse the ready primary Compatible Preview artifact as the baseline and prove reuse with request/artifact/generation/canonical-signature identity plus generation-boundary accounting; verify baseline generation count is unchanged and request-registration count alone cannot pass the test.
- [ ] 4.3 Generate each explicit variant through Compatible Preview with a distinct candidate/artifact identity; verify a completed or failed variant never replaces the baseline identity, primary playback, or source-backed Sample A/reference.
- [ ] 4.4 Enforce the default maximum of three variants plus one baseline, deterministic ordering, one heavy QSV phase at a time, and direct baseline-relative comparisons; verify a request for quality × GOP × BF combinations does not create a Cartesian product.
- [ ] 4.5 Implement cancellation cleanup for prepare, encode, evidence, artifact protection, and operation bookkeeping; verify cancellation releases capacity/protection, retains bounded terminal provenance, and leaves the ready baseline usable.
- [ ] 4.6 Verify retry/stale-generation behavior never compares mixed artifact generations and that an explicit promotion starts a new experiment identity instead of silently replacing the old baseline.

## 5. Expose explicit LAB workflow and evidence

- [ ] 5.1 Extend API/client types and the existing LAB action flow for explicit QSV experiment start/status/cancel and bounded variant summaries; verify tab changes, rerenders, and polling alone do not start heavy work.
- [ ] 5.2 Preserve the existing primary Preview Ready boundary, Sample A/reference identity, variant labels, and baseline-relative progress; verify the UI can distinguish measured temporal evidence, interpretation, unavailable roles, invalid isolation, and manual visual-review state.
- [ ] 5.3 Add focused frontend coverage for explicit start/cancel/retry, baseline reuse, variant identity preservation, stale-result fencing, no hidden tab work, and no replacement of the ready primary artifact.

## 6. Classify outcomes and produce safe recommendations

- [ ] 6.1 Implement the four bounded result classifications and conservative QSV-specific attribution requiring a matching non-QSV/control basis; verify QSV-only evidence cannot become `reproducible_qsv_specific_pumping` and decoder compatibility is reported separately.
- [ ] 6.2 Emit versioned mitigation recommendations with evidence basis, encoder/runtime, media characteristics, confidence, and size/speed/compression trade-offs; verify recommendations never mutate existing Profiles automatically.
- [ ] 6.3 Add an explicit draft/new-Profile application path only if the user or policy selects a recommendation; verify unrelated assets and existing Profiles remain unchanged.

## 7. Validate without NAS media, network, or hardware

- [ ] 7.1 Run backend unit tests through fake/injectable plan, capability, encoder, artifact, temporal-evidence, and optional-#69 seams; verify all selection, identity, isolation, lifecycle, cancellation, and temporal fixtures pass without NAS paths, network access, real media, or QSV hardware.
- [ ] 7.2 Run focused frontend tests/build checks for the changed API and LAB symbols, recording the repository's known global lint baseline separately; verify no unrelated lint baseline is presented as a regression.
- [ ] 7.3 Run `openspec validate qsv-grain-blur-pumping --type change --strict` and the canonical `./scripts/verify.sh --auto`; require `ALL CHECKS PASSED` before implementation delivery and confirm no production media or profile data was modified.
