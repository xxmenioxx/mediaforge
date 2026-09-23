## 1. Reproduction and FFmpeg capability evidence

- [ ] 1.1 Capture the current validated-IVTC behavior and worker FFmpeg capability contract for `fieldmatch`, `bwdif=deint=interlaced`, `send_frame`, `combmatch`, `decimate`, and progressive metadata; verify the evidence is based on the actual supported runtime and does not modify repository or production media.
- [ ] 1.2 Add a deterministic disposable synthetic mixed-telecine fixture/harness that contains clean matched content and a controlled cadence-breaking/interlaced segment for both TFF and BFF; verify the current `fieldmatch,decimate` chain reproduces a residual-combing observation without committing copyrighted media.
- [ ] 1.3 Record the fixture's expected frame flags, frame counts, duration, FPS, and field metadata at fieldmatch, selective-deinterlace, and decimate boundaries; verify the reproduction is stable enough to serve as an integration regression.

## 2. Canonical resolved motion plan

- [ ] 2.1 Extend the existing effective motion decision and frozen worker snapshot with the smallest effective field-order, residual-fallback, and rationale state while keeping requested cadence settings distinct; verify old snapshots default safely and the decision is inspectable without a new plan engine or migration.
- [ ] 2.2 Resolve the fallback only after the existing IVTC validation boundary accepts a field order, using explicit valid order before validated detected order and review/unknown otherwise; verify suspected telecine, conflicting order, progressive input, and native interlaced input do not gain the fallback.
- [ ] 2.3 Preserve the existing cadence and FPS resolver contract while enabling the fallback; verify the effective operation, output FPS, progressive state, and warning/unknown fields remain internally consistent for 24000/1001 IVTC and non-IVTC cases.

## 3. Selective filter rendering

- [ ] 3.1 Extend canonical IVTC rendering to place selective frame-preserving deinterlacing between fieldmatch and decimate, using the resolved parity and supported comb-marking configuration; verify the exact TFF and BFF chains, ordering, and single progressive metadata stage with focused command tests.
- [ ] 3.2 Keep native interlaced deinterlacing on the existing canonical full-frame behavior and keep progressive sources free of automatic motion filters; verify existing `bwdif=deint=all` and progressive no-op regressions remain green.
- [ ] 3.3 Verify clean fieldmatched frames are not unnecessarily deinterlaced and residual marked frames are repaired; use the synthetic runtime harness to assert frame flags, frame count before decimate, detail/checksum preservation for clean frames, and removal of residual comb markers after the fallback.
- [ ] 3.4 Verify unsupported FFmpeg capability or unresolved field order fails conservatively to the existing review/unsupported state rather than selecting an all-frame fallback or hard-coded parity.

## 4. Cross-surface parity and Queue freezing

- [ ] 4.1 Route Profile Lab automatic motion consumption through the backend-resolved motion decision while retaining restoration-only UI filters; verify LAB, Compatible Preview, Test Encode, and final MediaJobPlan produce equivalent resolved motion fields and filter prefixes for identical frozen inputs.
- [ ] 4.2 Preserve the resolved fallback, parity, progressive state, and FPS in Queue's immutable preparation snapshot; verify changing mutable profile or analysis settings after freeze cannot change the worker command or effective motion decision.
- [ ] 4.3 Keep analysis, resolution, rendering, and execution separate; verify no Preview/LAB/Queue path performs an independent residual-combing classification or duplicate cadence probe.

## 5. Validation and acceptance coverage

- [ ] 5.1 Extend cadence/output validation coverage to compare measured frame count/duration and stream FPS with the frozen resolved plan, including intentional progressive field metadata and unchanged 24000/1001 cadence; verify mismatches remain bounded diagnostics rather than invented rates.
- [ ] 5.2 Add focused regressions for clean telecine, residual combing, progressive source, native interlaced source, TFF, BFF, unresolved parity, and FPS/frame-count integrity; verify each scenario asserts resulting behavior or output evidence rather than filter-string presence alone where the runtime harness can do so.
- [ ] 5.3 Run separate manual validation on a user-owned affected sample through Preview, Test Encode, and Queue; verify visual combing removal, clean-IVTC detail/motion preservation, progressive output signaling, and frozen-plan parity without copying or committing the sample.

## 6. Repository verification and handoff

- [ ] 6.1 Run the focused backend cadence/interlace/media-plan and relevant frontend tests after implementation; verify all changed Go files are formatted and no unrelated policy changes are introduced.
- [ ] 6.2 Run `./scripts/verify.sh --auto` and `openspec validate residual-combing-telecine --type change --strict`; mark this checklist complete only after canonical verification reports `ALL CHECKS PASSED` and the change remains unarchived for review.
