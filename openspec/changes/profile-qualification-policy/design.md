## Context

#69 persists a `PerceptualEvidenceSet` with explicit source, reference, candidate, coverage, metric, and window identities. Test Encode stores this set inside its existing validation report after using the frozen #70 plan. #70 already records role-based windows, including `typical`, `high_motion`, `high_texture_or_grain`, and `dark_or_gradient`. The current LAB `PerceptualEvidencePanel` presents those frozen windows and metric evidence without universal thresholds. #42 BPPPF is frozen separately into Preflight `qualityEvidence`. #46 copies reviewed quality evidence into the accepted `ExecutionPlan`. #53 already defines the four outcome strings and independently validates the actual Result.

The current #69 `ArtifactEvidence` projection contains restoration-analysis context, including source-side severity; it does not prove candidate-versus-reference artifact regression for every domain. Qualification must require same-window reference and candidate severity before evaluating a regression. Source-only severity is retained as context and cannot fail a candidate. Current unsupported domains stay unavailable. No new analyzer is introduced here.

## Goals and Non-Goals

**Goals**

- Evaluate one existing Test Encode candidate against identity-matched evidence and one resolved profile fingerprint.
- Explain every policy rule and retain the worst relevant frozen evidence window.
- Persist immutable, narrow decisions and distinguish stale from current evidence.
- Project qualification into LAB and Preflight; freeze exactly what was reviewed.
- Keep all quality evaluation bounded by existing #70 windows and persisted #69 evidence.

**Non-Goals**

