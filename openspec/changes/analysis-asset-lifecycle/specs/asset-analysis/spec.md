## Purpose

Defines how canonical source-media evidence is exposed, reused, frozen, and
correlated through an asset's lifecycle without creating a parallel analysis
system.

## ADDED Requirements

### Requirement: Asset Info exposes canonical source evidence lazily

The system SHALL expose the current asset's persisted canonical source evidence
through the Analysis section of Asset Info. The section MUST reuse existing
`ScanResult`, scanner operations, and evidence authorities, and MUST NOT add a
parallel analysis model, pipeline, cache, or Asset-Info-only analyzer.

#### Scenario: Asset Info Overview performs no media analysis

- **WHEN** a user opens Asset Info or renders its Overview
- **THEN** the system reads persisted inventory/lifecycle data only and starts
  no ffprobe, FFmpeg, OCR, snapshot refresh, Advisor, LAB, Validation, or other
  heavy/media operation

#### Scenario: Analysis section reads a persisted snapshot

- **WHEN** a user selects the Analysis section for an asset with a persisted
  snapshot
- **THEN** the system lazily loads and presents the canonical snapshot and its
  existing AS-IS, cadence/GOP, HDR/color, audio, subtitle, and other available
  components without starting analysis

#### Scenario: Missing evidence requires explicit action

- **WHEN** the current asset has no canonical snapshot
- **THEN** the section reports `missing` and starts a scan only after an
  explicit Analyze action

#### Scenario: Stale evidence requires explicit refresh

- **WHEN** a persisted snapshot has stale source identity or stale evidence
  components
- **THEN** the section reports `stale`, identifies stale components when
  available, and refreshes only after an explicit user action

#### Scenario: Unavailable evidence stays distinct

- **WHEN** the source or evidence cannot be read or reliably classified
- **THEN** the system reports `unavailable` with a bounded reason when
  available and does not present it as missing, current, or successful

### Requirement: Evidence identity and component outcomes are preserved

Reusable evidence SHALL remain bound to the source identity and scanner policy
and component provenance that produced it. Path equality alone MUST NOT make
evidence current. Legacy, failed, partial, inconclusive, and unavailable
components MUST retain their existing meanings and provenance.

#### Scenario: Source path is reused for different media

- **WHEN** a path points to a different source revision than the one associated
  with the persisted snapshot
- **THEN** the system reports old evidence as stale or unavailable and does not
  relabel it with the new source identity

#### Scenario: Incremental refresh updates only stale evidence

- **WHEN** existing scanner policy permits reuse and only some components are
  stale
- **THEN** the existing scanner refreshes those components and retains the
  identity/provenance and outcomes of reused components

### Requirement: Batch Preflight consumes the shared evidence projection

Batch Preflight SHALL batch-resolve the same canonical evidence state and
identity consumed by Asset Info. It MUST remain read-only and MUST NOT introduce
a parallel eligibility resolver or one evidence request per selected asset.

#### Scenario: Preview mixed evidence states

- **WHEN** a batch contains assets with ready, stale, missing, or unavailable
  evidence
- **THEN** Preflight reports each asset's state and bounded reason alongside
  existing backend-authoritative eligibility/configuration outcomes

#### Scenario: Preview never starts heavy analysis

- **WHEN** a user opens or refreshes Batch Preflight
- **THEN** the system reads persisted evidence only and starts no scan, ffprobe,
  FFmpeg, OCR, Advisor, or LAB work

#### Scenario: Preflight and Asset Info share identity

- **WHEN** the same asset is inspected by Asset Info and Batch Preflight
- **THEN** both report the same persisted evidence identity and status for the
  same observation and neither invents or upgrades evidence state

### Requirement: Queue freezes evidence provenance with its accepted plan

Queue SHALL remain authoritative for claim, preparation, effective
configuration, and immutable freeze. When canonical source evidence is used to
resolve or explain an accepted plan, Queue MUST freeze a stable source/evidence
identity and the bounded evidence needed to explain that plan. Post-freeze
changes MUST NOT rewrite either the plan or its evidence provenance.

