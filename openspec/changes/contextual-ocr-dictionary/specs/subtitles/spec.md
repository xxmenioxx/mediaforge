## ADDED Requirements

### Requirement: OCR dictionary entries are language-scoped, persistent, and manageable

The system SHALL persist bounded OCR dictionary entries with a stable identity,
canonical source language, literal pattern and replacement, explicit matching,
boundary, context, and case policies, priority, enabled state, source, rule
version, and audit metadata. Users SHALL be able to create, search/filter,
inspect, edit, enable/disable, and delete user entries, and preview a rule
against sample cue text. Search and preview results SHALL be bounded and SHALL
not change dictionary entries or subtitle artifacts. Built-in rules SHALL be
identifiable and protected from editing or deletion; users MAY enable or
disable them. English (`eng`) is the initial seeded language.

#### Scenario: User rule survives restart and can be searched
- **WHEN** a user creates a valid English rule and the service is restarted
- **THEN** the rule retains its stable identity and version and is returned by a bounded search for its language, source, enabled state, pattern, or replacement

#### Scenario: Built-in rules remain protected
- **WHEN** a user attempts to edit or delete a built-in rule
- **THEN** the mutation is rejected, while enable/disable remains available and the built-in source and version remain visible

#### Scenario: Rule preview has no side effects
- **WHEN** a user previews a rule against sample cue text
- **THEN** the system returns bounded match and replacement results or a validation reason without persisting the sample or changing a dictionary entry or subtitle artifact

### Requirement: Contextual OCR rules match only explicit language and safe text boundaries

Dictionary matching SHALL use literal patterns with explicit, validated
context, boundary, and case policies; entries SHALL NOT provide arbitrary
regular expressions. English starter rules SHALL include `|'m → I'm`,
`|'ve → I've`, `|'ll → I'll`, `|'d → I'd`, `| can't → I can't`,
`| don't → I don't`, and the guarded `am | → am I` case. A rule SHALL be
eligible only for its exact canonical source language. Unknown or unsupported
language evidence SHALL NOT receive English rules. Matching SHALL preserve all
text outside the matched span, avoid subtitle markup and structural fields,
and leave unmatched or ambiguous text unchanged. A higher-priority rule MAY
resolve an overlapping match; conflicting matches at the same highest priority
SHALL remain unchanged and be reported for review.

The seeded English `| → I` rules SHALL match only at cue start or after the
validated discourse prefixes `Well,`, `No,`, `Yes,`, `Oh,`, or `Ah,`; other
preceding text is ambiguous and remains unchanged. The seeded `am | → am I`
rule SHALL match only when the visible cue suffix after `|` is empty or begins
with a validated punctuation boundary (`.`, `,`, `;`, `:`, `!`, `?`, or `…`),
ignoring trailing whitespace and subtitle formatting.

Published snapshots created before strict dictionary-language evidence was
recorded SHALL NOT be assumed to be English solely because legacy OCR language
normalization stored `eng`. Re-evaluation of such a snapshot MAY use an
explicitly selected supported language bound to that preview and apply; without
that selection, no language-scoped dictionary rule is applied.

#### Scenario: English contractions match with declared boundaries and case policy
- **WHEN** an English cue contains a complete token matching an enabled contraction rule such as `|'m` or `| can't`
- **THEN** only that span is replaced with its declared English form, surrounding cue text and punctuation retain their original casing, and the candidate is sent through canonical OCR validation

#### Scenario: Built-in first-person rules fail closed on ambiguous grammar
- **WHEN** a seeded English `| → I` rule occurs after text other than cue start or a validated discourse prefix
- **THEN** the text remains unchanged and reviewable, while the same pattern remains available for a user rule preview and explicit correction review

#### Scenario: The `am |` rule requires a standalone OCR token
- **WHEN** an English cue has `am` as a complete word followed by a standalone `|` token with the rule's validated right boundary
- **THEN** the guarded rule may propose `am I` only at cue end or before validated terminal punctuation; a following word, embedded bar, or ambiguous separator remains unchanged and reviewable

#### Scenario: Proper names, words, markup, and separators are not globally rewritten
- **WHEN** `|` occurs inside a word or name, in ASS markup or another structural field, as a separator, or in a cue with unknown or non-English language evidence
- **THEN** English dictionary rules do not replace it, and ordinary OCR quality findings remain authoritative

#### Scenario: Conflicting equal-priority rules fail safe
- **WHEN** overlapping enabled rules match the same text with different replacements and share the highest priority
- **THEN** no replacement is auto-applied and the ambiguous match is returned for review

