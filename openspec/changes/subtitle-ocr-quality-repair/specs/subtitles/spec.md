## ADDED Requirements

### Requirement: Published OCR analysis baseline is separate from execution cache

After successful OCR sidecar publication, the system SHALL retain a bounded
internal canonical cue snapshot associated with the published artifact. The
snapshot SHALL include canonical cue IDs, text, timing/order, geometry/layout
needed for ASS, OCR language/mode, a canonical OCR identity/provenance
reference, artifact or logical-transform identity, and versions/integrity
metadata. It SHALL NOT retain large prepared bitmap inputs or arbitrary per-cue
images indefinitely. The existing #6 reusable OCR execution cache SHALL remain
bounded and evictable, including its current seven-day and 256 MiB limits.

#### Scenario: Analysis works after execution-cache eviction

- **WHEN** a successfully published OCR artifact's #6 execution-cache entry
  has expired or been evicted but its internal canonical cue snapshot remains
- **THEN** `Analyze OCR` analyzes the snapshot without requiring a full OCR
  rerun and without exposing an evidence file beside the media

#### Scenario: Snapshot retains serializer-neutral baseline data

- **WHEN** a published OCR artifact is later analyzed or corrected
- **THEN** the internal snapshot supplies cue identity, text, timing/order,
  required ASS geometry/layout, OCR and provenance identity, and integrity
  metadata without retaining the cleaned shared-flight bitmap workspace

#### Scenario: Legacy artifact reconstructs only bounded text evidence

- **WHEN** an older OCR artifact has no snapshot but an MVForge-owned SRT/ASS
  and sufficient provenance can be read
- **THEN** the system may reconstruct a bounded text-only analysis snapshot;
  image-level/selective OCR remains unavailable unless valid cue input is
  safely reacquired, and Analyze OCR does not silently rerun full OCR

#### Scenario: Missing legacy provenance fails closed

- **WHEN** a legacy OCR artifact has neither a valid snapshot nor sufficient
  provenance for a bounded reconstruction
- **THEN** analysis reports the unavailable baseline and leaves the published
  sidecar unchanged rather than guessing, scanning the complete media, or
  starting full OCR automatically

### Requirement: OCR quality analysis uses canonical evidence

For an OCR-generated subtitle artifact, the system SHALL analyze the canonical
OCR evidence or the lightweight published canonical cue snapshot produced by
the existing subtitle pipeline.
Analysis SHALL be independent of the requested final sidecar format, SHALL
not start a full OCR lifecycle a second time, and SHALL not modify or publish
any sidecar. The result SHALL include aggregate clean, suspicious, and review
counts plus bounded per-cue findings containing the cue identity, original
text, finding status, evidence, and any candidate suggestion.

#### Scenario: Analyze generated OCR without changing the sidecar

- **WHEN** Asset Info requests `Analyze OCR` for a generated SRT, ASS, or
  logical `both` artifact
- **THEN** the system analyzes the existing canonical OCR evidence, returns
  quality counts and per-cue findings, and leaves the currently published
  sidecar bytes and metadata unchanged

#### Scenario: Analysis reuses canonical evidence

- **WHEN** equivalent SRT, ASS, or `both` requests refer to the same canonical
  OCR evidence
- **THEN** quality analysis consumes that shared evidence without running full
  OCR again or creating a format-specific OCR result

#### Scenario: Analysis reports bounded cue findings

- **WHEN** analysis completes for a subtitle stream
- **THEN** every reported finding identifies its canonical cue, original text,
  status, bounded reason/evidence, and optional suggestion, and the aggregate
  counts equal the per-cue statuses

### Requirement: Quality analysis fails safely and scores candidates

The quality analyzer SHALL use deterministic bounded evidence, including
structural character sanity, replacement or suspicious-character penalties,
known OCR confusion patterns, supported-language lexical or dictionary checks,
unknown-word evidence, and available recognition evidence. The canonical
`accurate` candidate scorer SHALL use a versioned deterministic contract that
selects its candidate before canonical cleanup and serialization. Dictionary
membership SHALL not be authoritative; unsupported or unknown languages SHALL
not receive English dictionary semantics. Proper nouns, unknown words, and
conflicting evidence SHALL remain clean or reviewable rather than being
silently rewritten. Analysis SHALL never treat uncertainty as permission to
change canonical cue text.

