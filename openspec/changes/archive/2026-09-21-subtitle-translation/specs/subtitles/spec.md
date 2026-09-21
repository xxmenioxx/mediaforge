## ADDED Requirements

### Requirement: Explicit Asset Info subtitle translation

Asset Info SHALL expose an explicit `Translate...` operation for an eligible
embedded text subtitle, embedded PGS/VobSub subtitle, or supported external
SRT/ASS subtitle artifact. V1 SHALL support source language `Auto`, `English`,
or `Japanese`, target locale `es-419` or `es-ES`, and provider `Ollama`.
Translation SHALL be opt-in from Asset Info and SHALL NOT be introduced into
Queue, Track Profiles, Profile Lab, or automatic processing policy.

#### Scenario: Embedded English text subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded English text subtitle
  with target `es-419` and an enabled, ready Ollama configuration
- **THEN** one subtitle translation operation is created and the resulting
  Spanish artifact is produced without changing the embedded source or its
  source classification

#### Scenario: Embedded Japanese bitmap subtitle can be translated

- **WHEN** Asset Info starts translation for an embedded Japanese PGS or
  VobSub subtitle with target `es-ES` and an enabled, ready Ollama
  configuration
- **THEN** the existing canonical OCR path supplies the source cues to one
  translation operation and a separate Spanish artifact is planned

#### Scenario: External sidecar can be translated

- **WHEN** Asset Info selects an eligible external SRT or ASS artifact and
  starts translation
- **THEN** the operation uses that exact artifact and its ownership/revision
  identity rather than guessing another same-language sidecar

#### Scenario: Translation is not automatic Queue behavior

- **WHEN** a Queue job, Track Profile, or Profile Lab request is processed
  without an explicit Asset Info translation operation
- **THEN** no translation provider is invoked and existing subtitle planning
  and publication behavior remains unchanged

### Requirement: Asset Info translation controls reflect backend capabilities

The Asset Info `Translate...` dialog SHALL expose source language (`Auto`,
`English`, `Japanese`), target locale (`Spanish (Latin America) / es-419`,
`Spanish (Spain) / es-ES`), and provider. It SHALL expose a model control only
when the selected provider reports model selection as supported. The backend
response SHALL provide informational media-context status and source, and the
dialog SHALL display them when available. Missing context SHALL remain
informational and SHALL NOT block translation.

The backend SHALL remain authoritative for canonical valid output formats.
The frontend SHALL show an output-format selector only when the backend
reports more than one valid format; with one valid format it SHALL use that
format without requiring a choice. The frontend SHALL NOT invent, infer, add,
or suppress output-format eligibility independently. For an intrinsically
eligible subtitle, the backend SHALL keep `Translate...` discoverable when
translation infrastructure is unavailable, but SHALL report it disabled with
an actionable reason. The frontend SHALL not probe Ollama or infer provider
readiness from endpoint/model fields.

#### Scenario: Dialog displays available media context

- **WHEN** the selected subtitle has backend-reported media context with a
  status and source
- **THEN** the dialog displays that context status and source as informational
  fields alongside the source language, target locale, provider, and supported
  model controls

#### Scenario: Missing media context does not block translation

- **WHEN** the backend reports that media context is unavailable or empty for
  an otherwise eligible subtitle
- **THEN** the dialog indicates that context is unavailable, leaves the
  translation action enabled, and does not invent replacement context

#### Scenario: Multiple backend-authorized formats show a selector

- **WHEN** the backend reports more than one canonical valid output format for
  the exact subtitle entry
- **THEN** the dialog shows an output-format selector containing only those
  backend-reported formats

#### Scenario: One backend-authorized format needs no selector

- **WHEN** the backend reports exactly one canonical valid output format
- **THEN** the dialog does not require an output-format choice and submits the
  single backend-authorized format

#### Scenario: Frontend does not decide output eligibility

- **WHEN** a source, filename, client capability, or provider state could
  suggest an output format that is absent from the backend capability response
- **THEN** the dialog does not add or enable that format and relies on the
  backend-authorized set

#### Scenario: Disabled Translate remains discoverable

