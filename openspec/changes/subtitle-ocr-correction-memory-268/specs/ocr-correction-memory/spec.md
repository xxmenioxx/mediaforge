## Purpose

Provide a user-controlled local memory of approved OCR cue corrections that can offer bounded, language- and scope-matched suggestions while preserving the existing OCR approval and publication boundaries.

## ADDED Requirements

### Requirement: OCR correction memory is explicit, local, and bounded

OCR correction memory learning SHALL be off by default and independent from OCR AI enablement. Disabling learning SHALL stop new captures and retrieval without deleting existing entries. The user SHALL be able to configure a bounded retention period. Memory SHALL store only bounded cue text, a context fingerprint, and correction provenance; it SHALL NOT retain PGS/VobSub images, entire media, full cue transcripts, or unbounded prompts. The memory SHALL remain separate from Translation Memory and SHALL NOT fine-tune model weights.

#### Scenario: Learning is off on a fresh installation

- **WHEN** OCR correction memory settings are first created
- **THEN** learning and OCR AI prompt enrichment are disabled and no correction is captured or retrieved until explicitly enabled

#### Scenario: Disabling learning preserves entries

- **WHEN** the user disables OCR correction memory
- **THEN** existing entries remain inspectable and exportable, while capture and suggestion retrieval stop

#### Scenario: Retention expires entries without affecting subtitles

- **WHEN** an entry exceeds the configured retention period
- **THEN** it is excluded from suggestions and removed from memory during bounded maintenance without changing any published subtitle artifact

#### Scenario: Memory retains bounded cue evidence only

- **WHEN** a correction is captured
- **THEN** only the corrected cue text, bounded context fingerprint, scoped source/artifact provenance, and bounded approval evidence are stored; no bitmap image or full transcript is retained

### Requirement: Only committed user-approved corrections become learned examples

The system SHALL capture a manual cue edit or an explicitly applied contextual dictionary suggestion only after the existing OCR correction operation commits its publication or approval revision successfully. Unapproved OCR AI output, pending/rejected decisions, and automatically applied dictionary matches SHALL NOT become learned examples or global rules. Captured entries SHALL record source language, scope, source and artifact revisions, approval action and time, and available dictionary rule version or model identifier. Duplicate approved examples SHALL merge bounded occurrence/evidence provenance.

#### Scenario: Manual cue edit is captured after commit

- **WHEN** learning is enabled and a manual text correction succeeds through the existing OCR correction operation
- **THEN** one language- and scope-bound memory example is stored with the committed source/artifact revision and approval provenance

#### Scenario: User-applied dictionary suggestion is captured as evidence

- **WHEN** learning is enabled and the user explicitly applies a contextual dictionary suggestion through OCR review
- **THEN** the accepted cue correction is captured with its dictionary rule provenance and is not promoted into a dictionary rule

#### Scenario: Unapproved model output is never learned

- **WHEN** OCR AI returns a suggestion that the user has not explicitly accepted through OCR correction
- **THEN** no memory entry or automatic rule is created from that output

#### Scenario: Failed, stale, or cancelled correction is not learned

- **WHEN** correction validation, publication, or the final revision check fails or is cancelled
- **THEN** the operation creates no learned example

#### Scenario: Duplicate captures merge evidence

- **WHEN** the same language-, text-, context-, and scope-bound correction is approved again
- **THEN** the memory keeps one deduplicated example and adds bounded occurrence/provenance evidence

### Requirement: Memory suggestions are deterministic, language-safe, and advisory

Memory retrieval SHALL require the exact source language and a matching resolved scope. Exact text/context matches SHALL rank ahead of less-specific or similar candidates. Similarity and context heuristics SHALL produce suggestions only; they SHALL never modify canonical cues or bypass the existing approval action. Retrieval SHALL use no loaded OCR or translation model. Memory examples MAY be added to an optional OCR AI prompt only when the user separately enables that use, and only in bounded same-language examples.

#### Scenario: Exact scoped match ranks first

- **WHEN** multiple enabled, unexpired entries match a cue in the same language
- **THEN** exact text and context matches from the most specific matching scope rank first and include their source provenance

#### Scenario: Similar cue is suggestion only

- **WHEN** a same-language cue is similar to a learned example but does not exactly match it
- **THEN** the example may be shown as a proposal and the published or canonical cue remains unchanged until a user confirms a correction

#### Scenario: Language mismatch returns no suggestion

- **WHEN** the source language differs from a memory example language
- **THEN** that example is excluded even if its text or context matches

#### Scenario: Lookups work without a loaded model

- **WHEN** the OCR AI model is unavailable or disabled
- **THEN** deterministic memory search and review suggestions continue to work without loading a model

#### Scenario: Prompt enrichment requires its own opt-in

- **WHEN** OCR AI is enabled but memory prompt enrichment is disabled
- **THEN** no memory text is sent to the OCR AI provider

### Requirement: Users can inspect, edit, export, revoke, and delete examples

The user SHALL be able to search and inspect learned examples and their provenance, export the bounded memory, edit text and scope, disable or revoke an example, delete it, and record accepted/rejected suggestion feedback. Editing text or source identity SHALL invalidate approval and disable the entry until a newly approved correction is captured. A revoked or invalidated entry SHALL not be retrieved. Existing entries SHALL not be silently propagated across languages or broader scopes.

#### Scenario: Editing learned text invalidates its approval

- **WHEN** the user changes an entry's original or corrected text
- **THEN** the entry is marked stale and disabled from retrieval until a new approved correction is captured

#### Scenario: User revokes a learned example

- **WHEN** the user disables or deletes an example
- **THEN** future retrieval excludes it and no published subtitle is changed

#### Scenario: Feedback counts are recorded

- **WHEN** the user accepts or rejects a displayed memory suggestion
- **THEN** the corresponding accepted or rejected counter is incremented without changing the entry's source provenance

#### Scenario: Export is bounded and secret-safe

- **WHEN** the user exports memory entries
- **THEN** the export contains only the bounded stored text and correction provenance and excludes credentials, full prompts, media bytes, and unrelated settings

#### Scenario: Existing database migrates without synthetic examples

- **WHEN** an existing SQLite database starts with OCR correction memory enabled by schema migration
- **THEN** its new memory tables and default-off setting are created without changing existing OCR approvals, dictionaries, Translation Memory, or subtitle artifacts
