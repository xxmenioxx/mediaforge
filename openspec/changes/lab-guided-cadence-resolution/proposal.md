## Why

LAB currently exposes an opaque planning failure when ambiguous cadence leaves Auto GOP without a known effective output FPS. A Preserve preview can also remain unverified under Canonical Frame Fidelity, so operators need backend-grounded alternatives that explain both gates without claiming that every source can be made Safe.

## What Changes

- Add a versioned, backward-compatible LAB resolution diagnostic for cadence/GOP planning blocks, including evidence identity, requested and effective motion state, blocking fields, and eligible actions.
- Evaluate only a bounded set of alternatives through existing cadence, motion, GOP, HEVC, preview, Test Encode, and Fidelity authorities; keep ambiguous or stale evidence review-blocked.
- Present asset-scoped Apply, Test alternative, and eligible Deep Cadence actions in LAB. Applying a choice must not alter global defaults or an already frozen Queue execution.
- Distinguish planning readiness from sample output fidelity. Only authoritative canonical fields and coverage/metric policy can produce Safe.

## Capabilities

### New Capabilities

- `lab-guided-cadence-resolution`: Explain blocked Auto cadence/GOP plans and offer bounded, evidence-backed asset alternatives in LAB.

### Modified Capabilities

None.

## Impact

- Backend LAB/preflight and cadence/Fidelity handlers plus focused Go tests.
- LAB API types and `ProfileLabPage` presentation plus focused frontend tests.
- No database migration, global profile mutation, Queue lifecycle change, new dependency, or automatic enqueue/publish. Existing error fields and HTTP safety semantics remain compatible.
