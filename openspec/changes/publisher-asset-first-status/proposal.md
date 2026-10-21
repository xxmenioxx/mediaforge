## Why

Publisher currently places persisted publication attempts before eligible work, hiding the jobs that need attention. An asset-first operational list will make current work visible while retaining structured publication evidence and the complete History view.

## What Changes

- Replace the separate attempts-first and eligibility lists with one searchable, filterable current-state list and compact bounded counts.
- Correlate only the current eligibility candidate with an exact same-job execution publication; retain every distinct publication and attempt identity.
- Keep attempt, audit, archive, and validation evidence available on demand, and link to the full History route.
- Explain that both API windows and displayed evidence are bounded; do not imply global totals.
- Preserve existing backend authorization, eligibility, validation, confirmation, reason, retry, hold, release, publish, and discard contracts.

## Capabilities

### New Capabilities
- `publisher-asset-first-status`: Asset-first unified Publisher state, bounded summaries, and preserved access to structured evidence and full history.

### Modified Capabilities

## Impact

- Frontend Publisher page and focused UI tests only; existing Publisher read models and mutations remain authoritative. No API, storage, or dependency changes.
