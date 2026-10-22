## ADDED Requirements

### Requirement: Asset Info proposes subtitle synchronization against an explicit audio stream

Asset Info SHALL offer synchronization for eligible SRT and ASS subtitle artifacts and supported SSA artifacts. A synchronization request SHALL identify the exact subtitle source and one explicit resolved audio stream from the same asset. The backend SHALL return a canonical proposal that identifies its source revision, reference audio identity, correction model, engine, quality state, and bounded diagnostics. Analysis SHALL NOT mutate or publish the source subtitle.

#### Scenario: User requests a fixed-offset proposal
- **WHEN** Asset Info requests a fixed-offset synchronization proposal for an eligible subtitle and explicitly selected audio stream
- **THEN** the backend returns the exact subtitle and audio identities, requested offset, fixed-offset model, and a bounded preview of current and proposed cue times without changing the subtitle

#### Scenario: No reference audio is selected
- **WHEN** a synchronization request omits or identifies a nonexistent audio stream
- **THEN** the backend rejects the request and returns no proposal

#### Scenario: Optional automatic engine is unavailable
- **WHEN** the user requests automatic synchronization and no compatible engine is available
- **THEN** the backend reports the unavailable capability and leaves the manual fixed-offset mode available without fabricating confidence or a proposal

### Requirement: Synchronization changes timing while preserving subtitle content

Applying a pure synchronization transform SHALL change only canonical cue timing fields. SRT dialogue and cue structure SHALL remain unchanged. ASS/SSA styles, Script Info, Events structure, dialogue text, override tags, positioning, karaoke structure, drawings, and attachments SHALL remain unchanged. The operation SHALL reject malformed or unsafe timing output before publication.

#### Scenario: SRT fixed offset preserves dialogue
- **WHEN** a fixed offset is applied to a valid SRT subtitle
- **THEN** only cue start and end timestamps change and all dialogue bytes and cue ordering remain intact

#### Scenario: ASS fixed offset preserves document structure
- **WHEN** a fixed offset is applied to a valid ASS or supported SSA subtitle
- **THEN** only Start and End values for playback dialogue events change while all other source bytes and structure remain intact

#### Scenario: Transform would create invalid timing
- **WHEN** a transform produces a negative, reversed, overflowing, or otherwise invalid cue interval
- **THEN** validation fails and the original subtitle remains unchanged

### Requirement: Synchronization review precedes apply

Asset Info SHALL show the proposed correction model, selected audio identity, quality state, and representative current/proposed cue times before enabling Apply. A proposal SHALL be bound to the exact source revision and audio stream identity used to create it. Apply SHALL fail closed if either identity is stale.

#### Scenario: User reviews a proposal before applying
- **WHEN** a valid proposal is available
- **THEN** Asset Info displays a bounded before/after timing preview and requires an explicit Apply action before publication

#### Scenario: Source changes after analysis
- **WHEN** the subtitle changes after a proposal is created and Apply is requested
- **THEN** the backend reports a stale proposal and publishes nothing

#### Scenario: Reference stream changes after analysis
- **WHEN** the selected audio stream identity no longer matches the proposal at Apply time
- **THEN** the backend rejects the stale proposal and leaves the subtitle unchanged

### Requirement: Synchronized subtitle publication is validated, atomic, and cancellable

Apply SHALL use the canonical subtitle parsing and serialization path, operation cancellation semantics, private staging, output validation, source revision recheck, and atomic publication barrier. The existing subtitle SHALL remain intact until the transformed artifact has passed validation and publication can commit. Cancellation, validation failure, or publication failure SHALL leave the original and any previously valid artifact intact and SHALL clean only operation-owned staging.

#### Scenario: Valid synchronized subtitle is published
- **WHEN** a reviewed proposal is applied to its unchanged source and output validation succeeds
- **THEN** the synchronized artifact replaces the selected source atomically and the operation records the applied result

#### Scenario: Apply is cancelled before publication
- **WHEN** an apply operation is cancelled before its publication barrier
- **THEN** the operation becomes cancelled after cleanup, publishes no synchronized artifact, and preserves the current subtitle

#### Scenario: Invalid output fails before replacement
- **WHEN** the staged synchronized subtitle fails parsing, cue-count, ordering, content-integrity, or format-integrity validation
- **THEN** publication does not occur and the existing subtitle remains intact

#### Scenario: Successful apply records bounded provenance
- **WHEN** a synchronization operation completes
- **THEN** provenance identifies the source revision, selected audio stream, engine and version, model, offset/scale/segments, quality state, analysis and apply times, and operation identity without storing unbounded raw engine logs
