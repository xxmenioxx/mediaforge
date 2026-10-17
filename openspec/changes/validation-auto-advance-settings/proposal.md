# Expose automatic Validation outcomes in Settings

## Problem

The Validation Settings card omits `validation.autoAdvanceOutcomes` when it saves. Operators cannot inspect or explicitly configure warning advancement through the UI, and missing or empty policy values retain the backend's PASS-only behavior. Enabling warning advancement can also affect jobs already waiting at `ready_to_publish`, so the control needs to explain that scope before it records an opt-in.

## Outcome

Expose the effective automatic outcome policy and Auto Publisher state in the existing Validation Settings card. Keep PASS enabled, offer only an explicit optional `PASS_WITH_WARNINGS` opt-in, and require confirmation that includes existing ready jobs. Preserve other Validation setting keys and preset behavior across edits and reloads.

## Scope

- Read and save `validation.autoAdvanceOutcomes` through the current Settings API.
- Explain PASS-only legacy defaults, the Auto Publisher disabled state, advisory/actionable findings, and independent Publisher delivery checks.
- Confirm the scope of enabling warning advancement before the opt-in enters the editable draft.
- Add focused frontend coverage for legacy values, reloads, merge-safe saves, preset changes, and confirmation behavior.
- Record the user-visible contract and implementation plan in OpenSpec.

## Out of scope

- Changing backend Validation or Publisher policy, deployment defaults, or existing jobs.
- Enabling warning advancement automatically or modifying NAS/production data.
- Computing a count of potentially affected jobs without an authoritative backend endpoint.
