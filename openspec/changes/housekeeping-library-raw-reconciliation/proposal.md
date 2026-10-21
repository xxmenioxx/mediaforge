## Why

Housekeeping currently describes some maintenance temporaries and blocked RAW directories with generic reasons. Operators cannot tell whether a referenced file still exists, whether a maintenance operation or recovery backup still depends on it, or which persisted record protects an empty directory. This change makes diagnosis actionable while keeping deletion behind the existing preview and execution safeguards.

## What Changes

- Classify maintenance temporary references using the persisted operation state and exact filesystem evidence; distinguish absent history, possible residuals, active/recovery work, and ambiguous cases.
- Permit manual cleanup only for an exact residual temporary whose Library role, operation ownership, identity, backup dependencies, references, and filesystem identity are verified at preview and revalidated before execution.
- Expose structured protection evidence for empty RAW directory candidates, including the records, fields, paths, and lifecycle states that protect them.
- Keep suspected stale references blocked until canonical inventory/reconciliation independently resolves them; preserve non-recursive removal of a truly empty leaf through Housekeeping.

Out of scope: automatic Library cleanup, general Library/RAW deletion permissions, force-delete, recursive RAW removal, changing historical Queue/Publisher/maintenance records, production/NAS cleanup, and changing retention policy.

## Capabilities

### New Capabilities
- `housekeeping-reconciliation`: truthful maintenance-temporary and empty-RAW-directory diagnostics with narrowly authorized cleanup through the existing Housekeeping lifecycle.

### Modified Capabilities
- None.

## Impact

- Backend Housekeeping preview/execute candidates, maintenance-operation cleanup evidence, and RAW protection diagnostics.
- Additive Housekeeping response types and the existing Settings Housekeeping presentation.
- Disposable-filesystem and SQLite fixtures for preview, execution, stale identity, backup/reference, and RAW protection cases.
- No persisted schema or migration is intended; no NAS access or production data changes.
