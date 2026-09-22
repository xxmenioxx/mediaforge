## Purpose

Defines one authoritative contract for resolving HEVC Level/Tier from output
requirements, rendering it into an encode, validating the resulting bitstream,
and reporting expected versus actual Fidelity evidence.

## ADDED Requirements

### Requirement: Resolve HEVC Level/Tier from final output requirements

The system SHALL resolve the HEVC Level/Tier decision from the effective output
geometry and effective output frame rate after all already-approved output
transform decisions, together with any applicable bounded output rate-control
constraints. The system MUST NOT use source average bitrate as an output
requirement. Preview, Test Encode, Queue, command rendering, and final
validation SHALL consume the same resolved decision.

#### Scenario: Source bitrate does not raise an unbounded Auto decision

- **WHEN** an HEVC output uses an unbounded quality mode and the source has a
  high average bitrate
- **THEN** Auto resolves from effective output geometry and frame rate only and
  does not raise the level solely because of source bitrate

#### Scenario: Resolved cadence and geometry drive the decision

- **WHEN** cadence, scaling, or an approved output transform changes the output
  frame rate or dimensions
- **THEN** the resolved Level/Tier uses those effective output values rather than
  the source stream values

#### Scenario: All entry points expose one decision

- **WHEN** the same effective profile and output inputs are used for Preview,
  Test Encode, and Queue
- **THEN** each entry point exposes identical expected HEVC Level/Tier and
  enforcement metadata, apart from transport-specific command arguments

### Requirement: Distinguish Auto, Recommended, and explicit Level intent

The system SHALL distinguish Auto, Recommended, and explicit Level intent.
Auto SHALL resolve the lowest supported Main-Tier Level that satisfies known
output requirements. Recommended SHALL be derived data whose input fingerprint
controls reuse. Explicit Level SHALL remain separate user intent and SHALL
undergo compatibility preflight without replacement by Recommended data or
silent up-leveling.

#### Scenario: Auto selects the minimum supported level

- **WHEN** effective geometry, frame rate, and bounded output constraints are
  known
- **THEN** Auto selects the lowest supported Main-Tier Level satisfying all of
  those constraints and records the limiting requirement

#### Scenario: Recommended fingerprint matches

- **WHEN** a stored Recommended Level fingerprint matches current effective
  geometry, frame rate, encoder, and applicable rate-control inputs
- **THEN** the system reuses the stored recommendation as the current derived
  value

#### Scenario: Recommended fingerprint changes with sufficient inputs

- **WHEN** the stored Recommended Level fingerprint differs from current inputs
  and current geometry, frame rate, and applicable rate-control inputs are
  sufficient
- **THEN** the system synchronously recomputes Recommended before command
  planning and does not pass the stale value to command generation

#### Scenario: Recommended fingerprint changes with insufficient inputs

- **WHEN** the stored Recommended Level fingerprint differs and current inputs
  are insufficient for recomputation
- **THEN** the decision is unresolved, the stale value is not authoritative, no
  Level/Tier enforcement is emitted from it, and the operation does not report
  a valid resolved Level/Tier plan

#### Scenario: Explicit Level is incompatible with current requirements

- **WHEN** an explicit Level cannot satisfy current effective geometry, frame
  rate, or bounded output constraints
- **THEN** planning reports an incompatible explicit constraint and does not emit
  a valid command plan that presents that Level as valid

#### Scenario: The decision becomes immutable at execution preparation

- **WHEN** Preview or Test Encode reaches command preparation, or Queue reaches
  PREPARING and freezes its effective configuration
- **THEN** the resolved Level/Tier decision and its input evidence become
  immutable for that operation; later profile or asset changes cannot alter the
  command or final validation expectation

### Requirement: Include only real bounded rate-control constraints

The system SHALL include a bounded output rate-control constraint in Level/Tier
resolution only when that bound is actually applied to the planned output. A
resolved maxrate, VBV/HRD ceiling, or equivalent encoder constraint SHALL be
preferred over a target or average bitrate. For QSV VBR and CBR, the effective
target/maxrate/buffer values rendered by the command SHALL be the source of the
bounded planning input; a target SHALL count as a peak bound only when the
effective command also makes it the maxrate. Unbounded CRF, ICQ, and equivalent
quality modes MUST NOT receive a synthetic peak bitrate from source metadata.