- **WHEN** a subtitle is intrinsically eligible but translation is disabled,
  unconfigured, untested, unavailable, or missing its configured model
- **THEN** Asset Info shows `Translate...` as disabled with the backend's
  bounded actionable reason, and the frontend does not probe or enable the
  provider itself

### Requirement: Translation provider readiness and explicit enablement

Translation SHALL be disabled by default. Configuration, readiness, and
enablement SHALL remain distinct backend-authoritative states. A configured
provider SHALL not become usable merely because its endpoint and model are
non-empty. The backend SHALL provide an explicit `Test Connection` operation
for the configured provider. For Ollama, readiness SHALL verify both
reachability of the configured endpoint and availability of the configured
model through a provider-native lightweight readiness/model request, without
generating a subtitle translation or requiring GPU capability. The frontend
SHALL not probe the provider or infer readiness.

The backend SHALL support bounded states equivalent to `disabled`,
`not_configured`, `untested`, `ready`, `unavailable`, `model_not_found`, and
`invalid_configuration`, with secret-safe diagnostics. A readiness timestamp,
if exposed, SHALL be informational and SHALL NOT be treated as a live health
guarantee. Explicit enablement SHALL require successful readiness for the
current configuration. Changing provider, endpoint, or model SHALL invalidate
readiness and enablement.

#### Scenario: Translation disabled by default

- **WHEN** translation settings are absent or newly initialized
- **THEN** translation is disabled and Asset Info cannot start provider work

#### Scenario: Configured but untested provider

- **WHEN** an Ollama endpoint and model are configured but have not passed
  readiness testing
- **THEN** translation remains disabled and backend capabilities report that
  provider verification is required

#### Scenario: Successful Ollama readiness

- **WHEN** `Test Connection` reaches the configured Ollama endpoint and
  confirms that the configured model is available
- **THEN** readiness becomes `ready`, no subtitle translation is generated,
  and translation remains disabled until explicitly enabled

#### Scenario: Missing model

- **WHEN** the configured Ollama endpoint is reachable but the configured
  model is unavailable
- **THEN** readiness reports a bounded actionable `model_not_found` state and
  translation cannot be enabled

#### Scenario: Enable after readiness

- **WHEN** the current provider configuration passes readiness testing and the
  user explicitly enables translation
- **THEN** backend capabilities may expose translation as available for an
  otherwise eligible subtitle

#### Scenario: Configuration change invalidates readiness

- **WHEN** provider, endpoint, or model changes after successful verification
- **THEN** the prior readiness and enablement are invalidated and translation
  cannot run until the new configuration is tested successfully and enabled

#### Scenario: Runtime outage does not trigger a preflight loop

- **WHEN** an enabled provider becomes unavailable after successful setup
- **THEN** a translation attempt calls the normal provider request directly,
  uses the existing timeout/unavailable/malformed/cancellation semantics, and
  does not run an additional health or model preflight first

### Requirement: Source-language selection is reliable and bounded

The operation SHALL accept `Auto`, `English`, or `Japanese` as source
language. Auto SHALL use reliable bounded source metadata or cue evidence and
SHALL proceed only when English or Japanese is identified with sufficient
confidence. Ambiguous, unsupported, or insufficient evidence SHALL require an
explicit source language and SHALL not invoke a provider. Missing media
context SHALL not prevent translation when source language is otherwise valid.

#### Scenario: Auto selects reliable English

- **WHEN** Auto language evidence reliably identifies English
- **THEN** the operation records effective source language English and sends
  English cues to the provider

#### Scenario: Auto selects reliable Japanese

- **WHEN** Auto language evidence reliably identifies Japanese
- **THEN** the operation records effective source language Japanese and sends
  Japanese cues to the provider

#### Scenario: Auto rejects ambiguity

- **WHEN** Auto evidence cannot distinguish English from Japanese or is not
  reliable for the selected subtitle
- **THEN** the operation fails before provider work with an actionable request
  for explicit source language selection

#### Scenario: Explicit source language overrides weak evidence

- **WHEN** the user explicitly selects English or Japanese and bounded source
  metadata is incomplete but the selected source is supported
