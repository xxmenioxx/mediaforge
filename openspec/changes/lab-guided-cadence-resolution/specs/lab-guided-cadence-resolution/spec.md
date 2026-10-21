## Purpose

Help operators understand and resolve LAB cadence/GOP planning blocks with bounded, evidence-backed alternatives while keeping output Fidelity and Queue execution governed by canonical backend evidence.

## ADDED Requirements

### Requirement: LAB explains unresolved automatic cadence and GOP plans
LAB SHALL return a versioned `resolution_required` diagnostic when automatic frame-structure planning is blocked by unresolved effective output FPS. The diagnostic SHALL identify each blocking field, its authority state and reason, source evidence identity/version, requested cadence and GOP, any known effective FPS/progressive state, and only currently eligible resolution actions. Existing HTTP status and legacy error fields SHALL remain compatible.

#### Scenario: Ambiguous hard telecine blocks automatic GOP
- **WHEN** ambiguous cadence leaves effective output FPS unknown and GOP mode is Auto
- **THEN** LAB reports a structured review diagnostic and eligible actions instead of only an opaque planning exception

#### Scenario: Existing cadence evidence resolves Auto GOP
- **WHEN** the canonical cadence resolver has validated the effective cadence and output FPS
- **THEN** LAB preserves the resolved motion/filter/FPS decision and resolves Auto GOP through the normal frame-structure authority

#### Scenario: Unsupported transform remains unresolved
- **WHEN** a selected transform cannot establish a supported output FPS or field structure
- **THEN** LAB leaves the plan in `NEEDS_REVIEW` or `UNAVAILABLE` and does not invent a fixed FPS or GOP workaround

### Requirement: Resolution actions are bounded and evidence-backed
The resolution assistant SHALL evaluate a small bounded candidate set through existing cadence, effective video, GOP, HEVC, preview, Test Encode, and Fidelity authorities. It SHALL not mutate source media, start Deep Cadence automatically, guess TFF/BFF from container metadata or one winning window, or create an unbounded FFmpeg retry loop.

#### Scenario: Deep Cadence is available to improve insufficient evidence
- **WHEN** current evidence is insufficient and the canonical bounded Deep Cadence operation is eligible
- **THEN** LAB offers an explicit re-analysis action that reuses the current source/snapshot evidence identity

#### Scenario: Preserve is only an alternative when independently supported
- **WHEN** source FPS and required field structure/geometry are independently supported and no contradictory active FPS transform exists
- **THEN** LAB may offer an asset-scoped preserve alternative with its telecine, judder, or combing trade-offs stated

#### Scenario: Field order is ambiguous
- **WHEN** canonical field-order authority cannot distinguish TFF from BFF
- **THEN** LAB requires operator-requested bounded comparison previews and does not select a winner automatically

### Requirement: Applying and testing an alternative preserves execution boundaries
An accepted alternative SHALL be applied only to an asset-scoped LAB draft/override, SHALL be revalidated against current source and profile evidence, and SHALL use the same frozen effective cadence, FPS, field order, GOP, encoder/HEVC decision, and fingerprint for LAB preview/Test Encode and Queue preflight. Applying an alternative SHALL NOT mutate global profile/default settings or an existing frozen Queue job and SHALL NOT enqueue or publish work.

#### Scenario: Operator applies and tests an asset alternative
- **WHEN** an operator applies an eligible recommendation and requests a test
- **THEN** LAB tests the resulting deterministic per-asset plan through compatible Preview/Test Encode without changing global settings or Queue jobs

#### Scenario: Evidence becomes stale before applying an alternative
- **WHEN** source identity, scan revision, or profile evidence changes after recommendation generation
- **THEN** LAB rejects or refreshes the stale recommendation before applying or testing it

### Requirement: Planning readiness and output Fidelity remain separate
LAB SHALL label planning status `RESOLVED`, `NEEDS_REVIEW`, or `UNAVAILABLE` independently of sample Fidelity. A sample SHALL be labelled Safe only when authoritative canonical field, metric, and coverage rules pass; `UNVERIFIED` SHALL remain distinct from mismatch and SHALL prevent Safe.

#### Scenario: Preserve preview has unresolved canonical fields
- **WHEN** the frozen Preserve plan cannot establish required source FPS or field structure
- **THEN** LAB shows field-level `UNVERIFIED` authority and reason and does not label the sample Safe

#### Scenario: Preview succeeds while Canonical Frame Fidelity is unverified
- **WHEN** Preview and planning resolve successfully but the canonical Fidelity aggregate has required `UNVERIFIED` fields
- **THEN** LAB shows those canonical field reasons and only existing eligible actions that can address those fields; it labels planning `RESOLVED`, Fidelity `UNVERIFIED`, and keeps the sample in review

#### Scenario: Bounded evidence cannot resolve field order
- **WHEN** bounded evidence remains inconclusive after analysis or comparison
- **THEN** LAB leaves Fidelity in review and exposes the manual comparison step without changing thresholds or bypassing canonical checks

### Requirement: Existing LAB errors and frozen jobs remain compatible
Non-resolution errors SHALL preserve existing operation safety and error fields. Existing frozen Queue executions SHALL remain immutable when LAB drafts, profiles, scans, or source media later change.

#### Scenario: Existing client reads legacy error fields
- **WHEN** a client receives a resolution diagnostic
- **THEN** it can continue reading the existing error/status fields while newer clients may consume the versioned diagnostic

#### Scenario: Queue execution was frozen before LAB edits
- **WHEN** an asset-scoped LAB alternative is edited after a Queue job has frozen its plan
- **THEN** the running Queue execution retains its captured cadence, FPS, field order, GOP, and encoder decisions
