## Why

Publisher currently filters completed Queue jobs by legacy `validationStatus`, so a `REVIEW_REQUIRED` Result with valid Publisher-scoped overrides is hidden even when canonical Validation authorizes manual publication. Operators also need backend reasons for jobs that remain blocked, without a listing request changing Queue or filesystem state.

## What Changes

- Add a bounded, read-only Publisher eligibility view based on the canonical ValidationRun, exact Result identity and output revision, override evidence, and frozen Publisher delivery requirements.
- Show manually eligible override-backed jobs with an explicit status, operator reason, and confirmation before publishing; show blocked jobs with the server's actionable reason.
- Revalidate current Validation, Result, sidecar, and delivery constraints when manual publication is requested. Keep automatic publication authorization separate.
- Preserve immutable ValidationRun findings and append-only override evidence.

## Capabilities

### New Capabilities

- `publisher-manual-eligibility`: Read-only bounded eligibility for manual Publisher handoff and explicit handling of override-backed and blocked completed jobs.

### Modified Capabilities

- None.

## Impact

- Backend Publisher handler, routes, and focused delivery/validation tests.
- Frontend Publisher API types/client, page, and focused tests.
- No persistence schema change is expected. QueueJob, staging, archive, ValidationRun, and override records are not mutated to list candidates.

Out of scope: changing Validation policy, adding a new override scope, changing automatic advancement, or modifying unrelated Queue lifecycle behavior.
