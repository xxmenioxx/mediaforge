## Why

The #180 Assets V2 Library already provides a compact, scan-first workspace, while source assets still use a separate Unprocessed view with title accordions and per-path tables. Issue #201 brings both inventory locations into one coherent Assets workspace and aligns its proportions and selected-asset presentation with the approved mockup without changing inventory, Queue, profile, Advisor, Preview, or lifecycle authority.

## What Changes

- Expose `RAW | Library` as the only canonical top-level Assets contexts, using the existing `location` URL parameter and removing incompatible context-specific filter and selection state when switching.
- Present RAW source inventory in the V2 workspace while preserving the Source Group → Logical Group / Title → Path → Asset hierarchy and its existing workflows.
- Reuse shared workspace presentation for hierarchy, scan-first rows, selected-asset detail, and responsive layout while retaining context-specific models and actions.
- Tune workspace proportions, density, typography, dividers, selection treatment, path actions, preview size, and summary presentation against the approved mockup.
- Add focused regressions for context switching, RAW hierarchy and actions, preserved Library behavior, selected detail, accessibility, and responsive structure.

## Capabilities

### New Capabilities

- `assets-v2-raw-library-fidelity`: RAW and Library inventory contexts share an accessible, responsive Assets V2 workspace while retaining their distinct hierarchy and existing domain workflows.

### Modified Capabilities

None. No existing main spec defines the Assets workspace; this change adds its user-visible contract without changing backend or lifecycle requirements.

## Impact

- Frontend: `AssetsPage`, shared Assets workspace components, existing application navigation only if needed, and focused Assets tests.
- URL compatibility: retain current `location=raw|library` and applicable Library filters; continue supporting legacy deep links while keeping RAW and Library local hierarchy/selection state isolated.
- Backend and APIs: no changes. Existing grouped inventory responses, persisted counts, Queue Preflight, Advisor, configuration, Asset Info, Preview, profile assignment, and Test Encode flows remain authoritative.
- Out of scope: new inventory models/endpoints, Queue or Test Encode lifecycle changes, metadata enrichment, new Advisor/qualification behavior, and top-level Converted, Archive, Reports, Validation, Publisher, or Analysis contexts.