#### Scenario: Queue prepares an evidence-dependent plan

- **WHEN** Queue preparation consumes persisted or newly refreshed source
  evidence to resolve the effective plan
- **THEN** the same freeze records the source identity, consumed evidence
  component/version identity, and the evidence payload or stable canonical
  reference needed to explain the accepted plan

#### Scenario: Evidence changes after freeze

- **WHEN** current source evidence changes after an execution plan is frozen
- **THEN** the queued/running execution and its historical evidence link remain
  bound to the frozen identity

#### Scenario: Plan does not consume optional evidence

- **WHEN** an evidence component was not used to resolve or explain the plan
- **THEN** Queue does not copy unrelated evidence into the frozen plan solely
  to make the snapshot appear complete

### Requirement: History correlates existing authorities

Per-asset History SHALL link source evidence to the frozen Queue plan and
execution result and SHALL expose the future Validation correlation point
without defining #53's behavior or storing duplicate reports.

#### Scenario: Render source evidence through execution

- **WHEN** canonical source evidence, a frozen plan, and a Queue result are
  available for an asset
- **THEN** History links each existing record by its authority and stable
  identity while preserving the source identity recorded at freeze

#### Scenario: Future Validation is absent

- **WHEN** no canonical #53 Validation record exists
- **THEN** History presents Validation as pending or unavailable and does not
  infer pass, warning, or failure

#### Scenario: Historical evidence cannot be rebound

- **WHEN** an asset is rescanned, renamed, moved, or replaced after Queue
  execution
- **THEN** historical plans and results keep their original source evidence
  identity and are not silently rebound to the current snapshot

### Requirement: LAB and Advisor remain separate capabilities

The Analysis lifecycle SHALL preserve Profile LAB and Advisor as distinct
existing capabilities with their own explicit actions and evidence/operation
lifecycles.

#### Scenario: Viewing Analysis does not run Advisor or LAB

- **WHEN** a user opens Asset Info Overview, Analysis, or Batch Preflight
- **THEN** no Advisor evaluation or LAB experiment runs implicitly

#### Scenario: Explicit Advisor and LAB actions remain available

- **WHEN** a user explicitly selects the existing Advisor or Profile LAB
  capability
- **THEN** each uses its established resolver and lifecycle independently of
  the passive Analysis evidence projection

### Requirement: Standalone Analysis routes migrate safely

The system SHALL expose source Analysis within Asset Info and SHALL NOT present
Analysis as a top-level workflow. Legacy Analysis and Scanner URLs MUST retain
supported asset context without triggering media work or binding evidence by
path equality alone.

#### Scenario: Analysis navigation is removed

- **WHEN** a user navigates the primary application menu
- **THEN** Analysis is not shown as a top-level section and the capability
  remains available from Asset Info

#### Scenario: Legacy route has no selected asset

- **WHEN** a user opens `/analysis` or `/scanner` without a selected asset
- **THEN** the system redirects to RAW Assets without starting an analysis

#### Scenario: Legacy route identifies an asset

- **WHEN** a legacy Analysis or Scanner URL includes a supported asset path
- **THEN** the system preserves the encoded context, opens Asset Info Overview
  first, and starts no scan automatically

#### Scenario: Legacy asset cannot be resolved

- **WHEN** the linked asset is missing from current inventory or its identity
  is ambiguous
- **THEN** the system shows a recoverable unresolved state and does not open an
  arbitrary filesystem path or attach evidence by path equality alone

#### Scenario: Legacy user decision is matched safely

- **WHEN** a saved standalone Analysis decision can be deterministically matched
  to its original source identity
- **THEN** History presents it as a legacy annotation and preserves its
  decision, notes, and recorded time without treating it as current evidence

#### Scenario: Legacy user decision cannot be matched

- **WHEN** a saved decision's source identity cannot be established
- **THEN** the record remains preserved and visible as unlinked legacy history
  and is not deleted, dropped, or attached to a path-reused asset
