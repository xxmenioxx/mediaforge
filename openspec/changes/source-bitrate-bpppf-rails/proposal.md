# Source Bitrate Evidence and BPPPF Safety Rails

Add one versioned source-video bitrate evidence contract to the existing
`ScanResult` analysis cache, then reuse it for quality recommendations, Advisor,
LAB context, and Queue review. Add a BPPPF planning warning based on the
effective output bitrate, geometry, frame rate, codec, encoder class, dynamic
range, and optimization goal.

The rail is advisory. It never rewrites profile bitrate/quality settings or
replaces measured LAB evidence. Queue review freezes the evidence and warning
through the existing source-analysis and ExecutionPlan snapshot contracts; a
Worker must continue to execute the frozen profile without re-resolving quality.

## Scope

- Resolve source bitrate in this order: video-stream bitrate, Matroska
  per-stream statistics, bounded distributed video-packet samples, container
  bitrate adjusted by known non-video tracks, then file-size/duration.
- Record method, confidence, range where sampling supports one, stream index,
  duration, successful/requested windows, and a schema version.
- Sample at most five deterministic windows, with per-window byte and time
  limits; reuse the versioned `ScanResult` cache and refresh only this analysis
  component when required.
- Calculate BPPPF only when effective target bitrate, geometry, and frame rate
  are known. Use small, versioned planning assumptions rather than universal
  encoder conversion factors.
- Expose the explanation in Quality Recommendation, Profiles, LAB, Advisor,
  and Queue Preflight/ReviewPlanner where those surfaces already have the
  necessary evidence.

Out of scope: automatic bitrate edits, an encoder support matrix, new LAB
lifecycle behavior, new Queue state, a Worker-side resolver, and a new analysis
store.
