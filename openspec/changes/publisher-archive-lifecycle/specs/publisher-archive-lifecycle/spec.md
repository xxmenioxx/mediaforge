## Purpose

This capability makes post-publication original archival visible and traceable while preserving Publisher, Queue, and Asset lifecycle authorities.

## ADDED Requirements

### Requirement: Publisher and archive stages are evidence-backed

Queue lifecycle history SHALL record `publishing` when a fenced Publisher claim is acquired, `archiving_original` when the original archive claim is acquired, and `completed` only after the archive outcome is terminal and persisted. A Queue job's overall `status` SHALL retain its existing meaning and SHALL NOT be changed to represent archive progress. The interface SHALL show timestamps only from persisted stage history. Unvisited historical stages SHALL NOT be presented as executed or skipped without persisted evidence.

#### Scenario: Publisher claim and archive claim advance the timeline

- **WHEN** publication and original archival claims are acquired in sequence
- **THEN** the Queue stage history records `publishing` and `archiving_original` at their actual transition times, with no premature `completed` event

#### Scenario: Archive outcome reaches a terminal state

- **WHEN** the archive outcome is persisted as `complete`, `failed`, or `not_applicable`
- **THEN** the Queue stage history records `completed` after the archive transition while the Queue job's overall status remains unchanged

#### Scenario: A historical job has no persisted Publisher transitions

- **WHEN** a completed historical job lacks `publishing`, `archiving_original`, or `completed` stage-history entries
- **THEN** the interface marks those stages as not recorded and does not synthesize timestamps or skipped events from terminal status or current archive state

### Requirement: Original archive state remains separate and visible

Queue and Asset Info SHALL present the archive state separately from the Queue job status and publication state. Supported archive states SHALL include `pending`, `archiving`, `complete`, `failed`, and `not_applicable`. A failed archive SHALL expose its persisted failure summary. Missing legacy archive status SHALL remain unknown or not recorded rather than being inferred.

#### Scenario: Published job is waiting for archival

- **WHEN** a Queue job is completed and its publication is `PUBLISHED` while the original archive is `pending`
- **THEN** Queue and Asset Info show the pending archive state without describing the original as archived

#### Scenario: Archive fails after publication

- **WHEN** publication remains `PUBLISHED` and the archive state is `failed`
- **THEN** Queue and Asset Info distinguish the failed archive from successful publication and show the available failure summary

#### Scenario: Archive is not applicable

- **WHEN** the archive operation reaches `not_applicable`
- **THEN** Queue and Asset Info show that terminal state without displaying an archived-original record

### Requirement: Active Publisher and archive work refreshes relevant views

Queue-job and Asset Info data SHALL refresh at a bounded interval while Queue, Publisher, or original archive work is nonterminal. Assets data SHALL refresh alongside the Queue-job projection so selected Asset Info can update from the canonical lifecycle response. Polling SHALL stop after the archive is `complete`, `failed`, or `not_applicable` and SHALL NOT continue for unrelated completed jobs.

#### Scenario: Slow original archival refreshes Assets and Queue

- **WHEN** a completed Queue job has a `PUBLISHED` publication and archive state `pending` or `archiving`
- **THEN** Queue and Assets continue targeted refreshes until the archive state becomes terminal

#### Scenario: Terminal archive stops refresh

- **WHEN** the archive state changes to `complete`, `failed`, or `not_applicable`
- **THEN** targeted polling stops after the terminal state is observed

### Requirement: Archived originals appear only after canonical persistence

Asset Info SHALL show an archived original only when the existing lifecycle projection resolves the owning Queue job, current publication, source provenance, and archived `AssetRecord`. The owning Queue job's `OriginalArchivedPath` and the archived `AssetRecord` SHALL be persisted together only after the actual archive move commits. Filename or path resemblance alone SHALL NOT create an association.

#### Scenario: Archive commit makes the original visible automatically

- **WHEN** the archive move succeeds and the canonical archive outcome transaction persists the Queue job and archived `AssetRecord`
- **THEN** the refreshed Asset Info shows that linked original without manual reload

#### Scenario: Archive is still in progress

- **WHEN** the archive state is `pending` or `archiving`
- **THEN** Asset Info shows the independent state and does not expose the pending archive destination as an archived original

#### Scenario: Identity is unresolved

- **WHEN** archive and Library paths resemble one another but canonical source or publication identity does not resolve
- **THEN** the archive remains unresolved and is not associated by name or path resemblance

### Requirement: Rename is blocked while the matching archive is active

Rename of an asset whose exact Queue/source/publication identity is undergoing original archival SHALL fail before filesystem or provenance mutation while archive status is `pending` or `archiving`. Existing identity checks and mutation guards SHALL be reused. Rename SHALL return to its existing policy after the archive reaches a terminal state.

#### Scenario: Rename targets a source during pending or active archival

- **WHEN** Rename targets the exact source or replacement asset reserved by a published Queue job with archive state `pending` or `archiving`
- **THEN** the backend rejects Rename without moving the file or changing Queue, publication, or AssetRecord identity

#### Scenario: Rename follows a terminal archive outcome

- **WHEN** the matching archive is terminal
- **THEN** Rename is evaluated by the existing location, identity, and active-work rules