#### Scenario: QSV maxrate raises the required level

- **WHEN** QSV VBR or CBR resolves a maxrate above a lower Main-Tier Level's
  MaxBR limit while geometry and frame rate still fit that level
- **THEN** Auto/Recommended selects the next supported level whose constraints
  include the effective QSV maxrate

#### Scenario: QSV target is not mistaken for a VBR peak

- **WHEN** QSV VBR has a target bitrate but no effective maxrate bound
- **THEN** the target is not used as MaxBR evidence and peak rate is marked
  `unverified` until the rendered command establishes a maxrate

#### Scenario: Unbounded quality mode has no invented rate limit

- **WHEN** the output uses CRF, ICQ, or another unbounded quality mode without
  VBV/HRD/maxrate constraints
- **THEN** Level/Tier resolution uses geometry and frame rate and does not derive
  a limit from ffprobe average bitrate

### Requirement: Render the resolved decision without silent quality changes

The system SHALL render the frozen resolved Level/Tier decision into the
encoder-specific command contract. QSV commands SHALL receive the resolved
Level and supported Tier. x265 enforcement SHALL be deterministic:

- Auto or Recommended with no real bounded peak-rate constraint SHALL use
  advisory enforcement and SHALL NOT inject `level-idc` or `high-tier`.
- Auto or Recommended with a real bounded maxrate/VBV constraint SHALL use hard
  enforcement after compatibility preflight. Every maxrate, buffer, and VBV
  consequence SHALL already be represented in the canonical frozen decision
  before command rendering.
- Explicit Level SHALL use hard enforcement after compatibility preflight. An
  incompatible explicit Level SHALL prevent a valid command plan rather than be
  silently up-leveled.

The command MUST NOT silently replace the user's quality semantics.

#### Scenario: QSV command matches the frozen plan

- **WHEN** a prepared QSV plan resolves a Level and Main Tier
- **THEN** the rendered command contains the corresponding Level/Tier values
  and the command's rate-control arguments match the constraints used to
  resolve the decision

#### Scenario: x265 Auto preserves unbounded quality semantics

- **WHEN** x265 Auto or Recommended resolves a geometry/FPS Level but no real
  bounded peak-rate constraint exists
- **THEN** the plan marks enforcement advisory and the rendered command omits
  `level-idc` and `high-tier`

#### Scenario: x265 bounded Auto or Recommended is hard-enforced

- **WHEN** x265 Auto or Recommended resolves with a real bounded maxrate/VBV
  constraint and compatibility preflight succeeds
- **THEN** the frozen decision contains the resulting maxrate, buffer, and VBV
  consequences before the command renders hard Level/Tier enforcement

#### Scenario: Explicit x265 Level is hard-enforced or rejected

- **WHEN** an explicit x265 Level passes compatibility preflight
- **THEN** the command hard-enforces that Level/Tier and uses the consequences
  recorded in the frozen decision

- **WHEN** an explicit x265 Level fails compatibility preflight
- **THEN** planning rejects the valid command plan and does not silently
  up-level the request

### Requirement: Validate Level/Tier and peak-rate evidence from the output

For applicable HEVC validation and Fidelity fields, the system SHALL use exactly
these statuses:

- `validated`: authoritative evidence exists and matches the claim.
- `mismatch`: authoritative evidence exists and differs from the claim.
- `unavailable`: authoritative evidence could not be obtained, including an
  unsupported runtime, missing or invalid parameter-set evidence, a legacy
  output with no stored evidence, or a failed bounded probe.
- `unverified`: evidence is available but is insufficient to prove the claim,
  such as ffprobe average `bit_rate` for MaxBR/CPB conformance.

Mismatch MUST NOT be inferred from insufficient evidence. Ordinary ffprobe
stream metadata, including average `bit_rate`, SHALL be retained only as
informational evidence and MUST NOT prove HEVC MaxBR or CPB peak conformance.
Final `general_level_idc` and `general_tier_flag` claims SHALL require bounded
bitstream-header evidence. If the runtime cannot obtain or parse that evidence,
the corresponding actual value SHALL be unavailable rather than inferred from
the command, profile, or average bitrate.