#### Scenario: Suspicious OCR confusion is identified

- **WHEN** a canonical cue contains a bounded OCR-confusion pattern supported
  by lexical or recognition evidence
- **THEN** analysis marks the cue suspicious or reviewable, reports the reason,
  and may provide a bounded candidate without changing the cue

#### Scenario: Accurate scoring changes a candidate when evidence supports it

- **WHEN** isolated and color-preserving accurate-mode OCR candidates include
  one valid-looking recognition error and one candidate supported by
  deterministic structural, confusion, and supported-language lexical
  evidence
- **THEN** the versioned accurate scorer may select the supported candidate
  before cleanup and serialization, while a case without sufficient evidence
  remains reviewable

#### Scenario: Accurate scoring provenance invalidates incompatible evidence

- **WHEN** the accurate candidate-scoring contract/version changes
- **THEN** cached OCR evidence produced under the older scorer is not reused as
  an equivalent accurate result and fresh compatible selection evidence is
  required

#### Scenario: Proper nouns and unknown words fail safe

- **WHEN** a token is a proper noun, an unknown word, or lacks reliable
  dictionary/context evidence
- **THEN** the analyzer does not auto-correct it solely because it is unknown
  and reports clean or review status with bounded evidence

#### Scenario: Analysis is deterministic for unchanged evidence

- **WHEN** the same canonical evidence, analyzer version, and bounded runtime
  inputs are analyzed again
- **THEN** the findings, statuses, counts, and deterministic candidate scores
  are equivalent

#### Scenario: Unsupported language does not use English dictionary semantics

- **WHEN** a cue's OCR language is unsupported or unknown to the lexical
  resources
- **THEN** scoring uses only applicable structural/confusion evidence and
  fails safe without treating dictionary absence as an OCR error

### Requirement: OCR repair is selective, provenance-aware, and not translation

OCR repair SHALL start from canonical OCR evidence and SHALL avoid rerunning
full OCR. Selective secondary OCR SHALL be available only when MVForge can
obtain the suspicious cue bitmap/input through a bounded supported mechanism,
such as a prepared subtitle representation, persisted lightweight source/cue
locator, or another explicit safe extraction path. For eligible bitmap cues,
the system MAY use bounded DVD/VobSub OCR-only preprocessing and selective
secondary OCR. Candidate comparison SHALL include the original text and the
provenance of each generated suggestion. An optional local or contextual
correction provider SHALL be subordinate to deterministic validation, SHALL be
unable to modify structural cue data, and SHALL not translate subtitles.

#### Scenario: Selective secondary OCR repairs an eligible suspicious bitmap cue

- **WHEN** a suspicious DVD/VobSub cue has cue input available through a
  bounded supported representation or locator and evidence supports a
  secondary OCR attempt
- **THEN** repair may preprocess only the bounded cue input, compare the
  resulting candidate with canonical evidence, and apply it only after
  deterministic validation accepts it

#### Scenario: Unavailable selective cue input remains reviewable

- **WHEN** a suspicious cue lacks a safely reacquirable bounded bitmap/input
- **THEN** the canonical text remains unchanged and the cue is marked for
  review without selective OCR

#### Scenario: Selective OCR does not trigger broad OCR

- **WHEN** selective cue input is unavailable or bounded extraction fails
- **THEN** repair leaves the cue unchanged or marks it for review without
  rerunning full-stream OCR or performing unbounded media analysis

#### Scenario: Optional contextual correction remains subordinate

- **WHEN** an optional local or contextual provider returns a text suggestion
- **THEN** the suggestion is treated as a candidate, is accepted only through
  the same deterministic validation and review rules, cannot alter timing,
  cue structure, or layout metadata, and is not used as a translation result

#### Scenario: Repair records provenance

