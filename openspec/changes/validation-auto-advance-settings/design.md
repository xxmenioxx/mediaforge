# Design: automatic Validation advancement Settings

## Current contract

The `validation` AppSetting already carries `autoAdvanceOutcomes`. Missing and empty values are interpreted as PASS-only. Auto Publisher has a separate `pipelineAutomation.autoPublisherEnabled` setting. Canonical Validation findings distinguish advisory/actionable findings from blocking findings, and Publisher independently checks Result identity and delivery readiness.

## Decisions

### Keep the existing persisted setting and API

The Validation card will use the existing `validation` Settings row. Its draft always represents PASS as allowed and may include `PASS_WITH_WARNINGS`; it never offers FAIL or REVIEW_REQUIRED. Saving merges the edited draft over the current setting value so extension keys survive. Preset selection and restore-defaults operations retain automatic outcomes and extension keys.

The API mutation and query invalidation remain in SettingsPage. A successful Settings response remains the source used to reload the card. No backend schema, policy evaluator, or Publisher path changes are needed.

### Make warning opt-in explicit

The card shows the currently effective stored policy, calling out that a missing or empty list means PASS-only. It shows Auto Publisher's current enabled or disabled state separately. Enabling `PASS_WITH_WARNINGS` first opens a confirmation dialog explaining that an enabled Auto Publisher may attempt eligible warning jobs already at `ready_to_publish` on its next tick, as well as future jobs. Confirmation changes the draft; the operator must still save it.

The card explains that actionable advisory findings remain visible, blocking findings still prevent handoff, and Publisher identity, sidecar, destination, replacement, and other delivery checks remain independent. No job count is shown because this UI has no authoritative count source.

## Compatibility and safety

Existing missing and empty policies remain effectively PASS-only. The UI does not change backend defaults or rewrite any job or ValidationRun. An explicit saved warning opt-in affects authorization at the existing Auto Publisher check. Auto Publisher must separately be enabled; the UI will show when it is off. Existing Publisher gates and immutable evidence remain authoritative.

## Validation

Focused component tests cover the default, empty and explicit policy states, reloading a saved value, confirmation scope, merge-safe saves, preset behavior, and the disabled Auto Publisher message. The repository's `./scripts/verify.sh --auto` remains the canonical verifier.
