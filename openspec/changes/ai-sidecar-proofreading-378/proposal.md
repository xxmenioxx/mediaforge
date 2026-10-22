## Why

Reviewing long imported, translated, and OCR-generated subtitle sidecars one cue at a time is slow. A single safe review entry point can surface likely text defects while keeping the sidecar unchanged until an operator approves specific edits.

## What Changes

- Add bounded Quick and opt-in Extended AI proofreading for eligible text SRT, ASS, and SSA sidecars in Asset Info.
- Reuse canonical sidecar ownership, revision, cue parsing, shared Ollama settings, subtitle operation cancellation, OCR quality evidence, and safe publication boundaries.
- Add a summary-first review experience with exact cue decisions and confirmed batch application, including restorable prior revisions.
- Keep OCR bitmap correction and source-to-translation fidelity review with their existing specialized owners.

## Capabilities

### New Capabilities
- `subtitle-proofreading`: bounded proofreading and explicit review of text subtitle sidecars.

### Modified Capabilities
- None. Proofreading is a separate observable workflow and does not change subtitle conversion, OCR correction, or translation requirements.

## Impact

- Backend: Asset Info subtitle handlers, canonical subtitle parsing/revision/publication utilities, shared Ollama adapter, and subtitle operation lifecycle.
- Frontend: Asset Info sidecar actions, API types/client, and a compact proofreading dialog.
- Tests: backend format/provider/revision cases and focused API/UI integration coverage.
- No new dependency, migration, provider endpoint, or global Translation Settings change.
