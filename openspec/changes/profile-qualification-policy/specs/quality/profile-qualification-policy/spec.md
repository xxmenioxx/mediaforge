## ADDED Requirements

### Requirement: Profile Qualification SHALL evaluate one existing candidate against canonical evidence

The system SHALL expose one deterministic candidate evaluator that consumes the resolved profile, its full resolved-profile fingerprint, a source identity, an existing Test Encode candidate identity, the canonical #69 `PerceptualEvidenceSet`, and the frozen #70 `SamplingPlan` identities and windows. It SHALL NOT create candidates, select scenes, run a new Test Encode, or implement another quality metric. The evaluator SHALL provide a clean per-candidate boundary that a future bounded search such as #71 can call independently for each candidate.

#### Scenario: Existing candidate evidence is evaluated

- **WHEN** a ready Test Encode has complete, identity-matched canonical evidence for its resolved profile and frozen SamplingPlan
- **THEN** the evaluator returns one rule-by-rule qualification result for that candidate and records the evidence identities it consumed

#### Scenario: A candidate has no comparison evidence

- **WHEN** the candidate has no valid #69 source/reference-to-candidate evidence
- **THEN** the result SHALL be `REVIEW_REQUIRED` or unavailable and SHALL NOT be a passing qualification

### Requirement: Qualification identity SHALL fence stale source, profile, candidate, sampling, reference, and policy evidence

Each qualification SHALL bind the asset/source identity, complete resolved-profile fingerprint and profile version, candidate/Test Encode identity and output revision, #70 SamplingPlan schema/policy/plan identity, #69 evidence identity and reference identities, and qualification policy version. A profile database ID alone SHALL NOT identify a profile. A record SHALL be current only when every required identity matches the candidate being presented or reviewed. Policy changes SHALL make prior records stale for current policy decisions without rewriting history.

#### Scenario: Profile settings change without changing the database ID

- **WHEN** a profile's resolved configuration or version changes while its database ID remains the same
- **THEN** the prior qualification SHALL not be presented as current for the changed profile

#### Scenario: Source, candidate, sampling, or reference identity changes

- **WHEN** any bound source revision, candidate generation/output revision, SamplingPlan identity, or reference evidence identity differs
- **THEN** the qualification SHALL be reported stale or requalification-required

#### Scenario: Qualification policy changes

- **WHEN** the active qualification policy version differs from the stored result
- **THEN** the stored result SHALL remain immutable history and SHALL not be reused as the active qualification

### Requirement: Qualification policy SHALL be versioned, explainable, and intent-aware

The policy SHALL record a version and the existing canonical optimization intent. Initial thresholds and tolerances SHALL be explicitly identified as MVForge policy defaults, not universal scientific boundaries. Every evaluated rule SHALL retain its key, outcome, observed value, reference value where applicable, allowed tolerance or floor, window identity, explanation, and evidence provenance. Raw #69 evidence SHALL remain available by identity and SHALL not be collapsed into one opaque score.

#### Scenario: A supported perceptual floor is evaluated

- **WHEN** a comparable VMAF result is available for a required typical or worst frozen window
- **THEN** the policy SHALL apply the recorded default for the candidate's canonical optimization intent and explain the observed value and floor

#### Scenario: An optimization intent has no policy default

- **WHEN** the resolved profile intent is unknown or unsupported by the active policy
- **THEN** the result SHALL require review and SHALL explain that no intent-specific default exists

#### Scenario: BPPPF is present

- **WHEN** Preflight contains #42 BPPPF evidence
- **THEN** it MAY be shown as separate heuristic context and SHALL NOT replace measured #69 evidence or independently create an artifact `FAIL`

### Requirement: Artifact rules SHALL evaluate candidate-introduced regression only

Artifact rules SHALL consume only canonical, same-window, identity-bound reference and candidate severity evidence supported by the current evidence contract. Regression SHALL be evaluated as candidate severity minus reference/source severity against the recorded per-domain MVForge tolerance. Existing source artifact severity by itself SHALL NOT fail the candidate. The evidence contract SHALL remain separate from perceptual metric values.

#### Scenario: Source and candidate have equal artifact severity

- **WHEN** the source/reference already contains blocking or banding and the candidate has equal same-window severity
- **THEN** the artifact rule SHALL report no encode-introduced regression

#### Scenario: Candidate artifact severity increases beyond policy tolerance

- **WHEN** comparable same-window candidate blocking, banding, blur/detail loss, ringing/halo, or supported grain/texture evidence exceeds the reference by more than its recorded tolerance
- **THEN** the rule SHALL return the configured failure or review result and retain both severities and their identities

#### Scenario: Artifact evidence is unsupported or incomplete

- **WHEN** the runtime does not support a domain or the candidate lacks a comparable reference/candidate pair
- **THEN** unsupported evidence SHALL remain unavailable without becoming a false failure, while missing required comparison evidence SHALL require review

### Requirement: Qualification SHALL reuse frozen #70 scene windows and preserve the worst evidence

Qualification SHALL consume only the selected windows, identities, roles, reasons, and fallback provenance already frozen in the #70 SamplingPlan. It SHALL not discover scenes, choose timestamps, scan the asset, or copy preview media. Each rule SHALL identify the relevant canonical window and scene role/category. The result SHALL preserve the most severe failing or review-required window and enough #69 identity to open the existing Test Encode/evidence view.

