## 1. Freeze canonical sampling and experiment identity

- [x] 1.1 Define the versioned `QSVTemporalExperiment`/`QSVTemporalTarget`/variant contract so each target owns exactly one #70 SamplingWindow, baseline identity, bounded variants, per-variant changedVariable/requested-effective delta/isolation result, temporal/#69 evidence, classification, and recommendation; verify repeated equivalent target graphs produce stable identities.
- [x] 1.2 Add the QSV consumer adapter over `SamplingPlanForQSVPumping()` for exactly `high_motion`, `high_texture_or_grain`, and `worst_predicted`; verify tests consume only frozen returned windows and preserve plan/policy provenance.
- [x] 1.3 Record unavailable #70 roles without reselection or a second hard-scene analyzer; verify missing-role tests show explicit unavailability and no full-file or replacement scan.
- [x] 1.4 Preserve the user-selected LAB primary/repro window, deduplicate only through the canonical interval identity/tolerance, and freeze source revision, policy, target, and window identities before baseline/variant requests start; verify primary plus two role windows produce three target/baseline identities, the primary remains first, and a stale source/plan is retryable rather than mixed.

## 2. Resolve effective QSV candidates and enforce isolation

- [x] 2.1 Build one baseline per target through the existing canonical profile, capability, effective-plan, and Compatible Preview resolution; reuse the ready primary only for the primary target and materialize/reuse an exact-interval baseline for each role target; verify a role target cannot reuse the primary artifact.
- [x] 2.2 Capture requested versus effective QSV quality/rate-control, GOP, B-frame, supported feature controls, encoder/runtime identity, and fallback reasons without adding undocumented flags; verify capability fixtures cover supported and unsupported controls.
- [x] 2.3 Implement per-target effective-setting delta validation with one declared changed variable and versioned derived fields only; verify an unexpected second effective change yields `invalid/non-isolated` and cannot produce causal attribution.
- [x] 2.4 Freeze and compare restoration filters, scaling, normalization, audio/subtitle handling, and effective media plan within each target; verify hqdn3d/chromanr/denoise/sharpening/grain/scale presence alone is not labeled as the cause of pumping and different targets are never cross-compared.
- [x] 2.5 Add isolated quality-only A/B coverage for `global_quality` and effective QSV rate-control behavior per target; verify the test compares effective settings and records requested/effective fallback state.
- [x] 2.6 Add isolated GOP-only A/B coverage for resolved GOP length and source-relative versus recommended GOP semantics per target; verify cadence, rate control, B-frame behavior, and filters remain frozen.
- [x] 2.7 Add isolated BF-only A/B coverage for `-bf`/maximum B depth and adaptive-B behavior as separate variables per target; verify an incidental GOP or rate-control change invalidates the comparison.

## 3. Add bounded temporal and optional perceptual evidence

- [x] 3.1 Add an injectable temporal-evidence seam over adjacent frames or short segments with bounded frame observations, deterministic normalization/region identity, detail/grain energy, blur/detail variation, loss/recovery summaries, and evidence references; verify deterministic fixtures produce stable measurements without a single-frame shortcut.
- [x] 3.2 Separate measured temporal evidence from a versioned interpretation policy; verify loss-then-recovery fixtures can yield `likely pumping`, adequate no-signal fixtures yield `not observed`, and insufficient/ambiguous fixtures yield `inconclusive` rather than a universal-detector claim.
- [x] 3.3 Add an optional #69 `PerceptualEvidence` adapter keyed by source/plan/target-window/candidate-generation identity; verify matching evidence is attached to exactly one target by bounded reference and absent, stale, or failing #69 evidence leaves the experiment runnable with an explicit warning.
- [x] 3.4 Reuse existing Frame Fidelity, QSV bitstream/frame-structure, size, bitrate, and speed evidence through injectable providers; verify no second VMAF/SSIM pipeline or unbounded raw-frame/log persistence is introduced.
- [x] 3.5 Define the separate `EncoderControlCandidate` seam for optional non-QSV attribution, including source/window/cadence/filter/normalization/quality-domain identity and generation evidence; verify it is not passed through the QSV one-variable isolation validator and does not auto-encode unless explicitly requested.

## 4. Extend the existing LAB/Compatible Preview lifecycle

