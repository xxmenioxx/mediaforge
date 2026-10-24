## ADDED Requirements

### Requirement: Translation provider responses are constrained and diagnosed by batch

The Ollama translation adapter SHALL provide a structured-output schema derived
from the current bounded batch. The schema SHALL require the canonical
translation contract version, exactly the current batch cue count, cue IDs from
the current batch, nonempty text values, and no unrecognized object properties.
The canonical decoder and validators SHALL remain authoritative and SHALL reject
wrong contract versions, missing, extra, duplicate, reordered, or empty cues,
invalid protected ASS structure, incomplete responses, and malformed envelopes.

A rejected batch SHALL produce no translated execution output. Operation
responses MAY include an allowlisted diagnostic category, one-based batch
index/count, expected/actual cue counts when known, and an HTTP status when
applicable. Diagnostics SHALL NOT include source cue IDs or text, prompts, raw
provider responses, source paths, secrets, or credentials. Translation SHALL
preserve its existing source revision fence, cancellation behavior, retry
identity, staging cleanup, and atomic publication barrier.

#### Scenario: Ollama receives the current batch schema

- **WHEN** the Ollama translation adapter sends a bounded batch
- **THEN** the request schema is bound to that batch's canonical contract
  version, cue count, and allowed cue IDs
- **AND** the canonical validator still checks exact cue order and identity

#### Scenario: Malformed provider output has a safe batch diagnostic

- **WHEN** an Ollama response is empty, malformed, truncated, or violates the
  response contract
- **THEN** the operation reports a stable diagnostic category and one-based
  batch context
- **AND** it does not expose cue IDs, subtitle text, paths, secrets, or the raw
  provider body
- **AND** it produces no partial translated output

#### Scenario: Protected ASS structure remains under MVForge control

- **WHEN** a provider response changes protected ASS dialogue structure
- **THEN** the batch fails validation before output serialization/publication
- **AND** timestamps, styles, override tags, cue order, and source bytes remain
  unchanged
