## Context

See `proposal.md` for motivation and scope. The existing canonical OCR path is
centered in `subtitle_ocr.go`; its Hunspell-backed `ocrKnownWords` helper scores
words but does not store editable phrase rules. `subtitle_ocr_quality.go`
already owns deterministic suggestions, canonical cue analysis, correction
operations, published revision checks, and staged correction publication.
`publishedCanonicalOCRSnapshot` retains the lightweight canonical cue baseline,
and `prepareCanonicalOCREvidenceForTranslation` prepares corrected canonical
cues before Translation. The main `subtitles` spec already requires canonical
identity, bounded published snapshots/history, correction provenance, safe
SRT/ASS serialization, and atomic publication.

The current `normalizedOCRLanguage` helper maps unknown values to `eng` for the
existing OCR workflow. Dictionary eligibility must therefore be stricter than
that general fallback: English rules apply only when the evidence explicitly
resolves to the canonical English language. This change does not alter the
global OCR language resolver. Older published snapshots may contain only the
post-fallback `eng` value, so that value alone cannot establish English rule
eligibility for legacy re-evaluation.

## Goals / Non-Goals

**Goals:**

- Persist a bounded, queryable dictionary in the existing SQLite/GORM data
  lifecycle and expose a backend-authoritative CRUD and preview contract.
- Add deterministic, literal contextual matching and route its candidates
  through existing OCR analysis, correction, Translation preparation, cue
  serializers, and publication safeguards.
- Re-evaluate saved canonical OCR with a preview and revision-checked apply,
  without image extraction or full PGS/VobSub OCR.
- Deliver a reusable dictionary component that issue #266 can place in
  Settings.

**Non-Goals:**

- A new OCR analyzer, correction engine, serializer, operation/job lifecycle,
  translation pipeline, or settings store.
- Arbitrary regular-expression rules, global character substitutions, LLM
  inference, automatic rule learning, or an OCR review/navigation redesign.
- Settings page, route, or navigation integration; issue #266 owns that work.

## Decisions

### Persist entries as SQLite rows with a deterministic effective revision

Add one dictionary-entry model/table to the existing migration path rather than
serializing the rule map into the whole-map `AppSetting` value or a standalone
JSON file. Rows support indexed language/source/enabled filtering, stable IDs,
row-level validation, and independent updates. Store the literal pattern,
replacement, match/boundary/context/case policies, priority, enabled state,
source (`builtin` or `user`), per-entry rule version, and creation/update
timestamps. Use server-generated immutable IDs. Search is bounded, ordered by
priority then stable ID, and matches the query against pattern and replacement.

Seed English built-ins idempotently by stable ID. Updating a built-in definition
requires a new built-in rule version and preserves its user-controlled enabled
state. Built-ins cannot be edited or deleted through the API; users may disable
them or create an independent user rule. Custom edits increment the entry
version. Compute a dictionary revision from the sorted effective rule IDs,
versions, enabled states, and matching fields; preview and apply use this
fingerprint to detect rule-set changes without introducing a second revision
store.

Use implementation bounds of 4,096 total entries, 256 Unicode code points per
pattern/context/replacement field, 100 entries per search page, and 4 KiB per
test-match sample. The matcher indexes rules by canonical language and literal
prefix so each OCR operation uses one immutable, bounded rule snapshot.

Alternative considered: a JSON setting would be simpler to add, but whole-map
read/modify/write makes concurrent CRUD and search harder to make reliable and
would create a second serialization contract for an editable collection.

### Model rules as literals plus explicit context, not regular expressions

Represent each rule as a literal target and replacement plus explicit
left/right context, token-boundary, and case policies. Reject a rule that can
replace `|` without a whole-token boundary and explicit contextual predicate.
Do not expose regex syntax. Built-ins use complete-token forms for `|'m`,
`|'ve`, `|'ll`, `|'d`, `| can't`, and `| don't`. The `am |` rule requires `am`
as a complete preceding word, `|` as a standalone token, and a safe right
boundary at cue end or punctuation. Each rule declares whether matching is
case-sensitive; the configured replacement is emitted literally, while all
text outside the matched span keeps its original bytes and casing.

The seeded English rules whose replacement begins with `I` also require a
conservative left context: cue start or one of the validated discourse
prefixes `Well,`, `No,`, `Yes,`, `Oh,`, or `Ah,`. Any other visible prefix,
including a grammatically ambiguous subject such as `They`, leaves the bar
unchanged for review. For `am |`, the visible suffix after the bar must be
empty after whitespace/formatting or begin with one of `. , ; : ! ? …`; a
following word is not a valid boundary.

Resolve language aliases only through the strict dictionary resolver. Apply
English rules to explicit `en`, `eng`, or `english` evidence normalized to
`eng`; capture this dictionary language and its evidence source from the raw
request or stream language tag before `normalizedOCRLanguage` applies its
fallback. Persist it separately from the effective OCR language so ordinary
OCR behavior and cache identity remain unchanged. Do not call the global
normalizer for dictionary eligibility because its unknown-language fallback
is English. The dictionary schema can carry canonical languages supported by
the OCR runtime, but English is the only seeded rule set in this change. A rule
for one language is never a fallback for another.

For a legacy published snapshot without strict language evidence, do not infer
English from its stored `RequestedLanguage=eng`. Re-evaluation accepts an
explicit supported language selection for that preview, binds it into the
preview revision, and records it in correction provenance. Without a selection
the snapshot remains analyzable, but no language-scoped dictionary rule is
applied.

Find non-overlapping candidates left-to-right after sorting by descending
priority. A unique highest-priority replacement wins an overlap. Conflicting
replacements tied at the highest priority produce an ambiguity finding and
leave the cue unchanged; identical replacement results may be coalesced while
retaining all supporting rule IDs in provenance. Keep ASS override blocks and
other structural fields outside the text matcher. An unmatched bar, a bar
inside a word/name, and a possible separator remain unchanged and subject to
the existing OCR quality gate.

