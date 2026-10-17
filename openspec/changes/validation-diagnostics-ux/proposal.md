## Why

Validation details currently put canonical state, nested evidence, expected and actual values, and override controls into one dense table. This makes it hard to understand the outcome and next action, while the existing immutable run data is not available as a dedicated technical evidence view.

## What Changes

- Present each Validation run with its asset/job/run identity, canonical outcome, score, finding counts, and effective Publisher handoff as distinct facts.
- Replace the dense evidence table with compact diagnostics that use friendly labels, explicit states, concise explanations, and expandable detail.
- Add an on-demand technical evidence view for the complete immutable run JSON, including copy and JSON download controls tied to the run identity.
- Preserve exact-run Skip and Skip all override flows, displaying override events as audit history without changing canonical finding states.
- Keep the work frontend-only and compatible with historical run responses that omit optional evidence.

## Capabilities

### New Capabilities

- `validation-diagnostics`: Human-facing Validation diagnostics and on-demand technical evidence presentation for immutable runs.

### Modified Capabilities

None. This change does not alter Validation or Publisher decisions, scoring, policy, or API behavior.

## Impact

- Frontend: `ValidationPage` and isolated presentation components/formatters; focused tests for statuses, historical evidence, A/V details, overrides, and JSON export interactions.
- API: Reuse the existing `ValidationRunDetail` response (`run`, exact-run `overrides`, `effectiveHandoff`, and policy metadata). No endpoint or API type change is planned.
- Backend, Validation engine, Publisher, persistence, and media processing: no changes.
- Dependencies and runtime resource usage: no new dependency, media probe, filesystem read, or background work.
- Out of scope: validation scoring/thresholds, canonical dispositions, override authorization, Publisher eligibility, policy snapshots, and run immutability.