- [x] 4.1 Extend `LabFidelityOperation` with explicit QSV experiment orchestration while retaining its existing states, cancellation, stale-run checks, and heavy-work slot; verify normal LAB/Fidelity runs without opt-in perform no variant work.
- [x] 4.2 Reuse the ready primary Compatible Preview artifact only for the primary target and prove zero new baseline encode with request/artifact/generation/canonical-signature identity plus generation-boundary accounting; verify request-registration count alone cannot pass the test.
- [x] 4.3 Reuse or materialize each non-primary target's exact-window baseline through Compatible Preview before its variants; verify a role window cannot use the primary artifact and the #9 evidence provider never raw-encodes.
- [x] 4.4 Generate each explicit variant through Compatible Preview with a distinct candidate/artifact identity nested under exactly one target; verify a completed or failed variant never replaces its target baseline, another target baseline, primary playback, or source-backed Sample A/reference.
- [x] 4.5 Enforce `QSVTemporalExperimentPolicyV1`: maximum 3 target windows, 3 variants per target, 6 total QSV variant encodes, 3 optional controls, 3 baseline materializations, and 12 total heavy encodes; verify caps are deterministic and checked before request creation.
- [x] 4.6 Run baselines, variants, and explicitly requested controls sequentially under the existing heavy-work slot; verify quality × GOP × BF requests do not create a Cartesian product and every variant compares only with its target baseline.
- [x] 4.7 Implement cancellation cleanup for prepare, encode, evidence, artifact protection, and operation bookkeeping; verify cancellation during target B leaves target A evidence bounded and addressable, releases capacity/protection, and leaves the primary playable.
- [x] 4.8 Verify retry/stale-generation behavior never compares mixed target artifact generations, any target baseline/generation change changes experiment identity, and explicit promotion starts a new experiment instead of silently replacing a baseline.

## 5. Expose explicit LAB workflow and evidence

- [x] 5.1 Extend API/client types and the existing LAB action flow for explicit QSV experiment start/status/cancel and bounded variant summaries; verify tab changes, rerenders, and polling alone do not start heavy work.
- [x] 5.2 Preserve the existing primary Preview Ready boundary, Sample A/reference identity, per-target baseline/variant labels, and target-relative progress; verify the UI can distinguish measured temporal evidence, interpretation, unavailable roles, invalid isolation, non-comparable controls, and manual visual-review state.
- [x] 5.3 Add focused frontend coverage for explicit start/cancel/retry, primary and non-primary baseline reuse, per-target variant identity preservation, stale-result fencing, no hidden tab work, and no replacement of the ready primary artifact.

## 6. Classify outcomes and produce safe recommendations

- [x] 6.1 Implement the four target-level result classifications and conservative QSV-specific attribution requiring sufficient QSV temporal evidence, isolated QSV evidence, matching target/control identity, materially absent/reduced control pattern, and no invalidating Frame Fidelity mismatch; verify QSV-only evidence cannot become `reproducible_qsv_specific_pumping` and decoder compatibility is reported separately.
- [x] 6.2 Verify a control with a different window, filter/normalization semantics, or stale generation is rejected, while an existing matching x265/Test Encode/LAB artifact can be reused without a new encode; verify manual “looks better” commentary is not canonical control evidence.
- [x] 6.3 Emit target-owned versioned mitigation recommendations with evidence basis, encoder/runtime, media characteristics, confidence, and size/speed/compression trade-offs; verify recommendations may be emitted from isolated QSV A/B without a control but never mutate existing Profiles automatically.
- [ ] 6.4 Add an explicit draft/new-Profile application path only if the user or policy selects a recommendation; verify unrelated assets and existing Profiles remain unchanged.

> 6.4 remains conditional/deferred: this implementation exposes recommendations but does not add an Apply recommendation action or mutate Profiles.

## 7. Validate without NAS media, network, or hardware

- [x] 7.1 Run backend unit tests through fake/injectable plan, capability, encoder, artifact, temporal-evidence, optional-#69, and control seams; verify three target-owned baselines, primary reuse, role-baseline separation, per-target isolation, deterministic caps, target-B cancellation cleanup, baseline-generation identity changes, and all evidence fixtures pass without NAS paths, network access, real media, or QSV hardware.
- [x] 7.2 Run focused frontend tests/build checks for the changed API and LAB symbols, recording the repository's known global lint baseline separately; verify no unrelated lint baseline is presented as a regression.
- [x] 7.3 Run `openspec validate qsv-grain-blur-pumping --type change --strict` and the canonical `./scripts/verify.sh --auto`; require `ALL CHECKS PASSED` before implementation delivery and confirm no production media or profile data was modified.