- **WHEN** a candidate is kept, applied, or rejected
- **THEN** internal provenance records the canonical evidence identity, cue
  identity, analyzer/repair contract versions, candidate source, relevant OCR
  language/mode and runtime evidence, validation outcome, and whether a user
  or provider selected it

### Requirement: Corrected canonical cues feed every requested serializer

The correction boundary SHALL be:

`canonical OCR evidence -> quality/correction -> canonical final cues -> SRT/ASS serializers`

Corrected canonical cues SHALL preserve cue IDs, timing, order, ASS override
  tags, styles, bitmap geometry, and positioning metadata. Correction MAY
  change cue text and correction metadata only. SRT SHALL serialize the
  corrected text and timing without geometry; ASS SHALL serialize the same
  corrected text with retained styles, geometry, and positioning. A logical
  `both` request SHALL use one corrected canonical cue set for both outputs.

#### Scenario: Text-only correction preserves cue structure

- **WHEN** a user or validated candidate corrects canonical cue text
- **THEN** cue IDs, timing, order, ASS tags/styles, geometry, and positioning
  are unchanged from the canonical evidence

#### Scenario: SRT and ASS share corrected text

- **WHEN** corrected SRT and ASS outputs are requested for equivalent canonical
  OCR input
- **THEN** both serializers consume the same corrected cue text, while SRT
  omits geometry and ASS retains the canonical layout metadata

#### Scenario: Logical both does not duplicate correction

- **WHEN** a logical `both` conversion is repaired and published
- **THEN** quality/correction runs once for the logical transformation and
  both final sidecars are rendered from the resulting canonical cues

#### Scenario: Native ASS/SSA remains outside OCR repair

- **WHEN** an original native ASS or SSA extraction is requested
- **THEN** the existing lossless native extraction behavior remains in force
  and OCR quality repair does not normalize or rewrite that native artifact

### Requirement: Asset Info review and correction are safe to publish

Asset Info SHALL expose `Analyze OCR` results with clean, suspicious, and
review counts and per-cue original-versus-suggestion details. Users SHALL be
able to keep a cue, apply a validated suggestion, or enter a manual text edit;
safe batch application SHALL apply only candidates meeting the deterministic
validation policy. Analysis SHALL be read-only. Correction SHALL use the
existing subtitle operation identity, status, progress, cancellation, and
retry model rather than introducing a second job system.

#### Scenario: Review shows a cue-level decision

- **WHEN** analysis returns a suspicious or reviewable cue
- **THEN** Asset Info shows the original text, any suggestion, bounded reason
  and provenance, and actions to keep, apply, or edit only the cue text

#### Scenario: Safe batch repair applies validated findings only

- **WHEN** a user requests a batch repair
- **THEN** only findings that pass deterministic validation are applied, while
  uncertain or conflicting findings remain unchanged and reviewable

#### Scenario: Failed correction preserves the published sidecar

- **WHEN** correction fails validation, encounters an execution failure, or is
  cancelled before publication
- **THEN** the currently published SRT/ASS sidecar remains untouched, all
  operation-owned staging is cleaned, and the result is distinct from a
  successful correction

#### Scenario: Publication is atomic for requested outputs

- **WHEN** a correction requests SRT, ASS, or logical `both`
- **THEN** every requested output is validated before the existing publication
  barrier commits, and no output from the current attempt is published alone

#### Scenario: No temporary repair artifacts are library-visible

- **WHEN** analysis or correction creates intermediate evidence or staged
  outputs
- **THEN** temporary files, `.bak`, and `.raw` artifacts remain internal and
  are cleaned or retained only in the internal provenance/evidence store; none
  appears beside the media as a Library-visible sidecar

#### Scenario: A cancelled repair can be retried

- **WHEN** a correction operation ends as cancelled or failed and the user
  retries it
- **THEN** the prior operation remains terminal, a new operation identity is
  used, existing published sidecars are preserved, and valid canonical OCR
  evidence may be reused without violating the existing cancellation rules

### Requirement: Analysis and correction are bound to the published revision

