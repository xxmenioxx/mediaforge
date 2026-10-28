## Why

Approved OCR cue edits are currently tied to their published artifact and do not help review later cues. Users need a small local memory of decisions that can propose language- and scope-matched corrections without changing model weights or retaining whole subtitles or bitmap inputs.

## What Changes

- Add a separate, default-off setting for learning approved OCR corrections, plus configurable retention and an independent opt-in for sending bounded examples to optional OCR AI prompts.
- Capture only committed manual cue edits and user-applied contextual dictionary suggestions, with language, cue-context fingerprint, source/artifact revision, scope, approval action, and available dictionary/model provenance.
- Provide deterministic, bounded memory suggestions, management/search/export, feedback counts, disable/revoke, edit, and delete controls. Suggestions never change cues without a fresh user decision.
- Keep this data model and lifecycle separate from Translation Memory.

## Capabilities

### New Capabilities

- `ocr-correction-memory`: Local, approved OCR correction examples and their safe retrieval, management, and retention behavior.

### Modified Capabilities

None. Existing OCR analysis, approval, correction, and publication requirements remain authoritative and are reused by the new capability.

## Impact

- Backend: SQLite models/migration, OCR correction commit integration, bounded retrieval and settings/API endpoints.
- Frontend: subtitles settings controls and a searchable correction-memory manager with review feedback actions.
- No new dependencies, OCR engine behavior, Translation Memory changes, model fine-tuning, or media-file writes.
