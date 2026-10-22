## Why

Manual Publish currently keeps its HTTP request open during NAS verification, delivery, archival, and reconciliation. A durable accepted request lets the API acknowledge operator intent promptly while Publisher continues to own safe delivery and recovery.

## What Changes

- Persist manual publication intent and its audit reason on the existing Publisher publication identity, then let the existing Publisher worker perform delivery and recovery.
- Return an explicit accepted response only after the intent is stored; expose accepted, in-progress, published, and failed states through the existing bounded publication read model.
- Make repeat clicks resolve to the existing active request and require the audited retry action for a new attempt after failure.
- Update Publisher UI to show accepted and final outcomes separately, with stored progress and actionable failures.
- **BREAKING**: Manual Publish returns `202 Accepted` with request identity and accepted state instead of waiting for a final delivery result.

## Capabilities

### New Capabilities
- `publisher-manual-request-lifecycle`: Durable acceptance, idempotent processing, recovery, and status reporting for manual Publish requests.

### Modified Capabilities

## Impact

- Backend Publisher handler, persisted publication model, existing AutoPublisher cadence, routes and API response types.
- Publisher page status presentation and focused backend/frontend regressions.
- SQLite schema evolves through the existing model migration path. No new Queue, generic job table, NAS operation, or external dependency is introduced.
