## Purpose

Defines how Validation compares source and output audio/video presentation timing and records enough evidence to explain each per-track decision.

## ADDED Requirements

### Requirement: A/V alignment uses comparable presentation anchors

Validation SHALL calculate introduced A/V offsets from comparable source and output presentation timestamps. Video alignment SHALL use the first presented video frame timestamp. Audio alignment SHALL use a presented audio timestamp or the first audio packet presentation timestamp. Stream-level start timestamps MAY remain diagnostic evidence, but SHALL NOT substitute for a missing presentation anchor in this comparison. Audio tracks SHALL be paired by the frozen stream plan and identity; unavailable or ambiguous pairs SHALL remain `UNVERIFIED`. Validation SHALL report `UNVERIFIED` when required source or output anchors are absent, and SHALL preserve the configured frame thresholds and timestamp quantization tolerance for measurable offsets.

#### Scenario: Source stream start differs from its first presented frame
- **WHEN** source `stream.start_time` is 0 ms, its first presented video frame is 42 ms, output's first presented frame is 42 ms, and corresponding audio presentation starts are both 0 ms
- **THEN** Validation compares the two presentation-relative A/V offsets and reports no introduced offset warning

#### Scenario: Zero-offset H.264 control
- **WHEN** source and output first presented video frames and corresponding audio presentation starts are all 0 ms
- **THEN** Validation reports a validated 0 ms introduced offset

#### Scenario: A measurable introduced offset exceeds policy tolerance
- **WHEN** comparable presentation anchors show an introduced offset beyond the configured warning or mismatch boundary after quantization tolerance
- **THEN** Validation reports the corresponding warning or mismatch and records the signed offset and frame equivalent

#### Scenario: Multiple audio tracks retain frozen identity pairing
- **WHEN** the frozen stream plan maps several source audio tracks to output tracks in a different stream order
- **THEN** Validation reports one comparison for each mapped identity without shifting evidence between tracks

#### Scenario: Presentation timestamps are missing or incomparable
- **WHEN** a required source or output presentation anchor is absent or a track identity cannot be paired
- **THEN** the affected comparison and overall alignment state remain `UNVERIFIED` without substituting `stream.start_time`

#### Scenario: Timestamp quantization lies on a policy boundary
- **WHEN** a measured introduced offset is slightly above the warning-to-mismatch frame boundary but within the existing bounded time-base quantization tolerance
- **THEN** Validation retains the warning decision without moving the configured pass boundary, and records the measured value and tolerance

### Requirement: A/V findings persist bounded, provenance-bearing track details

Each new Validation finding for A/V alignment SHALL retain the existing expected source evidence, actual output evidence, rule identity, and Result binding. It SHALL additionally persist per-track source and output relative offsets, introduced offset in milliseconds and frames, policy and quantization tolerances, timestamp evidence provenance, and the final decision. Validation SHALL reuse comparable evidence already present in the matching Queue or Result probe. If it is missing, any read-only fallback probe SHALL inspect at most the initial two seconds of each required input; evidence still unavailable after that probe SHALL remain `UNVERIFIED`. A/V evidence collection SHALL NOT scan the full media or run when Review Center or History list views are opened or refreshed.

#### Scenario: A bounded fallback supplies missing presentation evidence
- **WHEN** the matching Queue or Result probes lack presentation timestamps and the initial bounded probe finds them
- **THEN** the finding records their source, output, units, provenance, per-track comparison values, tolerances, and decision

#### Scenario: Bounded probing cannot establish comparable evidence
- **WHEN** the bounded read-only probe fails or does not find required presentation timestamps
- **THEN** the new finding records the evidence limitation and `UNVERIFIED` without inventing offsets

#### Scenario: Validation is rerun for the same Result
- **WHEN** a new Validation run evaluates the same canonical Result and execution identities
- **THEN** it records updated findings in a new immutable run while prior ValidationRuns and Result identities remain unchanged

#### Scenario: Publisher consumes a new A/V finding
- **WHEN** Publisher receives a result after the updated A/V rule is evaluated
- **THEN** Publisher continues to use the canonical Validation outcome and existing eligibility behavior without a new bypass or alternate Result identity
