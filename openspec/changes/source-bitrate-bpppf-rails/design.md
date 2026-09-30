# Design: Source Bitrate Evidence and BPPPF Rails

## Existing authorities

- `ScanResult` is the persisted source-analysis record and already carries a
  source fingerprint plus independently versioned analysis components.
- `quality` is the shared backend recommendation package used by Profiles,
  Asset Overrides, and LAB.
- Queue preparation resolves effective geometry and cadence/FPS before it
  freezes the prepared profile. Batch Preflight persists that same profile and
  accepts it through the existing `QueueSourceAnalysisEvidenceKey` and
  `ExecutionPlan` contracts.
- Perceptual LAB evidence remains measured, candidate-bound evidence. The
  BPPPF rail adds context and cannot create a quality verdict or override a
  measured LAB decision.

## Source bitrate resolution

Add one deterministic selector in `internal/quality`. It selects the primary
video stream and applies this provenance order:

1. Positive ffprobe video-stream bitrate (`stream_metadata`, high confidence).
2. Non-stale Matroska `BPS` or `NUMBER_OF_BYTES` per-stream statistics
   (`matroska_statistics`, high confidence).
3. Bounded packet samples for that video stream (`packet_sampling`, medium
   confidence with enough successful windows, otherwise low).
4. Container bitrate minus known audio/subtitle bitrate (`container_bitrate`,
   low confidence).
5. File size divided by duration, minus known non-video tracks
   (`file_size_duration`, low confidence).

The evidence stores the selected bitrate, optional sample range, method,
confidence, stream index, duration, requested/successful windows, individual
window outcomes, and schema version. A full-file scan is never used. Sampling is
limited to five unique interior positions, a short interval per position, a
small captured-output cap, and a per-window deadline. Unsupported or
unseekable inputs fall through to weaker evidence.

Persist the result as a field on `ScanResult`, not a second table. Add one
`sourceBitrate` analysis component version so older snapshots refresh only this
component through the existing incremental scanner path.

## Planning rail

The calculation is:

```text
BPPPF = effective target video bitrate
        / (effective output width × effective output height × effective FPS)
```

Do not calculate when the target or any effective output input is unknown.
Encoder assumptions are a small versioned table keyed by H.264/HEVC/AV1,
software/hardware class, SDR/HDR, and the saved optimization intent. They define
warning references only; they are not bitrate-equivalence claims. The rail
returns `normal`, `warning`, `high-risk`, or `unavailable`, explains the
calculated value and reference, and never edits the selected target.

Measured LAB output and perceptual evidence remain the stronger evidence on LAB
surfaces. Because those metrics are not universal pass/fail thresholds, the
rail remains labeled heuristic context beside them; it cannot demote a measured
result or create a LAB verdict.

## Queue and freeze behavior

Queue Preflight copies the canonical evidence and rail explanation into the
existing source-analysis evidence manifest and candidate warning list. The
accepted candidate persists the exact resolved profile and evidence into the
existing Queue snapshot; ExecutionPlan copies the manifest into
`DecisionSources`. ReviewPlanner may project that frozen assessment but must
not sample again or derive a different rail. Worker execution continues to
consume the frozen profile and does not apply the warning as a bitrate change.

## Validation

Unit tests cover precedence, audio-heavy inputs, tag staleness, deterministic
bounded packet windows and partial failures, BPPPF factor boundaries, LAB
precedence, analysis-component version refresh, and exact Queue snapshot reuse.