- Candidate generation/search, Pareto ranking, or automatic tuning (#71).
- Historical/cohort calibration (#72), cohort reuse, or encode recipes.
- New sampling, perceptual metrics, artifact analyzers, compatibility engines, or Test Encodes.
- Skipping or modifying #53 Validation.

## Decisions

### 1. The evaluator is a pure per-candidate boundary

Add `EvaluateProfileQualification(candidate, evidence)` in the existing backend handler domain, adjacent to the #69 evidence types. The candidate adapter carries the resolved `models.Profile`, source identity, Test Encode/candidate identity, frozen plan, and persisted evidence. The evaluator performs no database, filesystem, FFmpeg, or Queue operation. Future #71 code can call this function once per generated candidate without changing rule policy.

Profile fingerprints use a versioned canonical JSON digest of the complete resolved profile definition, profile version, and template identity. Profile ID remains a separate lookup field and is never sufficient as the fingerprint. Candidate configuration/output identity is bound independently by the canonical #69 `CandidateIdentity`.

### 2. Policy defaults are named product policy, not universal metric truth

The active ruleset records `profile-qualification-policy-v1` and source `mvforge_policy_defaults`. It uses the existing optimization-intent values `maximum_savings`, `balanced`, `conservative`, `maximum_quality`, and `archive`. Only VMAF receives an initial metric floor because it has a stable higher-is-better scale in the current evidence contract. Other available metrics remain visible as canonical evidence and are not assigned guessed cross-metric thresholds.

Intent defaults (typical VMAF floor / worst covered frozen-window floor):

| Existing intent | Typical | Worst |
| --- | ---: | ---: |
| `maximum_savings` | 90 | 82 |
| `balanced` | 92 | 85 |
| `conservative` | 94 | 88 |
| `maximum_quality` | 96 | 91 |
| `archive` | 97 | 93 |

These are explicit initial MVForge defaults; they are not claims of scientifically universal acceptability. A missing/unknown intent has no applicable default and requires review.

Artifact policy consumes only evidence with the same frozen window and comparable reference/candidate identities. A source-only artifact signal is not a regression measurement. Initial allowed severity increase is zero severity steps for supported domains; a high-confidence blocking regression beyond tolerance is a hard failure, and ambiguous/low-confidence comparisons require review. Unsupported temporal/chroma domains remain unavailable. No artifact signal is folded into VMAF or another metric.

### 3. Evidence gaps reduce confidence without fabricating failures

The evaluator verifies Test Encode ID, source fingerprint/revision, profile fingerprint, plan identity/version, evidence-set identity, candidate artifact/generation/configuration identity, and reference manifest identities before evaluating rules. It consumes only stored frozen windows and their existing roles. A stale identity or missing candidate comparison is `REVIEW_REQUIRED`; a domain explicitly unsupported by the runtime is unavailable/advisory and never `FAIL`.

Typical and worst-window VMAF rules are emitted in frozen plan order. Missing `typical` or selected hard-scene evidence produces an explicit review rule. Rules are ordered by the policy's stable domain order and then by frozen window order. Worst-window selection orders outcomes `FAIL`, `REVIEW_REQUIRED`, warning, pass, then severity and frozen plan order.

### 4. Qualification is immutable and references Test Encode evidence

Add one `profile_qualifications` table with a stable qualification ID, Test Encode ID, asset/path/source identity, resolved-profile fingerprint, candidate/reference/evidence identities, SamplingPlan and policy versions, outcome, ordered rules/warnings, worst-window summary, provenance, and timestamp. Add indexes for Test Encode and `(source identity, profile fingerprint, policy version, created_at)`. Add idempotent append-only triggers. The row references the canonical evidence embedded in Test Encode; it does not duplicate full metric or media evidence.

New Test Encodes persist the resolved-profile fingerprint and bounded source stat revision used by Preflight. Existing records keep empty values and are never backfilled into qualifying evidence. A current effective-asset Test Encode already marks changed source/configuration stale; a LAB draft result remains scoped to that exact frozen fingerprint.

Evaluation runs as bounded CPU work over the persisted evidence after Test Encode completion, with an explicit evaluate endpoint for an already-retained candidate. Repeated evaluation of the same Test Encode/evidence/policy identity returns the existing immutable record. Missing evidence cannot synthesize `PASS`.

### 5. LAB reuses existing Test Encode details and evidence presentation

The existing Test Encode inventory is already loaded in batches by its dialog. Extend that response with the latest qualification per Test Encode using one batched query, add an outcome chip and focused Qualification details, and index the already-persisted #69 evidence windows by their frozen identities. Selecting the stored worst-window identity navigates to that existing evidence entry, with its timestamp, pair identity, metrics, and reference/candidate identities. The feature creates no preview or media copy and adds no second comparison player.

### 6. Preflight copies the reviewed identity, not a new evaluation

During resolved Preflight, collect all candidate source/profile pairs and load matching ready Test Encodes and current-policy qualification rows in bounded batch queries capped at 10,000 rows each. Only exact source revision and resolved-profile fingerprint matches are current. Put the qualification record ID, outcome, policy, evidence and candidate identities, rule summary, and stale/not-qualified reason under the existing per-asset `qualityEvidence`. Add a warning for warnings/review/fail or missing/stale evidence; never turn these into a hard blocker by default.

Acceptance already copies `qualityEvidence` into the created `ExecutionPlan.Evaluation` and labels its decision source `batch_preflight_frozen`. Preserve that path. Worker and #53 Validation do not recompute qualification. Final Validation continues its independent actual-Result checks and four-outcome aggregation.

### 7. #42 and future integrations remain separate

The existing BPPPF data stays a separate heuristic in `qualityEvidence`; it cannot satisfy a measured metric or create an artifact failure. The exported per-candidate evaluator is the only intended #71 seam. It accepts one resolved profile, candidate identity, frozen plan, and evidence, and returns ordered results; candidate generation, size/cost comparisons, selection, and historical learning remain out of scope.

## Risks / Trade-offs

- A short Test Encode may not cover all #70 hard-scene windows. Such a candidate can still show measured evidence but may require review; the evaluator must not request another encode.
- Current artifact projection may provide source-side context without a candidate comparison. This can increase review outcomes but cannot create a false regression failure.
- Initial VMAF floors are product defaults and may need later calibration; the stored policy version makes changes auditable.
- Profile fingerprint and source revision fields are empty for old Test Encodes by design, so these rows remain unavailable to Preflight until evaluated with sufficient current evidence.

## Migration Plan

1. AutoMigrate the new qualification table and nullable/default-empty identity columns on Test Encode.
2. Create qualification indexes and idempotent append-only triggers.
3. Do not backfill profile fingerprints or qualifications for historical Test Encodes.
4. New candidates persist their source revision and resolved profile fingerprint before execution; any failure leaves existing Test Encode lifecycle handling authoritative.

## Validation Plan

- Focused pure-policy tests for thresholds, rule ordering, identity/staleness, artifact deltas, evidence gaps, and all four outcomes.
- Focused SQLite tests for table/index migration, append-only behavior, and no synthetic historical qualification.
- Focused handler tests for Test Encode persistence/currentness and Preflight exact-match/frozen provenance.
- Focused frontend tests for qualification detail/provenance/worst-window navigation and Preflight status presentation.
- `npm run lint`, `npm run build`, strict OpenSpec validation, `./scripts/verify.sh --auto`, and `./scripts/verify.sh --full`.