#### Scenario: A hard-scene role was not frozen or covered

- **WHEN** required hard-scene evidence is absent or outside the existing candidate's coverage
- **THEN** the qualification SHALL be `REVIEW_REQUIRED` or warning-bearing and SHALL not manufacture evidence or return false `PASS`

#### Scenario: A worst window is available

- **WHEN** one or more rules fail or require review for frozen windows
- **THEN** the result SHALL deterministically identify the most severe window, its role/category, triggering rules, and existing candidate/reference evidence identities

### Requirement: Qualification outcomes SHALL aggregate deterministically

The system SHALL use the #53 outcome vocabulary: `PASS`, `PASS_WITH_WARNINGS`, `REVIEW_REQUIRED`, and `FAIL`. A hard failing rule SHALL produce `FAIL`; a manual-review or required-evidence gap SHALL produce `REVIEW_REQUIRED`; advisory warnings without failures or review findings SHALL produce `PASS_WITH_WARNINGS`; and `PASS` SHALL require all applicable required rules to pass. Rule order and explanations SHALL be stable for identical inputs and policy version.

#### Scenario: Hard failure outranks review and warnings

- **WHEN** a candidate has a hard failing rule plus other review or warning rules
- **THEN** the overall outcome SHALL be `FAIL` while preserving every rule result

#### Scenario: Review evidence outranks advisory warnings

- **WHEN** no rule hard-fails and at least one required rule is unresolved or explicitly review-required
- **THEN** the overall outcome SHALL be `REVIEW_REQUIRED`

#### Scenario: Only advisory warnings remain

- **WHEN** all required rules pass and one or more advisory warnings remain
- **THEN** the overall outcome SHALL be `PASS_WITH_WARNINGS`

#### Scenario: All required rules pass

- **WHEN** all required rules pass with no warnings, unresolved evidence, or failures
- **THEN** the overall outcome SHALL be `PASS`

### Requirement: Qualification persistence SHALL be immutable and compatible with history

Qualification decisions SHALL be stored in a narrow append-only, indexed persistence model that references the canonical Test Encode and evidence identities rather than creating another generic quality store. Migrations SHALL be idempotent. Historical Test Encodes without sufficient evidence or newly stored profile/source fingerprints SHALL remain unqualified until explicitly evaluated; migration SHALL not synthesize `PASS` results.

#### Scenario: Existing Test Encode has no qualification record

- **WHEN** a historical Test Encode lacks the required evidence or identity fields
- **THEN** it SHALL remain unqualified/unavailable and Preflight SHALL show that no current qualification exists

#### Scenario: Qualification history is re-evaluated

- **WHEN** evidence or policy version changes
- **THEN** the system SHALL append a new immutable result and preserve the prior decision

### Requirement: LAB SHALL present qualification using existing Test Encode evidence

The existing LAB/Test Encode workflow SHALL show the result, policy version, each rule explanation, warnings/failures, provenance identities, and worst frozen window. It SHALL navigate to the corresponding existing Test Encode/evidence window and SHALL use an existing comparison view when available. Rendering an existing result SHALL not create another Test Encode or copy preview media.

#### Scenario: Operator inspects an existing qualification

- **WHEN** a qualification exists for a ready Test Encode
- **THEN** LAB SHALL show the outcome, policy version, rules, provenance, worst window and existing candidate/reference identity

#### Scenario: Operator opens the worst evidence window

- **WHEN** the operator selects the stored worst window
- **THEN** LAB SHALL select that existing evidence window without selecting a replacement scene or generating another candidate

### Requirement: Queue Batch Preflight SHALL freeze the qualification reviewed by the operator

Preflight SHALL surface the latest current qualification only when asset/source identity, resolved-profile fingerprint, candidate/evidence identity, and active policy version match. `PASS` SHALL be informational, `PASS_WITH_WARNINGS` a warning, and `REVIEW_REQUIRED` or `FAIL` prominent review warnings. Missing or stale qualification SHALL be shown as not qualified for this asset/profile evidence. These states SHALL not hard-block V1.x acceptance by default. On acceptance, the exact reviewed qualification identity, outcome, policy version, and provenance SHALL be frozen in the accepted plan. Worker execution SHALL not recompute or silently change it.

#### Scenario: No current qualification exists

- **WHEN** Preflight has no qualification matching its source and resolved profile evidence
- **THEN** it SHALL show a not-qualified message without adding a default hard blocker

#### Scenario: Reviewed qualification is accepted

- **WHEN** an operator accepts a ready Preflight candidate after reviewing its qualification
- **THEN** the accepted Queue plan SHALL retain the exact qualification record and policy/evidence identity reviewed

#### Scenario: Worker executes an accepted plan

- **WHEN** the Worker claims and executes the frozen plan
- **THEN** it SHALL preserve the reviewed qualification provenance and SHALL not recompute qualification or alter the frozen execution

### Requirement: Qualification SHALL not weaken final Validation

Profile Qualification SHALL be pre-batch confidence only. #53 Validation SHALL independently evaluate the actual produced Result against its frozen execution plan. A qualification `PASS` SHALL not skip, soften, or authorize publication past any final Validation rule.

#### Scenario: Qualified profile produces a final regression

- **WHEN** a profile qualification passed but the actual final output violates a #53 Validation rule
- **THEN** Validation SHALL report its own authoritative failure/review outcome and preserve its normal Queue/Publisher behavior