#### Scenario: Legacy snapshots do not inherit the English fallback
- **WHEN** a published snapshot lacks trustworthy dictionary-language evidence and its legacy OCR language is `eng`
- **THEN** English rules are not applied unless the re-evaluation request explicitly selects English, and that selection is included in the preview and correction provenance

### Requirement: Dictionary candidates reuse canonical OCR correction and Translation preparation

OCR review, canonical OCR correction, and bitmap Translation preparation SHALL
use the same deterministic contextual-rule matcher over canonical cue text.
Only unambiguous high-confidence matches that satisfy their declared
boundaries and context MAY be auto-applied. Dictionary membership alone SHALL
NOT bypass the canonical OCR safety gate; uncertain, conflicting, or unmatched
bars SHALL remain reviewable. Applied-rule provenance SHALL identify the
canonical cue and OCR language, dictionary revision, rule identity and
version, correction operation, and before/after text hashes.

Translation preparation SHALL auto-apply only validated candidates whose
matched rule identities all resolve to seeded built-in entries. User-defined
rules remain available to preview and canonical OCR Review, but their
suggestions require explicit correction acceptance and SHALL NOT auto-correct
Translation input.

#### Scenario: OCR analysis offers a contextual candidate without silently publishing it
- **WHEN** saved or newly produced canonical OCR contains a high-confidence contextual match
- **THEN** the shared analyzer returns the candidate and its rule provenance, and publication still follows the existing correction and review safeguards

#### Scenario: Translation uses the same validated correction result
- **WHEN** bitmap subtitle evidence is prepared for Translation
- **THEN** the same deterministic dictionary rules are evaluated once on canonical cues before translation, and remaining review findings prevent Translation from consuming unreviewed text

#### Scenario: Translation keeps user-defined rule suggestions behind explicit review
- **WHEN** a user-defined dictionary rule produces a validated candidate during Translation preparation
- **THEN** the suggestion remains available through canonical OCR Review, Translation does not apply it automatically, and unresolved OCR quality findings prevent consumption of that cue

#### Scenario: Applied correction records reproducible provenance
- **WHEN** a dictionary rule changes a canonical cue
- **THEN** the committed evidence records the source language, dictionary revision, rule ID/version, cue ID, operation identity, and hashes of the cue text before and after correction

### Requirement: Existing canonical OCR can be re-evaluated and previewed before publication

The system SHALL re-evaluate a saved, valid published canonical OCR snapshot
with the current enabled dictionary without extracting bitmap images or
rerunning PGS/VobSub OCR. Re-evaluation SHALL return proposed cue changes and
the resulting clean, suspicious, and review counts without changing published
SRT/ASS bytes or snapshot state. Applying previewed changes SHALL be bound to
the analyzed artifact revision and dictionary revision, use the existing
canonical correction, serializer, validation, and publication lifecycle, and
advance the canonical baseline with bounded predecessor provenance/history.
SRT and ASS outputs for one logical transformation SHALL publish as one unit
from the same corrected canonical cues. Cancellation, validation failure,
revision conflict, or publication failure SHALL preserve the prior sidecars
and authoritative snapshot; internal history SHALL not create Library-visible
backup files.

#### Scenario: Re-evaluation uses saved evidence without PGS OCR
- **WHEN** a user requests re-evaluation for an artifact with a valid canonical OCR snapshot
- **THEN** the system evaluates its saved canonical cues with the current enabled rules and does not re-extract subtitle images or invoke PGS/VobSub OCR

#### Scenario: Legacy language choice is bound to re-evaluation
- **WHEN** a legacy snapshot lacks strict dictionary-language evidence and the user explicitly selects a supported language for preview
- **THEN** that language is used only for the previewed rule evaluation and apply rejects the preview if its language selection changes

#### Scenario: Preview reports changes and remaining review work
- **WHEN** re-evaluation finds dictionary candidates
- **THEN** the response shows proposed cue-level text changes and updated clean, suspicious, and review counts while leaving published files and snapshots unchanged

#### Scenario: Stale preview cannot overwrite changed rules or sidecars
- **WHEN** the sidecar revision or dictionary revision changes after preview and before apply
- **THEN** apply fails as stale, publishes nothing, preserves the current sidecars and snapshot, and requires a fresh preview

#### Scenario: SRT and ASS integrity survives rule application
- **WHEN** dictionary corrections are applied to an SRT, ASS, or logical `both` artifact
- **THEN** cue identity, order, timing, and all non-text canonical metadata remain unchanged; ASS styles, override tags, geometry, and positioning are retained; SRT and ASS serialize the same corrected cue text

#### Scenario: Cancellation or publication failure leaves no partial output
- **WHEN** correction is cancelled or any requested serializer, revision check, or publication step fails
- **THEN** no member of the new output set remains published, the prior authoritative baseline is restored, and operation-owned staging is cleaned
