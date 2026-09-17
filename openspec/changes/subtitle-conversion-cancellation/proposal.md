## Why

Asset Info can start subtitle extraction and OCR work that lasts long enough for
users to need an immediate stop, especially for PGS and VobSub conversion. The
existing operation model already exposes progress and results, so cancellation
should make that operation lifecycle safe and controllable without creating a
second queue or job system.

## What Changes

- Add a Cancel action for running Asset Info subtitle conversion operations,
  covering text SRT/ASS conversion and bitmap PGS/VobSub OCR conversion.
- Extend the existing subtitle operation lifecycle with cancellable execution,
  context propagation, child-process termination, and a distinct `cancelled`
  terminal status.
- Keep conversion and OCR outputs in temporary, validated staging until the
  complete requested result is ready; cancelled or failed work must never
  publish partial sidecars.
- Clean temporary conversion/OCR workspace after cancellation while preserving
  completed sidecars and valid sidecars that existed before the operation.
- Allow the same conversion request to be started again after cancellation.
- Stop Asset Info progress polling cleanly when cancellation completes and keep
  the cancelled state distinct from an error state.
- Preserve the canonical `subtitles` capability for source classification,
  format selection, OCR, cue normalization, artifact identity, and validation.
- Out of scope: a new Queue/job subsystem, subtitle translation, new sidecar
  discovery or matching rules, changes to canonical OCR semantics, cancellation
  of unrelated Asset Info analysis/actions, and media/source deletion.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Add cancellation and cancellation-safe lifecycle behavior to the
  existing Asset Info subtitle extraction/conversion operation while preserving
  the canonical subtitle planning and artifact contract.

## Impact

- Backend Asset Info operation endpoints, operation state transitions, subtitle
  extraction and OCR orchestration, child-process execution, staging cleanup,
  and artifact publication guards.
- Frontend Asset Info API types/client, Cancel control, operation progress state,
  polling and retry behavior.
- Focused backend tests for text, PGS, and VobSub cancellation; process-tree
  termination; staging cleanup; existing-artifact preservation; cancelled versus
  failed outcomes; and retry. Focused frontend tests will cover the Cancel
  action, terminal-state polling, and retry affordance.
- No persistent job migration is expected because cancellation remains within
  the existing in-process Asset Info operation model.
