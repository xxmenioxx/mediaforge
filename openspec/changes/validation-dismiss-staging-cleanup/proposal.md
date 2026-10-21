## Why

Operators can dismiss completed jobs after reviewing failed or unresolved Validation, but must handle retained staging separately. This change offers an explicit, inspectable cleanup choice while preserving the established Dismiss transaction and Housekeeping ownership safeguards.

## What Changes

- Add a read-only, job-scoped staging preview before Dismiss, with an opt-in-by-default checkbox in the confirmation dialog.
- Keep Dismiss transactional and independent from filesystem cleanup; after a committed Dismiss, allow explicit cleanup bound to the preview and exact job identity.
- Persist append-only cleanup evidence and expose retained, cleaned, failed, or blocked status with bounded History retry.
- Make cleanup fail closed on stale identity, unsafe ownership/storage, active references, path changes, or filesystem races; keep general Housekeeping protection unchanged.
- Preserve historical Validation, Result, ExecutionPlan, reports, logs, and publication data.

## Capabilities

### New Capabilities
- `validation-staging-cleanup`: Explicit preview-bound staging cleanup associated with Validation Dismiss, including durable outcomes and History retry.

### Modified Capabilities
- None.

## Impact

Affected systems include Validation and History API/UI, the existing Housekeeping candidate and deletion engine, and durable backend cleanup evidence. No global retention policy changes, media processing, NAS operations, or deletion of historical records are included. Database changes require backward-compatible migration behavior.
