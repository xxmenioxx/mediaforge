## Why

Test Encode currently fingerprints a resolved profile before the media plan applies its final automatic decisions, while Qualification reads the profile saved after planning. Queue Preflight compares yet another restored representation of that profile, and the current fingerprint also includes per-asset analysis context. These mismatched identities can mark valid evidence stale or hide it from Preflight; collapsing them would also risk reusing one asset's evidence as proof for another.

## What Changes

- Define separate, versioned identities for reusable profile definitions, frozen effective execution configuration, and source/candidate/evidence.
- Capture and compare each identity at the stage whose immutable snapshot it describes across Test Encode, Qualification, #69 evidence, and Queue Preflight.
- Preserve assignment-source independence for profile definitions while keeping effective overrides and automatic encoding decisions in the execution identity.
- Explain mismatches and legacy evidence with explicit stale or review causes; preserve append-only history and never trigger re-encoding during review.
- Keep Qualification advisory for Preflight and leave authoritative Validation behavior unchanged.

Out of scope: #306, candidate search, new quality metrics, changes to profile assignment precedence, and changes to final Validation policy.

## Capabilities

### New Capabilities

- `quality/frozen-profile-evidence-identity`: Versioned identity and evidence-matching behavior shared by Test Encode, Profile Qualification, #69 evidence, and Queue Preflight.

### Modified Capabilities

None. Existing `asset-defaults` assignment and precedence requirements do not change.

## Impact

- Backend profile snapshots, Test Encode persistence and execution planning, Qualification persistence/evaluation, Queue Preflight matching, and #69 candidate identity inputs.
- Focused backend regression tests and the Queue Preflight architecture document.
- Additive persistence fields, if required, must migrate idempotently; existing records remain untouched and are classified explicitly when they lack the new identity contract.