Alternative considered: a global `|` to `I` substitution or free-form regex
would catch more cases but would also alter names, words, separators, or markup
without evidence. The contextual literal model makes each automatic change
inspectable and testable.

### Extend the existing deterministic correction boundary

Add the matcher to the existing deterministic candidate generation used by
`deterministicOCRSuggestion` and canonical OCR analysis. Apply only an
unambiguous match whose declared context/boundaries pass and whose result passes
the existing deterministic OCR validation. High-confidence matches may be
applied to the in-memory canonical cue set before serialization; all other
matches remain suggestions/review findings. Rule presence alone never clears
the existing safety gate. OCR review and
`prepareCanonicalOCREvidenceForTranslation` call the same matcher on the same
canonical cue text, so Translation receives one validated corrected result and
does not run its own substitutions or OCR pass.

Translation preparation automatically applies only candidates whose complete
matched rule-ID set resolves to built-in entries. User-defined rules still
produce matcher and OCR Review suggestions, and explicit acceptance continues
through the existing correction operation; Translation leaves those cues
unchanged so the canonical OCR quality gate keeps them behind review.

Extend existing correction provenance with the effective dictionary revision,
source language, rule ID/version, cue ID, correction operation identity, and
before/after cue-text hashes. Keep the canonical OCR identity, source
fingerprint, cue timing/order/geometry, and existing provenance fields as the
authority; do not store duplicate full media evidence in dictionary rows.

Alternative considered: a standalone correction package or a frontend matcher
would duplicate backend policy and could make preview, publication, and
Translation disagree. The shared backend boundary keeps one deterministic
result and leaves the frontend responsible only for editing and previewing.

### Re-evaluate with a snapshot, then apply through the existing operation

Preview reads the valid current `publishedCanonicalOCRSnapshot` or uses only
the existing bounded legacy reconstruction path. It evaluates a copy of the
saved canonical cues with a frozen enabled-rule snapshot and returns cue-level
proposals, rule provenance, and updated quality counts. It does not modify the
sidecar, baseline, or OCR execution cache and does not reacquire bitmap input.
If no valid snapshot or bounded legacy baseline is available, report that
re-evaluation is unavailable; do not silently start full OCR.

Applying selected or accepted preview candidates uses the existing subtitle
correction operation, canonical SRT/ASS serializers, final published revision
check, and `publishCorrectedSubtitleFiles` publication barrier. The request is
bound to both the analyzed sidecar revision set and effective dictionary
revision. A changed sidecar or changed enabled ruleset makes the preview stale
and requires a new preview. A logical `both` output uses one corrected cue set
and one publication unit. Extend the existing bounded published-snapshot and
provenance history with the predecessor canonical cues needed for re-analysis
or rollback; do not add public `.bak` files or a second history authority.

If validation, cancellation, revision checking, file publication, or snapshot
commit fails, restore the previous sidecars and authoritative snapshot using
the existing rollback boundary and clean only operation-owned staging. A
successful correction advances the current baseline and records the previous
and new revisions with the operation ID and rule provenance.

Alternative considered: rerunning PGS OCR could refresh evidence but costs
time/resources and can produce a different recognition result. The published
canonical baseline is already designed to support re-analysis after execution
cache eviction, so it remains the source for dictionary re-evaluation.

### Keep the component isolated for Settings integration by #266

Provide a reusable frontend component that lists/filters rules, edits only user
entries, toggles built-ins, and invokes backend validation/test preview. Keep
all matching and validation decisions in the backend. Export the component and
API types/client for the owning Settings page, but do not change
`SettingsPage`, navigation, routes, or Settings query ownership in this change.

## Risks / Trade-offs

- `[False positive on names, lyrics, or separators]` → Require explicit token
  context, keep `am |` end/punctuation guarded, leave ambiguous matches for
  review, and cover negative cases in focused tests.
- `[Unknown OCR language appears as English through the current fallback]` →
  Use strict dictionary language eligibility based on explicit evidence and
  test empty, unknown, Spanish, and Japanese language cases without changing
  global OCR normalization.
- `[Legacy snapshots do not retain language authority]` → Treat their
  normalized `eng` value as ambiguous and require an explicit language choice
  for dictionary re-evaluation; bind that choice to preview and apply.
- `[User rules conflict or shadow each other]` → Validate rule structure,
  expose priority and test-match preview, and fail closed on conflicting
  equal-priority matches.
- `[Large dictionaries increase NAS CPU or storage use]` → Enforce field,
  entry, page, and sample bounds; index by language/prefix; evaluate a frozen
  ruleset snapshot once per operation.
- `[Rule edits race with previewed publication]` → Bind previews to a
  deterministic dictionary revision and reject stale applies.
- `[Historical snapshots consume disk]` → Reuse the existing owned snapshot
  retention and cleanup policy and keep only bounded canonical cue history.
- `[SRT and ASS siblings diverge]` → Use one canonical corrected cue set,
  validate every requested representation, and retain the existing all-output
  publication/rollback barrier.

## Migration Plan

1. Add the dictionary table through the existing SQLite/GORM migration path and
   idempotently seed versioned English built-ins. Preserve existing OCR
   snapshots and sidecars; no sidecar or execution-cache migration is required.
2. Deploy the backend API and shared matcher. Existing installations with no
   dictionary rows receive only the seeded built-ins; custom rules are created
   through CRUD.
3. Deliver the isolated component and API client for #266 to embed. This change
   does not make Settings navigation depend on the component.
4. On rollback to an older application version, the additive table is ignored;
   do not delete user dictionary rows as part of application rollback.
