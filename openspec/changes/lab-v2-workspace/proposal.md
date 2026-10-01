## Why

The current Profile Lab puts asset selection, preview range, profile editing, fidelity tools, and Test Encode lifecycle in separate stacked areas. Test Encodes can be created and managed, but their retained outputs are not the primary comparison surface. The approved LAB V2 screenshot gives this workflow a compact context header, a focused parameter panel, an Original/Result comparison, and a small history strip.

## What Changes

- Recompose LAB around a compact asset context, the existing Path/Asset profile scope, one editable sample range, and one primary Test Encode action.
- Put the existing profile drafts and capability-driven controls in the parameter workspace without duplicating the profile selector or configuration resolver.
- Compare the source reference with a selected ready Test Encode, show only persisted metadata and canonical quality evidence, and keep previous runs discoverable from the retained inventory.
- Keep the existing AppLayout shell, Test Encode lifecycle, profile editing, compatible preview, Fidelity, cancellation, and qualification behavior.
- Apply the approved LAB V2 screenshot as visual authority and Assets V2/MUI as the production component and theme source.

## Capabilities

### New Capabilities

- `lab-v2-workspace`: LAB context, range, parameter, comparison, evidence, retained history, accessibility, responsive behavior, and performance contract.

### Modified Capabilities

None. The work changes LAB presentation and interaction while preserving existing Test Encode and quality-evidence contracts.

## Impact

- Frontend: `ProfileLabPage`, focused LAB presentation components, and LAB tests.
- OpenSpec: a new `lab-v2-workspace` capability.
- No backend API, persisted model, worker, scheduler, Queue, Validation, Publisher, profile resolver, media-analysis, or dependency changes are intended.
- Visual reference: `/Users/anuelvs/Documents/MediaForge/docs/design-reference/lab-v2.png`; the read-only Make reference implementation is under `/Users/anuelvs/Documents/MediaForge/docs/design-reference/lab-v2-make/`.
