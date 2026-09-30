## ADDED Requirements

### Requirement: Source bitrate SHALL have one versioned provenance contract

The system SHALL resolve the primary source video bitrate through video-only
evidence before container estimates, in this order: trusted video-stream
metadata, valid Matroska per-stream statistics, bounded distributed
video-packet sampling, container bitrate adjusted by known non-video tracks,
and file size divided by duration. The persisted evidence SHALL identify its
method, confidence, stream index, duration, schema version, and sampled-window
outcomes when sampling was attempted. Confidence SHALL describe evidence
provenance, not desirability.

#### Scenario: Stronger video-only evidence wins

- **WHEN** video-stream metadata or valid Matroska statistics exist alongside
  a higher container bitrate caused by audio or other tracks
- **THEN** the system SHALL select the video-only evidence and SHALL NOT
  inflate the source video bitrate with the container value

#### Scenario: Sampling is the next source after missing stream evidence

- **WHEN** no trusted video-stream bitrate or valid Matroska statistics exist
  and the source format supports bounded seeks
- **THEN** the system SHALL sample deterministic distributed video-packet
  windows before using container or file-size estimates

#### Scenario: Sampling remains bounded and explainable

- **WHEN** packet sampling runs for a source asset
- **THEN** it SHALL use no more than five windows, bound bytes and elapsed time,
  select video packets where supported, record requested and successful
  windows, tolerate partial failures, and never read the full file

#### Scenario: Improved evidence logic refreshes only its component

- **WHEN** a persisted `ScanResult` has current source identity but an old or
  missing source-bitrate evidence version
- **THEN** the scanner SHALL refresh the source-bitrate component through the
  existing analysis cache without invalidating unrelated valid components

### Requirement: BPPPF SHALL be a planning warning, not an automatic override

When target video bitrate and effective output width, height, and FPS are
known, the system SHALL calculate
`target bitrate / (width × height × FPS)` and compare it with one versioned
planning reference selected by codec, software/hardware encoder class, SDR/HDR,
and optimization intent. The response SHALL expose the calculated value,
reference, effective inputs, and an explanation. It SHALL use warning severity
such as `normal`, `warning`, or `high-risk`; it SHALL NOT silently change user
settings or treat BPPPF as a perceptual-quality verdict.

#### Scenario: Effective output values drive the calculation

- **WHEN** a plan changes source resolution or frame rate
- **THEN** the rail SHALL use the resolved output geometry and FPS from that
  plan, not source dimensions or source FPS

#### Scenario: A candidate crosses a planning rail

- **WHEN** an estimated target is below the applicable BPPPF reference
- **THEN** the system SHALL explain the estimate, reference, output geometry/FPS,
  codec, encoder class, HDR class, and optimization intent without changing the
  target or blocking Queue solely for that warning

#### Scenario: A required planning input is unknown

- **WHEN** target bitrate, effective geometry, effective FPS, or supported
  codec/encoder assumptions are unavailable
- **THEN** the rail SHALL be `unavailable` and SHALL not substitute source
  values or a guessed universal factor

### Requirement: Measured LAB evidence SHALL remain authoritative

The BPPPF rail SHALL remain heuristic context beside candidate-bound measured
LAB evidence. It SHALL NOT demote a measured LAB result, create a LAB pass/fail
verdict, or supersede a validated candidate decision solely because a generic
BPPPF threshold was crossed.

#### Scenario: LAB has measured candidate evidence

- **WHEN** a matching candidate has measured LAB evidence and its BPPPF rail is
  warning or high-risk
- **THEN** the LAB surface SHALL preserve and prioritize the measured evidence
  while labeling BPPPF as heuristic context

### Requirement: Queue SHALL freeze reviewed quality evidence

Queue Preflight SHALL expose the source-bitrate evidence and BPPPF warning using
the existing source-analysis and frozen-plan snapshot mechanisms. Acceptance
SHALL preserve the reviewed evidence and warning in the accepted Queue and
ExecutionPlan provenance. Worker execution SHALL NOT recompute the evidence or
change the frozen quality configuration because of a later source or policy
change.

#### Scenario: Reviewed evidence survives acceptance

- **WHEN** a Preflight candidate with source bitrate evidence and a BPPPF
  warning is accepted at its reviewed revision
- **THEN** the Queue snapshot and ExecutionPlan SHALL retain that same evidence
  and warning, and the Worker SHALL execute the exact frozen plan

#### Scenario: Source metadata changes after review

- **WHEN** source metadata or rail policy changes after the Queue plan is
  accepted
- **THEN** the accepted plan SHALL retain its reviewed evidence and SHALL NOT
  be re-resolved by Worker execution
