## ADDED Requirements

### Requirement: Batch Preflight candidate

The system MUST persist a stable candidate containing the batch configuration, selected asset identities, resolved per-asset Queue plan, review state, and revision without creating QueueJobs before acceptance.

#### Scenario: Resolve a configured batch

- **WHEN** an operator starts Batch Preflight
- **THEN** the backend resolves effective per-asset decisions through the canonical Queue resolvers and returns a persisted candidate revision
- **AND** no QueueJob or ExecutionPlan is created for that candidate

#### Scenario: Edit a candidate

- **WHEN** an operator edits candidate configuration or selections
- **THEN** the previous resolved revision becomes unacceptably stale
- **AND** the candidate retains the original selection and configuration until re-resolution

### Requirement: Exact reviewed plan acceptance

The system MUST atomically create QueueJobs and existing ExecutionPlans from the exact reviewed candidate snapshots. Accepted material plan fields MUST NOT be recomputed from mutable settings, profiles, Asset Defaults, or resolver state during Queue processing.

#### Scenario: Accept the current candidate revision

- **WHEN** an operator accepts ready assets from the current candidate revision
- **THEN** the accepted Queue snapshots match the reviewed per-asset plans
- **AND** a stale revision, blocker, or reservation conflict prevents partial Queue insertion

#### Scenario: Accept a safe subset

- **WHEN** some candidate assets are ready and others require review
- **THEN** the operator can queue an explicit subset of ready assets
- **AND** the remaining configured assets and review reasons stay in the candidate

### Requirement: Preflight validation context

The system MUST pass exact candidate identity, revision, asset identity, and resolved configuration to supported Preview, LAB, and Test Encode validation actions. LAB edits MUST remain isolated until an explicit apply action triggers re-resolution.

#### Scenario: Validate a candidate asset

- **WHEN** an operator opens a supported validation action from a candidate plan
- **THEN** the existing canonical validation flow receives that candidate asset's resolved configuration
- **AND** validation does not create or mutate a QueueJob

### Requirement: Queue compatibility and bounded review

Every batch or multi-asset Queue creation path MUST require Preflight acceptance. Existing QueueJobs and snapshots MUST remain readable without rewrite, and candidate review MUST reuse persisted evidence without full-library scans or unnecessary repeated ffprobe work.

#### Scenario: Use an existing Queue snapshot

- **WHEN** a historical QueueJob without a complete preflight snapshot is claimed
- **THEN** it continues through the existing claim-time compatibility path
- **AND** new accepted preflight snapshots bypass material profile resolution during processing
