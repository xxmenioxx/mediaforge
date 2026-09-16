## 1. Canonical planning and artifact model

- [x] 1.1 Inventory the existing Pipeline subtitle resolver, transform plan, and artifact metadata; document the integration points and verify the inventory against `specs/subtitles/spec.md`.
- [x] 1.2 Extend the canonical decision model to distinguish text versus bitmap sources, original versus converted mode, requested format, OCR requirement, stages, extension, and stable naming; verify with unit tests for default SRT and explicit ASS.

## 2. Text extraction and converted outputs

- [x] 2.1 Route text subtitle conversion from SRT/ASS/SSA-compatible streams through the canonical planner and renderer; verify converted SRT and ASS artifacts contain normalized cues.
- [x] 2.2 Preserve stream-copy extraction for original ASS and SSA, including `.ass`/`.ssa` extensions and source formatting; verify styles and metadata are not routed through conversion.
- [x] 2.3 Reuse only valid artifacts already identified by the existing canonical pipeline, at most once per matching transform, without adding sidecar discovery or matching semantics; verify planned and generated artifact metadata remains consistent.

## 3. Bitmap OCR outputs

- [x] 3.1 Allow PGS and VobSub requests to reach the OCR-capable path before rejection; verify OCR produces normalized cues for converted SRT.
- [x] 3.2 Feed the same normalized OCR cues to converted ASS output and report explicit failure when OCR is unavailable or yields no supported result; verify no misleading artifact is created.

## 4. Entry-point integration

- [x] 4.1 Update Asset Info Convert/Generate actions to consume the canonical subtitle plan and artifact outputs; verify parity with an equivalent Pipeline request.
- [x] 4.2 Update Track Profile subtitle output selection to use the canonical planner while retaining explicit policy precedence; verify profile-level SRT and ASS scenarios.
- [x] 4.3 Update Profile Lab subtitle preview/generation to consume canonical decisions without duplicating extraction, OCR, or format selection; verify Lab output metadata matches Pipeline.
- [x] 4.4 Ensure Queue snapshots and Worker execution/retry use the frozen canonical artifact plan; verify retries consume the snapshot and do not silently re-resolve legacy snapshots.

## 5. Regression coverage and validation

- [x] 5.1 Add focused backend tests covering text-to-SRT, text-to-ASS, default SRT, original ASS/SSA, PGS/VobSub OCR, unsupported OCR failure, sidecar satisfaction, and artifact extensions.
- [x] 5.2 Add focused Asset Info, Track Profile, and Lab tests proving equivalent requests resolve to identical canonical subtitle plans and metadata.
- [x] 5.3 Run `./scripts/verify.sh --auto` and verify it reports `ALL CHECKS PASSED`.
