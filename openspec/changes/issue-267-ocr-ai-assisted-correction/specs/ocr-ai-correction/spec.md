## ADDED Requirements

### Requirement: OCR AI suggestions are optional and use an explicit OCR model

OCR AI assistance MUST be disabled when no OCR AI setting exists and MUST use only the explicitly selected model in `ocrAISettings`. It MUST NOT read, copy, or fall back to Translation's selected model. Disabled, absent, or invalid OCR AI settings MUST make no inference request and MUST leave deterministic OCR analysis and review available.

#### Scenario: OCR AI is disabled or has no explicit model

- **WHEN** canonical OCR review runs while OCR AI is disabled, absent, or invalid
- **THEN** the report identifies the unavailable or disabled assistance state
- **AND** no Ollama inference request is made
- **AND** deterministic OCR findings, review, and Translation remain available

#### Scenario: Translation uses a different model

- **WHEN** OCR AI is enabled with an explicitly selected model and Translation has another selected model
- **THEN** OCR assistance uses exactly the OCR model
- **AND** it does not inspect or fall back to the Translation model

### Requirement: OCR AI inference requires admission and bounded suspicious input

OCR AI MUST use the shared Ollama provider and shared resource admission lifecycle. It MUST make no inference request when admission or the explicitly selected model is unavailable. It MUST consider only suspicious cues remaining after deterministic dictionary and quality analysis, and MUST bound cue text, contextual text, batch size, and total cue count. Clean or already-resolved cues MUST NOT be sent.

#### Scenario: Resource admission is unavailable

- **WHEN** OCR AI is enabled but the shared lifecycle or inference admission gate is unavailable or denies the request
- **THEN** the report exposes a resource-unavailable state
- **AND** no inference request is made
- **AND** canonical OCR findings and review remain available

#### Scenario: Only bounded suspicious cues are submitted

- **WHEN** a canonical OCR report contains both clean and suspicious cues
- **THEN** only unresolved suspicious cues and bounded neighboring text are sent in small bounded batches
- **AND** clean, dictionary-resolved, or unrelated cues are excluded

### Requirement: OCR AI suggestions are strictly bound to canonical cue identity

Every suggestion MUST match the exact canonical OCR review target and captured source/artifact/baseline revisions, cue ID, original cue hash, expected model, count, and order. Responses MUST satisfy the existing strict proofreader contract and text bounds. The source language, meaning, timestamps, positions, ASS tags/styles, cue ordinal, and sidecar structure MUST remain controlled by MVForge. Suggestions MUST be discarded when the source or configuration changes, a response is malformed, or the request is cancelled; partial batches MUST NOT be exposed.

#### Scenario: Current cue identity returns a suggestion

- **WHEN** a valid response matches the selected OCR model, exact review target, cue ID, source hash, order, and captured revisions
- **THEN** a transient suggestion with its reason and model identity is attached to that cue
- **AND** no subtitle sidecar, approval, or publication is changed

#### Scenario: Source revision changes during inference

- **WHEN** the canonical source, selected OCR identity, artifact revision, or baseline changes while an AI batch is running
- **THEN** the operation reports a stale result and exposes no suggestion from that operation
- **AND** it does not alter canonical cues or sidecars

#### Scenario: Response is malformed, incomplete, or cancelled

- **WHEN** any AI batch fails strict response validation, times out, or is cancelled
- **THEN** the operation exposes no partial suggestions and reports the failure state
- **AND** deterministic findings and canonical OCR review remain available

### Requirement: OCR AI suggestions require human decisions

The OCR review surface MUST display Original, deterministic Dictionary suggestion, and AI Suggestion as distinct values. Accepting an AI suggestion MUST only stage a human correction for preview and explicit confirmation through the existing canonical review flow. Correct, Reject, and Leave Pending MUST remain available. No suggestion may automatically modify, approve, publish, translate, or otherwise persist subtitle content. A suggestion MUST be hidden or invalidated when its cue hash or baseline no longer matches.

#### Scenario: Human stages and confirms an AI suggestion

- **WHEN** a reviewer accepts an AI suggestion
- **THEN** the UI stages it as a correction and shows the existing decision preview
- **AND** no correction or publication request occurs until the reviewer explicitly confirms the preview

#### Scenario: Reviewer rejects or leaves a suggestion pending

- **WHEN** a reviewer rejects a suggestion or leaves it pending
- **THEN** the suggestion is dismissed or remains unresolved according to the selected action
- **AND** no AI text is saved as canonical OCR content

#### Scenario: A suggestion becomes stale in the UI

- **WHEN** the displayed cue hash or baseline no longer matches the suggestion
- **THEN** the UI does not offer that suggestion for acceptance

### Requirement: Translation continues to honor canonical human OCR approvals

OCR AI assistance MUST NOT change the Translation engine or approval contract. Retrying Translation after an existing canonical OCR approval MUST continue through the current Translation path, recognize the matching approval for the same source and cue hash, and avoid re-blocking on that approved finding.

#### Scenario: Retry uses an existing canonical OCR approval

- **WHEN** Translation is retried with the same source after a current canonical OCR approval exists
- **THEN** preparation recognizes the approval and the Translation provider may receive the approved canonical cue text after other required findings are resolved
- **AND** no OCR AI suggestion is auto-applied