- **THEN** translation may proceed using the explicit language and records the
  requested and effective source language

### Requirement: Translation operates on canonical cues

The translation boundary SHALL be:

```text
source -> canonical cues -> optional bounded context -> TranslationProvider
       -> structured cue-ID response -> strict validation
       -> canonical final cues -> existing serializer -> staging -> publication
```

Providers SHALL receive and return text only. MVForge SHALL retain ownership
of cue IDs, order, timing, semantic metadata, ASS/SSA structure, styles,
override tags, geometry, positioning, artifact identity, validation, and
publication. Translation SHALL never create a second subtitle planner,
serializer, OCR lifecycle, operation system, or publication path.

#### Scenario: SRT translation preserves canonical timing

- **WHEN** an SRT source is translated successfully
- **THEN** the translated SRT contains provider-approved text with the same
  cue IDs, order, start/end timing, and cue boundaries as the source

#### Scenario: ASS translation preserves immutable structure

- **WHEN** an ASS source is translated successfully
- **THEN** the translated ASS uses the translated dialogue text while
  preserving styles, layer, margins, positioning, override tags, line breaks,
  karaoke/drawing structure, cue order, and timing from MVForge's canonical
  representation

#### Scenario: SSA translation preserves source representation

- **WHEN** an embedded native SSA source is translated through the existing
  text subtitle path
- **THEN** the translated artifact uses the existing SSA-aware serializer and
  preserves the source representation's structural fields while changing only
  eligible dialogue text

#### Scenario: ASS/SSA protected anchors make one provider result reversible

- **WHEN** an ASS or SSA dialogue cue contains override blocks, positioning,
  karaoke, drawing data, or `\\N`/`\\n`/`\\h` escapes
- **THEN** the provider-facing cue text contains only eligible human dialogue
  plus bounded MVForge-owned opaque anchors, and exact-once ordered anchor
  validation restores the retained structure before serialization

#### Scenario: Invalid protected anchors fail closed

- **WHEN** a provider removes, duplicates, reorders, malforms, or invents a
  protected ASS/SSA anchor
- **THEN** MVForge rejects the response before reconstruction and publishes no
  translated artifact

#### Scenario: Provider text cannot inject ASS/SSA structure

- **WHEN** provider-owned translated text outside the exact expected anchors
  contains override braces, `\\N`, `\\n`, `\\h`, or physical CR/LF line breaks
- **THEN** MVForge rejects the response before reconstruction while allowing
  ordinary translated punctuation such as commas

#### Scenario: Providers cannot change subtitle structure

- **WHEN** a provider response attempts to add markup, alter timing, merge or
  split cues, reorder cues, or change ASS/SSA structural tokens
- **THEN** validation rejects the response and no translated artifact is
  published

### Requirement: Structured provider responses are strictly validated

Each translation batch SHALL carry stable canonical cue IDs and the provider
response SHALL contain exactly one text result for each requested cue ID.
Missing, duplicate, unexpected, empty-invalid, or structurally mutated cue
results SHALL fail the operation. A provider response SHALL not be accepted by
position alone when cue IDs are available.

#### Scenario: Valid cue-ID response is accepted

- **WHEN** a provider returns exactly the requested cue IDs with text-only
  translations
- **THEN** MVForge attaches each translation to its matching canonical cue and
  continues to serializer validation

#### Scenario: Missing cue ID fails closed

- **WHEN** a provider omits one requested cue ID
- **THEN** the batch fails validation, the source remains unchanged, and no
  partial translated sidecar is published

#### Scenario: Duplicate or unexpected cue ID fails closed

- **WHEN** a provider returns a duplicate or unknown cue ID
- **THEN** the response is rejected as malformed and no output from that
  operation is published

#### Scenario: Provider structural mutation fails closed

- **WHEN** provider output includes timing, ordering, ASS tags, style fields,
  geometry, or other fields outside translated text
- **THEN** the operation rejects the response and preserves existing source and
  translated artifacts

### Requirement: Ollama is the bounded V1 translation provider

