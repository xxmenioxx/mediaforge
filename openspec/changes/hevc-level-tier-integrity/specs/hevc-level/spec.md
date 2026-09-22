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
- **THEN** Auto resolves from effective output geometry and frame rate only,
  records that peak-rate evidence is unavailable, and does not raise the level
  solely because of the source bitrate

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
output requirements. Recommended SHALL be derived data, not an immutable
authority: a stored recommendation MUST be invalidated or recomputed when
effective geometry, effective frame rate, or applicable bounded rate-control
inputs change. An explicit Level SHALL remain user intent, but the system MUST
preflight it against current output requirements and MUST surface an
incompatible constraint instead of silently treating it as valid or silently
up-leveling it.

#### Scenario: Auto selects the minimum supported level

- **WHEN** effective geometry, frame rate, and bounded output constraints are
  known
- **THEN** Auto selects the lowest supported Main-Tier Level satisfying all of
  those constraints and records the limiting requirement

#### Scenario: Recommended data is stale after an output change

- **WHEN** a stored Recommended Level was calculated for one geometry, frame
  rate, or bounded rate-control set and any of those inputs changes
- **THEN** the stored recommendation is not reused silently; the system
  recomputes it or marks it stale before command generation and exposes the new
  decision and reason

#### Scenario: Explicit Level is below current requirements

- **WHEN** an explicit Level cannot satisfy the current effective geometry,
  frame rate, or bounded output constraint
- **THEN** planning reports an incompatible Level constraint and does not emit a
  command that presents the incompatible Level as valid

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
- **THEN** the target is not used as MaxBR evidence; the decision records peak
  rate as unavailable unless the rendered command establishes a maxrate

#### Scenario: Unbounded quality mode has no invented rate limit

- **WHEN** the output uses CRF, ICQ, or another unbounded quality mode without
  VBV/HRD/maxrate constraints
- **THEN** Level/Tier resolution uses geometry and frame rate, marks bitrate
  confidence as unavailable or unverified, and does not derive a limit from
  ffprobe average bitrate

### Requirement: Render the resolved decision without silent quality changes

The system SHALL render the frozen resolved Level/Tier decision into the
encoder-specific command contract. QSV commands SHALL receive the resolved
Level and supported Tier. For x265, hard `level-idc` enforcement MUST be
explicit in the resolved plan because it can introduce VBV behavior and change
CRF quality semantics. Auto or Recommended x265 decisions without a real
bounded peak-rate constraint SHALL use an advisory enforcement state that
preserves unbounded quality behavior; bounded or explicit decisions MAY use hard
enforcement only when the plan surfaces its rate-control impact. The command
MUST NOT silently replace the user's quality semantics.

#### Scenario: QSV command matches the frozen plan

- **WHEN** a prepared QSV plan resolves a Level and Main Tier
- **THEN** the rendered command contains the corresponding Level/Tier values
  and the command's rate-control arguments match the constraints used to
  resolve the decision

#### Scenario: x265 Auto preserves unbounded quality semantics

- **WHEN** x265 Auto resolves a geometry/FPS Level but no bounded peak-rate
  constraint exists
- **THEN** the plan marks Level enforcement as advisory, does not silently add
  VBV limits through `level-idc`, and reports that final conformance evidence
  may be unavailable or differ

#### Scenario: Explicit x265 enforcement is visible

- **WHEN** a user explicitly selects an x265 Level or a bounded plan requires
  hard enforcement
- **THEN** the plan records the enforcement and any resulting VBV/rate-control
  constraint, and the rendered command applies that same decision

### Requirement: Validate Level/Tier and peak-rate evidence from the output

The system SHALL validate the final output against the immutable expected
decision and SHALL distinguish evidence types. Ordinary ffprobe stream
metadata, including average `bit_rate`, MAY be retained as informational
evidence but MUST NOT prove HEVC MaxBR or CPB peak conformance. Final
`general_level_idc` and `general_tier_flag` claims SHALL require bounded
bitstream-header evidence. If the runtime cannot obtain or parse that evidence,
the corresponding actual value SHALL remain unavailable/unverified rather than
being inferred from the command, profile, or average bitrate.

#### Scenario: Header evidence validates Level and Tier

- **WHEN** bounded inspection of the output bitstream provides
  `general_level_idc` and `general_tier_flag` for the selected video stream
- **THEN** validation compares both actual values with the expected Level/Tier
  and reports each as validated when they match

#### Scenario: ffprobe average bitrate is present but peak evidence is absent

- **WHEN** ffprobe reports an average video `bit_rate` but no output evidence
  represents MaxBR, CPB, or another peak-rate bound
- **THEN** average bitrate is displayed as informational and the peak-rate
  conformance status remains unavailable or unverified, never validated solely
  from that average

#### Scenario: Header inspection is unavailable

- **WHEN** the runtime lacks the required bounded header inspection capability,
  the stream has no parseable parameter set, or inspection fails
- **THEN** Level/Tier actual evidence is reported as unavailable or unverified,
  not as the expected planned value

#### Scenario: Output Level or Tier differs from the plan

- **WHEN** authoritative output evidence reports a Level or Tier different from
  the frozen expected decision
- **THEN** validation reports a mismatch with both expected and actual values
  and the overall Fidelity result cannot claim validated conformance

### Requirement: Report HEVC Fidelity with consistent expected and actual states

Fidelity SHALL report expected Level, expected Tier, actual Level, actual Tier,
rate-control evidence, and an evidence-aware status using the same immutable
plan consumed by encoding. It SHALL distinguish `validated`, `mismatch`, and
`unavailable`/`unverified` evidence. A planned Tier MUST NOT be presented as an
observed Tier unless output bitstream evidence supports it.

#### Scenario: Validated output reports both sides

- **WHEN** output header evidence supports actual Level and Tier and both match
  the frozen plan
- **THEN** Fidelity reports expected and actual values separately and marks the
  Level/Tier comparison validated

#### Scenario: Missing Tier evidence does not default to Main

- **WHEN** the plan expects Main Tier but output inspection cannot establish
  `general_tier_flag`
- **THEN** Fidelity retains expected Tier as Main, leaves actual Tier
  unavailable/unverified, and does not report observed Tier as Main

#### Scenario: A Level/Tier mismatch is actionable

- **WHEN** actual output evidence differs from expected Level or Tier
- **THEN** Fidelity reports the mismatch with expected and actual values and
  identifies the failed comparison rather than collapsing it into a generic
  bitrate or codec warning

#### Scenario: Legacy output lacks new evidence

- **WHEN** an existing output or stored validation record predates header
  evidence support
- **THEN** it remains readable and is reported as unavailable/unverified for
  fields it cannot prove, without backfilling observed values from planned
  settings
