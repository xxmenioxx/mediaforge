## 1. Shared source model and frozen policy

- [x] 1.1 Extend the existing `assDocument`/`assEvent`/`assProtectedDialogue` model and `parseASSDocument` reconstruction boundary with the minimum source/style-section and provenance data needed to merge ASS/SSA documents, while preserving the Translation contract; verify existing ASS/SSA protected-structure round-trip tests remain green.
- [x] 1.2 Define the bounded composition event view, source identity/revision descriptor, `CompositionCueClass` values, layout-evidence completeness, and resolved-language representation; verify table-driven unit tests cover native ASS/SSA, SRT, OCR-derived cues, unknown language, and compound track classification without creating a new semantic role.
- [x] 1.3 Add the versioned bounded composition policy to Track Profile resolution through `resolveTrackPlan`, including primary/supplemental order, language authorization, role/class rules, style behavior, the non-overridable preferred/default structural gate plus explicit promotion request, bounded overrides, and the maximum source bound; verify identical inputs produce identical resolved plans and invalid policies fail closed.
- [x] 1.4 Freeze resolved composition inputs beside the existing Queue profile snapshot and style-policy snapshot; verify Track Profile snapshot determinism, post-freeze setting isolation, legacy snapshot compatibility, and preferred/default eligibility.

## 2. Canonical normalization and reconciliation

- [x] 2.1 Route ASS/SSA, SRT, and canonical OCR results into the shared ASS event/document flow, reusing the existing text and bitmap/OCR positioning paths without a composition-specific OCR or sidecar-discovery path; verify SRT normalization and bitmap single-result reuse tests.
- [x] 2.2 Implement evolving-canonical-set reconciliation with primary-first then stable supplemental precedence, provenance-aware candidate selection, bounded timing tolerance, and deterministic one-to-one ownership; verify exact dedupe, primary enrichment versus duplicate retention, repeated identical text at different timestamps, duplicate cue across two supplementals, primary plus duplicate in multiple supplementals, stable supplemental order/ownership, and ambiguous timing/text matches.
- [x] 2.3 Verify simultaneous top/bottom dialogue, sign, and song events remain independent ASS events, including equal/overlapping text and timing with distinct layout or role evidence; add focused regression tests for conflicting supplemental copies that assert conflict provenance, no silent overwrite, event count, order, timing, and position evidence.
- [x] 2.4 Apply the frozen #26 layout evidence and #123 `SubtitleStylePolicy`/`resolveSubtitleStyle` behavior without inferring roles from geometry; verify native ASS style/override preservation, explicit `\\an`/`\\pos`/`\\move` precedence, missing-layout behavior, and the explicit `CompositionCueClass=lyrics` → `ExplicitStyleID=MVF_Lyrics` → `resolveSubtitleStyle` chain without a Composition-specific style resolver.
- [x] 2.5 Reconcile same-named ASS styles and required font attachments deterministically, preserving source event references and using the existing attachment artifact model; verify style-collision tests, byte-identical attachment deduplication, deterministic conflicting-font renaming, and unresolved-attachment rejection.

## 3. Identity, provenance, and canonical artifact validation

- [x] 3.1 Implement the versioned logical output identity over canonicalized/ordered source identities and revisions, resolved languages/roles/classes, composition/reconciliation policy, style policy, representation, and publication mode; verify retries with the exact source set are idempotent and changed revisions/policies produce distinct identities without overwriting prior composites.
- [x] 3.2 Persist bounded composition provenance for kept-primary, deduplicated, enriched/repaired, added, ambiguous/conflicting, source tracks/sidecars, language/role resolution, attachment decisions, and output identity/path; verify counts and bounded summaries omit full cue bodies, full ASS documents, and OCR payloads.
- [x] 3.3 Validate staged canonical ASS output using the shared parser/document boundary and existing subtitle validation contracts; verify malformed style references, invalid protected structure, missing source decisions, forbidden implicit language composition, partial output, and identity mismatch fail before publication.

## 4. Track Profile, Queue, and Asset Info integration

- [x] 4.1 Make `resolveTrackPlan` expose backend-authoritative composition eligibility/decisions for primary and supplemental sources, same-language automatic selection, explicit cross-language policy, unknown-language rejection, role/class compatibility, and preferred/default gating; verify same-language automatic selection and cross-language rejection/default tests.
- [x] 4.2 Extend Asset Info subtitle eligibility with bounded composition candidates, exact revisions, roles/classes, source-combination support, attachment/conflict reasons, and allowed overrides; verify Asset Info eligibility/provenance tests and the no-new-sidecar-discovery boundary.
- [x] 4.3 Add the manual `Compose subtitles` request contract using explicit primary/supplemental source descriptors, observed revisions, and bounded overrides; verify backend revalidation rejects stale, unsupported, ambiguous, unauthorized, or over-limit requests before creating an operation.
- [x] 4.4 Update the Asset Info UI to consume backend composition eligibility and operation state without locally deciding source validity, role, language compatibility, or publication authority; verify focused frontend contract tests cover disabled reasons, explicit conflict presentation, and no client-side candidate invention.

## 5. Existing subtitle operation and publication lifecycle

- [x] 5.1 Add the required `compose` kind to the existing subtitle operation registry with an immutable request snapshot, bounded preparation/normalization/reconciliation/style/validation/publication progress, and existing status visibility; verify Asset Info operation snapshots expose frozen contributors, revisions, policies, and provenance without full cue bodies.
- [x] 5.2 Reuse existing cancellation, retry, cleanup, and stale-source checks for composition; verify stale one-of-many source revisions, frozen-input retry, terminal-operation independence, source preservation, and retry/idempotency tests.
- [x] 5.3 Stage the canonical ASS and required attachment members as one publication unit under `subtitlePublicationMu` and the existing publication-committed barrier; verify cancel/publication race tests prove no partial publication, no destructive cleanup after commit, and stable terminal outcome.
- [x] 5.4 Verify publication path/identity collisions, existing exact composite reuse, preferred/default restrictions, and native source-sidecar preservation with real temporary filesystem paths and disposable fixtures.

## 6. Focused integration and repository validation

- [x] 6.1 Add end-to-end focused backend coverage spanning Track Profile resolution, Queue snapshot freeze, Asset Info eligibility/request, composition execution, canonical ASS validation, bounded provenance, and publication; verify all listed composition scenarios execute without real NAS, production media, or network dependencies.
- [x] 6.2 Run the focused subtitle/parser/reconciliation/lifecycle tests, then run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED`; document any known frontend lint baseline separately from composition-specific results.
