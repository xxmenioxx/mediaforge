## Purpose

Queue Batch Preflight lets operators understand the exact subtitle actions in a candidate and make deliberate, one-time per-stream changes before accepting Queue work.

## ADDED Requirements

### Requirement: Preflight presents authoritative subtitle decisions

For each candidate asset, Preflight SHALL present a versioned projection of the effective canonical subtitle plan frozen in that candidate revision. The projection SHALL describe source streams and their individual effective Keep, Extract, Keep + Extract, or Remove action, available language, stream index, codec, title, forced/default state, semantic role, action provenance, and planned sidecar representations and OCR. A compact source-stream summary SHALL distinguish embedded tracks from sidecars and removed tracks without counting multiple representations as multiple source streams. Missing plan evidence and assets without eligible subtitle streams SHALL be stated explicitly.

#### Scenario: Default summary and detailed decisions
- **WHEN** a resolved candidate asset has subtitle source streams
- **THEN** Preflight shows counts for embedded, sidecar, and removed source streams and can expand to show each exact frozen decision and available metadata
- **AND** requested sidecars are described as planned outputs, not as already generated artifacts

#### Scenario: No subtitle streams or unavailable plan
- **WHEN** a candidate has no source subtitle streams or its authoritative subtitle projection is unavailable
- **THEN** Preflight distinguishes `No subtitle tracks selected` from unavailable plan evidence and does not infer decisions from mutable Settings

#### Scenario: Multiple sidecar representations
- **WHEN** one subtitle stream requests both SRT and ASS representations
- **THEN** the summary counts one source stream and the detailed decision lists both planned representations, with OCR shown when required

### Requirement: Preflight explains effective policy without treating preferences as a filter

Preflight SHALL reflect effective actions from the canonical plan, including fallback actions and per-language action rules. Preferred subtitle rules SHALL be explained as action rules rather than a language allowlist, and the default subtitle language SHALL be explained as a default-flag preference rather than a stream filter. When unmatched non-preferred streams inherit an extracting fallback, Preflight SHALL clearly identify that those streams will also be extracted. A Remove decision SHALL authorize no subtitle preparation, OCR, or sidecar output for that stream.

#### Scenario: Extract fallback includes unmatched languages
- **WHEN** the effective subtitle fallback is Extract and preferred rules select SPA Extract, ENG Extract, and JPN Keep + Extract
- **THEN** unmatched source languages are shown as Extract, the preferred actions remain as resolved by the canonical plan, and the UI explains why unmatched streams are included

#### Scenario: Remove fallback excludes unmatched languages
- **WHEN** the effective subtitle fallback is Remove and preferred rules select actions for SPA, ENG, and JPN
- **THEN** unmatched streams remain Remove unless another higher-priority policy selects an action, with no subtitle work or sidecar planned for removed streams

#### Scenario: Preferred and default-language help
- **WHEN** Preflight displays subtitle decisions
- **THEN** its help text states that preferred-language rules select actions and the default subtitle language affects default-flag behavior, not stream selection

### Requirement: Candidate-only subtitle disposition overrides

Preflight SHALL allow an operator to explicitly override an individual eligible subtitle stream for the current candidate with Inherit, Keep, Extract, Keep + Extract, or Remove. Candidate overrides SHALL be shown as candidate-only, identify before-and-after actions, and be resolved through the canonical subtitle planner. Inherit SHALL restore the canonical decision. Overrides SHALL be bound to exact source stream identity and the candidate revision; ambiguous, stale, missing, or mismatched identity SHALL require re-resolution rather than mapping by language. Overrides SHALL NOT change saved Asset Tracks, Asset Defaults, or Track Profiles.

#### Scenario: Override one additional stream
- **WHEN** an operator changes one identified stream from its inherited action to Keep, Extract, or Keep + Extract for this candidate
- **THEN** Preflight shows the candidate-only override and before-and-after decision, and the resolved Queue plan reflects only that stream's authorized action

#### Scenario: Inherit or discard local edits
- **WHEN** an operator reverts a stream to Inherit or discards unsubmitted custom edits
- **THEN** the candidate returns to its inherited plan and no persistent Asset Tracks or settings override is written

#### Scenario: Stream identity becomes stale
- **WHEN** the source revision or selected stream identity changes while candidate overrides exist
- **THEN** Preflight invalidates or rejects those overrides and requires a fresh resolution without matching by language alone

### Requirement: Accepted Queue work retains reviewed subtitle decisions

Accepting a candidate SHALL freeze its reviewed per-stream subtitle decisions, sidecar formats, OCR requirements, and candidate-only overrides into the accepted Queue snapshot consumed by Worker and downstream artifact, Validation, and Publisher paths. Later Settings or Asset changes SHALL NOT alter the accepted snapshot. Existing explicit Asset Tracks selections and canonical role/forced-track rules SHALL retain their precedence and safety behavior.

#### Scenario: Accepted plan survives later configuration changes
- **WHEN** a candidate with reviewed subtitle decisions is accepted and subtitle settings later change
- **THEN** the accepted Queue work continues with the exact reviewed per-stream actions and planned sidecar/OCR decisions

#### Scenario: Existing explicit Asset Tracks selection remains authoritative
- **WHEN** an Asset has an explicit saved subtitle stream subset
- **THEN** candidate review preserves its explicit-versus-inherited state and candidate edits do not silently broaden the saved selection

#### Scenario: Historical Queue snapshots remain compatible
- **WHEN** an existing Queue snapshot predates the subtitle decision projection
- **THEN** it remains readable and its frozen subtitle behavior is not re-resolved or rewritten