The V1 provider SHALL be Ollama with a configurable endpoint and model. The
endpoint SHALL be usable over a NAS, LAN, or container network and SHALL NOT
assume localhost. CPU-only Ollama deployments SHALL be supported. Requests
SHALL contain bounded batches of cues and SHALL propagate the existing subtitle
operation context. The provider contract SHALL be replaceable by future
OpenAI, DeepL, Google, or Azure adapters without changing canonical cue or
operation contracts; those adapters are not part of V1.

#### Scenario: Configured remote Ollama is used

- **WHEN** translation is enabled after the configured Ollama endpoint and
  model have passed readiness testing
- **THEN** the operation sends bounded structured batches to that endpoint and
  records the configured provider/model identity

#### Scenario: Runtime Ollama outage fails clearly

- **WHEN** an enabled translation request reaches an endpoint that is missing,
  invalid, or unreachable
- **THEN** the operation ends with an actionable provider failure before
  publication and does not silently use localhost or another provider

#### Scenario: Translation is batched

- **WHEN** a subtitle contains more cues than one bounded provider request can
  carry
- **THEN** the operation sends multiple bounded batches, preserves global cue
  identity/order, and never issues one provider request per cue

#### Scenario: Provider cancellation propagates

- **WHEN** the user cancels while an Ollama request is in progress
- **THEN** the request stops through the existing operation context, the
  operation reaches `cancelled` after cleanup, and no translated output is
  published

### Requirement: Translation context is bounded and advisory

The operation MAY provide bounded neighboring canonical cues and optional
media metadata such as title/year, movie or series name, season/episode,
synopsis, and known character names. Context SHALL be advisory only; it SHALL
not authorize structural subtitle changes, replace source-language evidence,
or block translation when absent. Context sent to a provider SHALL be bounded
and SHALL not be persisted as a full prompt or full response.

#### Scenario: Neighboring cues provide bounded context

- **WHEN** context is enabled for a cue batch
- **THEN** only the configured bounded neighboring cues and permitted metadata
  are sent, and cue ownership remains based on explicit IDs

#### Scenario: Missing metadata does not block translation

- **WHEN** title, synopsis, character, or episode metadata is unavailable
- **THEN** translation proceeds without that context when the source and
  provider contracts are otherwise valid

#### Scenario: Context cannot alter structure

- **WHEN** provider output influenced by context proposes a timing, tag, style,
  geometry, or cue-boundary change
- **THEN** canonical validation rejects the structural change and preserves the
  source representation

### Requirement: Bitmap translation reuses canonical OCR and correction

PGS and VobSub translation SHALL first use the existing canonical bitmap OCR
and OCR-quality/correction lifecycle. Japanese `jpn`/`jpn_vert` fallback and
accurate candidate selection SHALL occur once at the canonical OCR boundary.
Translation SHALL consume the resulting canonical corrected cues and SHALL
never invoke OCR itself or cause serializers to invoke OCR again.

#### Scenario: PGS translation reuses one OCR result

- **WHEN** a PGS subtitle is translated
- **THEN** canonical OCR produces the source cues once, correction is applied
  before translation, and the provider receives only canonical cue text

#### Scenario: VobSub translation reuses one OCR result

- **WHEN** a VobSub subtitle is translated
- **THEN** the existing VobSub OCR path supplies canonical cues to translation
  without a translation-specific OCR or bitmap extraction path

#### Scenario: Japanese fallback precedes translation

- **WHEN** Japanese OCR requires its existing bounded opposite-orientation
  fallback
- **THEN** the successful canonical language attempt becomes the source for
  translation and the provider is called only after OCR/correction completes

#### Scenario: OCR failure prevents translation publication

- **WHEN** OCR, OCR correction, or required canonical cue validation fails
- **THEN** the provider is not invoked and no translated artifact is staged as
  publishable

### Requirement: Locale-specific translated artifact identity and source safety

Translation SHALL publish a separate artifact and SHALL never overwrite the
source sidecar by default. `es-419` and `es-ES` SHALL have distinct canonical
artifact and logical-transform identities. Artifact naming SHALL reuse existing
subtitle naming and ownership helpers while adding a deterministic locale
qualifier. An existing translated artifact MAY satisfy only the exact source
identity, source revision, target locale, provider contract, and requested
representation; it SHALL not satisfy another locale or stale source.

