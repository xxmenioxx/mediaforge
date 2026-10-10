## Why

Translation can stop safely when canonical OCR evidence needs human review, but the current **Review OCR** action only starts analysis and closes the Translation dialog. Users do not get a visible, reliably matched review surface, and an unchanged legitimate cue can trigger the same blocker again on retry.

## What Changes

- Open a visible OCR review surface from the blocked Translation dialog, with explicit loading, no-findings, failure, and cancel/return states that preserve the selected Translation source.
- Resolve the review target against the exact canonical media/stream revision, OCR request identity, published sidecar revision, and logical SRT/ASS transform; surface missing or conflicting evidence without guessing.
- Provide cue-level Correct, Accept as correct, and Leave pending decisions, with bounded batch preview and immutable timing/layout fields.
- Persist approval provenance against the exact cue-text hash and source revision so Translation can retry past that reviewed cue without weakening stale-source checks or calling its provider before OCR review is complete.
- Reuse canonical OCR evidence, the existing #262 contextual dictionary preparation, correction operations, serializers, and atomic publication; add end-to-end frontend and backend regressions.

Out of scope: rebuilding or changing the OCR dictionary, OCR engine behavior, a new review/correction pipeline, and the separate #267 AI correction work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Translation-triggered OCR review becomes visible and exact; human cue decisions are revision-bound and honored by canonical Translation preparation.

## Impact

Affected areas are the existing subtitle eligibility and translation/OCR review contracts in `backend/internal/handlers`, their API types and operation responses, the Asset Info Translation and sidecar review UI, and focused frontend/backend regressions. The existing canonical subtitle operation, cue serializers, provenance snapshots, and atomic SRT/ASS publication remain the authorities.