Quality findings SHALL record a bounded published-artifact revision for every
SRT or ASS sidecar they describe. The revision SHALL include artifact/path
identity, size, mtime, full sidecar content hash, and logical `both` membership
where applicable. Before applying an individual, manual, or batch correction
and again immediately before publication, the system SHALL verify that the
currently published output set still matches the recorded revision. A mismatch
SHALL fail closed as stale analysis, require re-analysis, clean operation
staging, and preserve the current sidecar.

#### Scenario: External sidecar edit invalidates analysis

- **WHEN** a published SRT or ASS sidecar changes after analysis but before a
  correction is applied
- **THEN** the correction is rejected as stale, the changed sidecar is not
  overwritten, operation staging is cleaned, and the user must analyze again

#### Scenario: Logical both checks every published member

- **WHEN** analysis targets a logical `both` artifact and either the published
  SRT or ASS member no longer matches its recorded revision
- **THEN** the complete correction is rejected before publication and neither
  output from that correction is committed

#### Scenario: Concurrent corrections cannot overwrite each other

- **WHEN** operation A and operation B both analyze revision R, A publishes a
  valid correction, and B reaches its final publication check afterward
- **THEN** B observes that revision R is stale, fails closed, cleans its
  staging, and does not overwrite A's published result

### Requirement: Contextual correction providers are bounded and subordinate

If a contextual correction provider is enabled, the system SHALL send only
suspicious or reviewable candidates, SHALL batch them within bounded request
limits rather than issuing one request per cue, and SHALL propagate the
existing subtitle operation cancellation context. Provider output SHALL remain
text-only and SHALL pass deterministic validation before application. Provider
output SHALL never change structural subtitle data or perform translation.

#### Scenario: Provider receives bounded suspicious batches

- **WHEN** a correction operation has suspicious or reviewable cues and a
  contextual provider is enabled
- **THEN** only those candidates are sent in bounded batches, clean cues are
  excluded, and each suggestion retains provider provenance for review

#### Scenario: Provider cancellation leaves canonical text unchanged

- **WHEN** the subtitle operation is cancelled or the provider times out while
  a contextual request is in progress
- **THEN** the operation cancellation context stops the provider work, no
  provider suggestion is applied, and the existing sidecar remains unchanged

#### Scenario: Provider suggestions remain text-only

- **WHEN** a provider returns a correction suggestion
- **THEN** deterministic validation may accept only its cue text, while cue IDs,
  timing, order, tags, styles, geometry, and positioning remain canonical and
  no translation is performed

### Requirement: Quality repair is bounded and backward compatible

Quality analysis and repair SHALL use bounded reads and bounded candidate work
appropriate for NAS-hosted media. The #6 OCR execution/reuse cache SHALL retain
its seven-day and 256 MiB bounds and SHALL be treated as evictable; the
lightweight published canonical cue snapshot is the later analysis baseline.
Cache lookup and provenance SHALL be based on the existing canonical OCR
evidence identity and, only where available, bounded prepared-input provenance,
not a whole-media fingerprint as the normal path. Temporary workspaces SHALL
be cleaned after terminal completion, failure, or cancellation. Subtitle
conversion behavior for non-OCR text, native ASS/SSA extraction, existing
Japanese OCR fallback, bitmap positioning, and existing publication semantics
SHALL remain unchanged when quality repair is not explicitly requested.

#### Scenario: Large NAS media is not fully hashed for analysis

- **WHEN** analysis or selective repair looks up reusable OCR evidence for a
  large asset
- **THEN** it uses existing bounded source/prepared-input identity and does not
  hash or scan the complete media file solely to perform quality analysis

#### Scenario: Terminal cleanup removes temporary repair work

- **WHEN** analysis or correction completes, fails, or is cancelled
- **THEN** its temporary workspace is cleaned using the existing bounded
  cleanup path, while committed internal evidence and provenance remain
  available according to their retention policy

#### Scenario: Existing behavior is preserved without repair

- **WHEN** a subtitle conversion does not request OCR quality analysis or
  repair
- **THEN** the existing canonical subtitle/OCR lifecycle, cancellation,
  Japanese fallback, geometry/ASS positioning, native ASS/SSA extraction, and
  artifact publication behavior are unchanged
