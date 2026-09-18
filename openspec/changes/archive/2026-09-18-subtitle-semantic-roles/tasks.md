## 1. Canonical role contract and evidence

- [x] 1.1 Add the canonical role values, provenance, confidence, classifier
  contract version, source snapshot binding, stream signature, and JSON
  compatibility fields to the existing subtitle stream and artifact models;
  verify stream index remains the stable identity through encode/decode and
  older payloads remain readable.
- [x] 1.2 Implement deterministic role evidence precedence of manual override,
  explicit semantic metadata, unambiguous dispositions, bounded heuristics, and
  safe `unknown`; verify no OCR or unbounded media read is invoked during role
  classification.
- [x] 1.3 Cover explicit `full`, `forced`, `signs`, `commentary`, and `sdh`
  evidence, source-disposition/role independence, `signs`/`forced` behavior,
  conflicting evidence, and missing evidence; verify each result includes
  actionable provenance/confidence and ambiguous cases remain `unknown`.

## 2. Overrides and canonical plan resolution

- [x] 2.1 Add per-asset/per-stream manual role override persistence and API
  handling bound to source snapshot identity and relevant stream signature;
  verify stale overrides fail closed, are not moved by stream-index reuse, and
  automatic classification is used for the current stream.
- [x] 2.2 Add Asset Info role, provenance, confidence, manual state, and
  reset-to-auto behavior; verify reset removes only the selected stream override
  and recomputes the current automatic role.
- [x] 2.3 Extend `resolveTrackPlan` through the existing `subtitleRules` model
  so rules match optional language, stream index, and roles alongside existing
  action/sidecar/OCR fields; verify absent roles are wildcards and existing
  stream-specific-over-language/path-safe precedence is unchanged.
- [x] 2.4 Preserve independent same-language stream decisions through resolved
  subtitles, sidecars, and role-aware selection; verify stream indexes and roles
  remain distinct even when language values match.

## 3. Track Profile and Queue integration

- [x] 3.1 Add optional semantic-role selectors to Track Profile subtitle rules
  and read/edit/save UI contracts; verify legacy language-only rules round-trip,
  role-specific policies coexist by language, explicit `unknown` is matchable,
  and configured rules select only matching roles.
- [x] 3.2 Freeze role, provenance, confidence, override state, source snapshot
  binding, stream signature, and classifier evidence version in the existing
  `ResolvedTrackPlan`/Queue snapshot; verify a profile or metadata change after
  freeze cannot alter the stored role decision.
- [x] 3.3 Update Worker consumption to use the frozen role-aware plan without
  reclassification or a second resolver/job path; verify execution uses the
  snapshot role and preserves existing subtitle/OCR planning.

## 4. Artifact and publication propagation

- [x] 4.1 Carry canonical role metadata with original and converted subtitle
  artifacts, including logical `both`, without changing stream identity,
  subtitle text/timing, OCR, bitmap geometry, ASS positioning, native ASS/SSA
  extraction, cancellation, or atomic publication; verify focused artifact
  metadata tests.
- [x] 4.2 Derive effective forced publication from source `forced=true` OR
  `role=forced`; require supported external `.forced` and `.sdh` semantics and
  forced/commentary/hearing-impaired muxed dispositions where available; verify
  `signs + forced=true`, `signs + forced=false`, `sdh + forced=true`,
  `role=forced` with source false, existing forced sidecar naming, independent
  original dispositions, and no invented `.signs` convention.
- [x] 4.3 Verify role metadata follows sidecar creation, Queue publication, and
  existing artifact/provenance records without publishing a new raw/intermediate
  subtitle path.

## 5. Regression coverage

- [x] 5.1 Add backend classifier coverage for all explicit roles, disposition
  precedence, signs without forced, ambiguity-to-unknown, bounded evidence, and
  the no-OCR classification guarantee.
- [x] 5.2 Add override and Asset Info API/UI coverage for manual assignment,
  same-language isolation, snapshot/signature mismatch, stale/ignored display,
  provenance/confidence, and reset-to-auto.
- [x] 5.3 Add Track Profile resolver and frontend coverage for `subtitleRules`
  role selectors, language-specific role policies, stream-specific precedence,
  legacy language-only compatibility, explicit `unknown`, and same-language
  stream selection.
- [x] 5.4 Add Queue snapshot and Worker coverage proving role decisions freeze at
  preparation and are consumed unchanged after profile or source metadata changes.
- [x] 5.5 Add role-aware publishing and artifact coverage for effective forced
  disposition mapping, required `.forced`/`.sdh` sidecars, `signs + forced=true`,
  `signs + forced=false`, `sdh + forced=true`, `role=forced` mapping, existing
  forced sidecar naming, forced/commentary/hearing-impaired muxed dispositions,
  unsupported targets, original ASS/SSA, converted SRT/ASS/both,
  geometry-aware ASS, and preservation of existing publication barriers.
- [x] 5.6 Run focused subtitle, resolver, Queue, publishing, and frontend tests;
  verify the canonical subtitle/OCR pipeline remains the only planner and role
  resolution does not add OCR or duplicate media analysis.

## 6. Validation

- [x] 6.1 Run `openspec validate subtitle-semantic-roles --type change
  --strict` and verify all change artifacts pass strict validation.
- [x] 6.2 Run `./scripts/verify.sh --auto` and verify the repository reports
  `ALL CHECKS PASSED`.
