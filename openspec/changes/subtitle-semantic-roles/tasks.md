## 1. Canonical role contract and evidence

- [ ] 1.1 Add the canonical role values, provenance, confidence, classifier
  contract version, and JSON compatibility fields to the existing subtitle
  stream and artifact models; verify stream index remains the stable identity
  through encode/decode and older payloads remain readable.
- [ ] 1.2 Implement deterministic role evidence precedence of manual override,
  explicit semantic metadata, unambiguous dispositions, bounded heuristics, and
  safe `unknown`; verify no OCR or unbounded media read is invoked during role
  classification.
- [ ] 1.3 Cover explicit `full`, `forced`, `signs`, `commentary`, and `sdh`
  evidence, `signs`/`forced` independence, conflicting evidence, and missing
  evidence; verify each result includes actionable provenance/confidence and
  ambiguous cases remain `unknown`.

## 2. Overrides and canonical plan resolution

- [ ] 2.1 Add per-asset/per-stream manual role override persistence and API
  handling; verify overrides are keyed by stream index, override automatic
  evidence, and do not move between same-language streams.
- [ ] 2.2 Add Asset Info role, provenance, confidence, manual state, and
  reset-to-auto behavior; verify reset removes only the selected stream override
  and recomputes the current automatic role.
- [ ] 2.3 Extend `resolveTrackPlan` with role decisions and optional role
  predicates while preserving existing language-only behavior; verify absent
  role configuration does not change legacy selection and explicit `unknown`
  filtering behaves distinctly from an unconfigured filter.
- [ ] 2.4 Preserve independent same-language stream decisions through resolved
  subtitles, sidecars, and role-aware selection; verify stream indexes and roles
  remain distinct even when language values match.

## 3. Track Profile and Queue integration

- [ ] 3.1 Add optional semantic-role filtering to Track Profile read/edit/save
  contracts and UI controls; verify legacy profiles round-trip unchanged and
  configured filters select only matching roles.
- [ ] 3.2 Freeze role, provenance, confidence, override state, and classifier
  evidence version in the existing `ResolvedTrackPlan`/Queue snapshot; verify a
  profile or metadata change after freeze cannot alter the stored role decision.
- [ ] 3.3 Update Worker consumption to use the frozen role-aware plan without
  reclassification or a second resolver/job path; verify execution uses the
  snapshot role and preserves existing subtitle/OCR planning.

## 4. Artifact and publication propagation

- [ ] 4.1 Carry canonical role metadata with original and converted subtitle
  artifacts, including logical `both`, without changing stream identity,
  subtitle text/timing, OCR, bitmap geometry, ASS positioning, native ASS/SSA
  extraction, cancellation, or atomic publication; verify focused artifact
  metadata tests.
- [ ] 4.2 Add only supported publication mappings for `forced` and `sdh`, while
  retaining the canonical role and existing naming when no target convention is
  available; verify unsupported targets do not lose or reinterpret the role.
- [ ] 4.3 Verify role metadata follows sidecar creation, Queue publication, and
  existing artifact/provenance records without publishing a new raw/intermediate
  subtitle path.

## 5. Regression coverage

- [ ] 5.1 Add backend classifier coverage for all explicit roles, disposition
  precedence, signs without forced, ambiguity-to-unknown, bounded evidence, and
  the no-OCR classification guarantee.
- [ ] 5.2 Add override and Asset Info API/UI coverage for manual assignment,
  same-language isolation, provenance/confidence display, and reset-to-auto.
- [ ] 5.3 Add Track Profile resolver and frontend coverage for role filters,
  legacy language-only compatibility, explicit `unknown`, and same-language
  stream selection.
- [ ] 5.4 Add Queue snapshot and Worker coverage proving role decisions freeze at
  preparation and are consumed unchanged after profile or source metadata changes.
- [ ] 5.5 Add role-aware publishing and artifact coverage for forced/SDH
  conventions, unsupported targets, original ASS/SSA, converted SRT/ASS/both,
  geometry-aware ASS, and preservation of existing publication barriers.
- [ ] 5.6 Run focused subtitle, resolver, Queue, publishing, and frontend tests;
  verify the canonical subtitle/OCR pipeline remains the only planner and role
  resolution does not add OCR or duplicate media analysis.

## 6. Validation

- [ ] 6.1 Run `openspec validate subtitle-semantic-roles --type change
  --strict` and verify all change artifacts pass strict validation.
- [ ] 6.2 Run `./scripts/verify.sh --auto` and verify the repository reports
  `ALL CHECKS PASSED`.