#### Scenario: Target locales remain distinct

- **WHEN** the same source is translated once to `es-419` and once to `es-ES`
- **THEN** two distinct artifact identities and locale-qualified sidecars are
  available without either operation overwriting the other

#### Scenario: Source sidecar is preserved

- **WHEN** translation succeeds for an SRT, ASS, or SSA source
- **THEN** the source artifact remains byte-for-byte untouched and the Spanish
  artifact is published separately

#### Scenario: Existing exact translation is reusable

- **WHEN** an existing translated artifact matches the exact canonical source
  identity, revision, locale, provider contract, and representation
- **THEN** the operation may report it satisfied without duplicating publication

### Requirement: Translation is protected by source revision and atomic publication

The operation SHALL capture the selected embedded stream or external artifact
revision before provider work and SHALL recheck that revision immediately
before publication. A stale or replaced source SHALL fail closed, preserve the
source and any previously published translated artifact, and publish nothing
new. All requested translated outputs SHALL pass canonical validation before
the existing all-output publication barrier commits them.

#### Scenario: External source edit invalidates translation

- **WHEN** an external SRT or ASS source changes after translation starts
  before publication
- **THEN** the operation reports stale source, cleans its staging, publishes
  nothing new, and leaves the changed source and prior translated artifact
  intact

#### Scenario: Embedded source replacement invalidates translation

- **WHEN** the media source or selected subtitle stream snapshot changes before
  publication
- **THEN** the operation fails the final revision check and does not publish a
  translation for the old source revision

#### Scenario: Atomic publication prevents partial translated output

- **WHEN** one requested translated representation fails validation or
  publication
- **THEN** no newly generated member of that operation is published and
  previously existing artifacts remain intact

### Requirement: Translation uses the existing operation lifecycle

Translation SHALL use the existing subtitle operation identity, status,
progress, cancellation, cleanup, retry, staging, and publication contracts.
Progress SHALL expose meaningful preparation, batching/translation,
validation, and publication phases with bounded processed/total values where
the existing operation API supports them. A terminal `cancelled` or failed
translation SHALL remain terminal; retry SHALL create a new operation identity.

#### Scenario: Translation reports progress

- **WHEN** batches are prepared, translated, and validated
- **THEN** Asset Info can display the existing operation progress and current
  phase without creating a second job model

#### Scenario: Cancelled translation preserves all prior artifacts

- **WHEN** a translation is cancelled before the publication barrier
- **THEN** provider work and child processes stop, operation-owned staging is
  cleaned, no new artifact is published, and existing source/translated
  artifacts remain unchanged

#### Scenario: Failed translation can be retried

- **WHEN** translation ends as failed or cancelled
- **THEN** the terminal operation remains unchanged and a retry may create a new
  operation identity without overwriting existing artifacts

### Requirement: Translation provenance is bounded and secret-safe

Successful or terminal translation provenance SHALL record bounded metadata for
provider, model/version, requested and effective source language, target
locale, context source, input revision, operation identity, contract version,
and available token/request metrics. It MUST NOT persist secrets, credentials,
authorization headers, full prompts, full provider responses, or unbounded
translated payloads in artifact metadata or the Asset Info UI.

#### Scenario: Audit metadata identifies a translation

- **WHEN** a translation operation completes or fails after provider contact
- **THEN** bounded provenance identifies its provider/model, languages, locale,
  input revision, context mode, operation, and contract version

#### Scenario: Secrets and full provider content are excluded

- **WHEN** provider configuration or responses contain credentials, prompts,
  or large payloads
- **THEN** those values are excluded or redacted from persisted provenance,
  sidecar metadata, logs intended for Asset Info, and UI responses

#### Scenario: Existing non-translation provenance remains readable

- **WHEN** an existing subtitle artifact has no translation provenance
- **THEN** it remains readable and publishable through the existing subtitle
  pipeline without migration or being mislabeled as translated
