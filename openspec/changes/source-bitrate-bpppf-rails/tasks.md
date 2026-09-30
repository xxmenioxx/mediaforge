## 1. Canonical source bitrate evidence

- [x] 1.1 Add the versioned evidence model and pure precedence resolver with
  provenance confidence and audio/subtitle-aware weak fallbacks.
- [x] 1.2 Add deterministic bounded distributed video-packet sampling with
  limits, short-asset behavior, and per-window outcome recording.
- [x] 1.3 Persist evidence on `ScanResult`; version and incrementally refresh
  only the source-bitrate analysis component.
- [x] 1.4 Reuse the canonical evidence in source quality recommendation and
  Advisor; remove competing source-bitrate inference paths.

## 2. BPPPF planning rail

- [x] 2.1 Add the versioned codec/encoder-class/SDR-HDR/goal reference model and
  calculate BPPPF from effective bitrate, output dimensions, and FPS.
- [x] 2.2 Expose rail severity and explanation in Quality Recommendation,
  Profiles, LAB, and Advisor without mutating selected settings.
- [x] 2.3 Preserve LAB measured-evidence precedence and contextual-only rail
  behavior.

## 3. Queue and Preflight

- [x] 3.1 Carry source evidence and rail warning through the existing #46
  source-analysis manifest and reviewed candidate response.
- [x] 3.2 Preserve the exact reviewed evidence in accepted Queue snapshots and
  ExecutionPlan evaluation; verify Worker consumes the frozen plan.

## 4. Documentation and validation

- [x] 4.1 Update the implemented Queue Preflight architecture note.
- [x] 4.2 Add focused resolver, sampling, BPPPF, LAB, and Queue regression tests.
- [x] 4.3 Run strict OpenSpec validation and `./scripts/verify.sh --auto`.
