## Purpose

Specify safe Settings control of automatic Validation advancement outcomes.

## ADDED Requirements

### Requirement: Settings expose the effective automatic Validation outcome policy

The Validation Settings UI SHALL expose the effective `validation.autoAdvanceOutcomes` policy. PASS SHALL always be allowed. The UI MAY allow explicit `PASS_WITH_WARNINGS`; it SHALL NOT offer FAIL or REVIEW_REQUIRED. Missing or empty settings SHALL be presented as the effective PASS-only default. The UI SHALL show Auto Publisher's enabled or disabled state separately from the outcome policy.

#### Scenario: Legacy or empty policy is displayed

- **WHEN** `autoAdvanceOutcomes` is missing or empty
- **THEN** Settings displays the effective policy as PASS-only
- **AND** saving Validation rule changes preserves that effective behavior

#### Scenario: Warning advancement is explicitly selected

- **WHEN** an operator confirms and saves the optional warning outcome
- **THEN** Settings writes PASS and PASS_WITH_WARNINGS through the existing `validation` setting
- **AND** FAIL and REVIEW_REQUIRED cannot be selected

#### Scenario: Auto Publisher is disabled

- **WHEN** `pipelineAutomation.autoPublisherEnabled` is false
- **THEN** Settings identifies Auto Publisher as disabled independently of the Validation outcome policy

### Requirement: Warning advancement confirmation describes its scope

Settings SHALL require explicit confirmation before adding PASS_WITH_WARNINGS to the draft. The confirmation SHALL explain that an enabled Auto Publisher may attempt eligible existing jobs already at `ready_to_publish` with PASS_WITH_WARNINGS at its next tick, as well as future jobs. It SHALL state that confirmation alone does not start publication and that Publisher continues to enforce independent identity and delivery checks.

#### Scenario: Operator cancels warning opt-in

- **WHEN** the operator dismisses the confirmation
- **THEN** PASS_WITH_WARNINGS remains absent from the draft and is not saved

#### Scenario: Advisory and blocking findings are distinguished

- **WHEN** the warning option is presented
- **THEN** Settings explains that actionable advisory findings remain visible for follow-up and blocking findings still prevent handoff
- **AND** Publisher identity, sidecar, destination, replacement, and other delivery checks remain independent

### Requirement: Validation Settings saves preserve policy data

Saving Validation Settings SHALL retain other top-level keys in the current `validation` setting and preserve automatic outcome values across preset selection and restore-defaults actions. The card SHALL reflect the value returned by Settings after reload or query refresh.

#### Scenario: Validation setting contains additional fields

- **WHEN** Validation rule policy or automatic outcomes are saved
- **THEN** unedited top-level Validation setting fields remain present

#### Scenario: Saved outcome policy is reloaded

- **WHEN** Settings returns a saved `autoAdvanceOutcomes` value
- **THEN** the card displays the corresponding effective outcome and checkbox state