#### Scenario: Header evidence validates Level and Tier

- **WHEN** bounded inspection of the output bitstream provides
  `general_level_idc` and `general_tier_flag` for the selected video stream and
  both match the frozen expected decision
- **THEN** validation reports both comparisons as `validated`

#### Scenario: Header evidence disagrees with the plan

- **WHEN** authoritative header evidence reports a Level or Tier different from
  the frozen expected decision
- **THEN** validation reports that comparison as `mismatch` with expected and
  actual values

#### Scenario: Header inspection is unavailable

- **WHEN** the runtime lacks the required bounded header inspection capability,
  the stream has no parseable parameter set, a legacy output has no stored
  evidence, or the bounded probe fails
- **THEN** actual Level/Tier evidence is reported as `unavailable`, not as the
  expected planned value

#### Scenario: ffprobe average bitrate is present but peak evidence is absent

- **WHEN** ffprobe reports an average video `bit_rate` but no output evidence
  represents MaxBR, CPB, or another peak-rate bound
- **THEN** the average is retained as informational and the peak-rate claim is
  reported `unverified`, never `validated` solely from that average

### Requirement: Reuse one stored output evidence record per validation

For each applicable HEVC output validation, the system SHALL run the bounded
`trace_headers` probe at most once. The parsed Level/Tier evidence, probe
outcome, and evidence statuses SHALL be stored in the existing validation
report or job-artifact boundary used by the operation. Fidelity and report
refreshes SHALL consume that stored record and MUST NOT rerun FFmpeg merely to
render the report. The system SHALL NOT introduce a new global cache or
persistence subsystem for this reuse.

#### Scenario: One probe result serves validation and Fidelity

- **WHEN** an HEVC output validation runs the bounded header probe
- **THEN** validation stores its parsed evidence and later Fidelity/report
  refreshes read that record without another FFmpeg invocation

#### Scenario: A failed probe is also reused

- **WHEN** the single bounded probe attempt fails or produces invalid parameter
  set evidence
- **THEN** the stored result remains `unavailable` for the validation and all
  report refreshes reuse that outcome without retrying solely for rendering

#### Scenario: Legacy validation remains readable

- **WHEN** an existing validation report or job artifact predates stored header
  evidence
- **THEN** it remains readable and affected actual fields report `unavailable`
  without backfilling them from planned settings

### Requirement: Report HEVC Fidelity with consistent expected and actual states

Fidelity SHALL report expected Level, expected Tier, actual Level, actual Tier,
rate-control evidence, and the exact evidence status from the stored validation
record using the same immutable plan consumed by encoding. A planned Tier MUST
NOT be presented as an observed Tier unless output bitstream evidence supports
it. The overall result can be `validated` only when required authoritative
evidence exists and matches; differing authoritative evidence is `mismatch`,
missing authoritative evidence is `unavailable`, and insufficient evidence is
`unverified`.

#### Scenario: Validated output reports both sides

- **WHEN** stored output header evidence supports actual Level and Tier and both
  match the frozen plan
- **THEN** Fidelity reports expected and actual values separately and marks the
  Level/Tier comparison `validated`

#### Scenario: Missing Tier evidence does not default to Main

- **WHEN** the plan expects Main Tier but stored output inspection cannot
  establish `general_tier_flag`
- **THEN** Fidelity retains expected Tier as Main, reports actual Tier as
  unavailable, and does not report observed Tier as Main

#### Scenario: A Level/Tier mismatch is actionable

- **WHEN** stored authoritative output evidence differs from expected Level or
  Tier
- **THEN** Fidelity reports `mismatch` with expected and actual values and
  identifies the failed comparison rather than collapsing it into a generic
  bitrate or codec warning

#### Scenario: Insufficient peak evidence is not a mismatch

- **WHEN** only average ffprobe bitrate is available for a MaxBR/CPB claim
- **THEN** Fidelity reports that claim as `unverified`, not `validated` or
  `mismatch`
