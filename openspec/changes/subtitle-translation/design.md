## Context

The repository already has one canonical subtitle planner and cue pipeline.
Asset Info subtitle extraction uses the existing subtitle operation identity,
operation context, progress model, staging workspace, revision checks, and
atomic publication barrier. Text subtitles are parsed and serialized through
the canonical cue path; PGS and VobSub use the canonical OCR lifecycle, which
already includes Japanese orientation fallback and OCR quality/correction. The
archived OCR lifecycle and quality-repair changes make canonical cues
serializer-neutral before SRT/ASS serialization and keep OCR workspaces,
reusable evidence, and subscriber operation staging separate.

This change adds translation after canonical source cues exist. It must not
make the provider responsible for subtitle structure or create another
operation/job path.

## Goals / Non-Goals

**Goals:**

- Add one explicit Asset Info translation operation for English/Japanese
  embedded and eligible external subtitle sources.
- Translate bounded canonical cue text to `es-419` or `es-ES` through a
  configurable Ollama provider.
- Preserve cue identity, timing, ordering, semantic metadata, and ASS/SSA
  structure while reusing canonical serializers and publication safety.
- Reuse canonical OCR, correction, cancellation, retry, source revision,
  staging, and atomic publication contracts.
- Keep provider traffic, context, audit data, and temporary work NAS-safe and
  bounded.

**Non-Goals:**

- Automatic translation in Queue, Track Profiles, Profile Lab, or workers.
- A second subtitle planner, OCR pipeline, operation/job system, or publisher.
- Any provider implementation other than Ollama in V1.
- Translation of arbitrary source languages, translation as OCR repair, or
  changes to playback/client policy.

## Decisions

### 1. Attach translation to the existing Asset Info subtitle operation

Asset Info starts translation through the existing subtitle operation model.
The operation is identified, persisted/polled, cancelled, cleaned, retried,
and published using the same lifecycle as subtitle extraction and OCR quality
operations. A translation-specific operation kind and request payload may be
added to the existing model/API, but it is not a second job system.

The operation stages translated output in an operation-owned workspace and
uses the existing publication barrier. Its phases reuse the existing progress
shape: preparing source/revision, preparing canonical cues, translating
bounded batches, validating, and publishing. `processed` and `total` count
cue batches or cues consistently for the operation. Cancellation propagates
through the existing context to provider HTTP requests, OCR, child processes,
validation, and staging cleanup. Retry creates a new operation identity.

An Asset Info control is shown only for an exact eligible embedded stream or
external subtitle entry. External entries carry the existing ownership/path
identity; translation must not locate a source using language alone.

The `Translate...` dialog is driven by a backend capability response for that
exact subtitle entry. It exposes source language (`Auto`, `English`, or
`Japanese`), target locale (`Spanish (Latin America) / es-419` or `Spanish
(Spain) / es-ES`), and the configured provider. It exposes a model control
only when the selected provider reports model selection as supported. The
response also carries informational media-context status and source, which the
dialog displays when available; missing context never disables translation.

The backend returns the canonical valid output-format set for the selected
source. The frontend renders an output-format selector only when that set has
more than one member. With exactly one valid format, the dialog presents no
choice and submits that backend-authorized format. The frontend never derives,
adds, or filters output eligibility from source filenames, client capability,
or provider state.

**Alternative rejected:** a separate translation queue or synchronous HTTP
request. Either would split cancellation/progress/retry semantics from the
existing subtitle operation contract and make atomic publication harder to
prove.

### 2. Use a serializer-neutral canonical translation envelope

The translation seam consumes an internal immutable envelope derived from the
existing canonical subtitle cues:

```text
TranslationCue {
  cueID
  sourceOrdinal
  start/end
  sourceText
  representation metadata
  protected structure reference
  geometry/layout reference when present
}
```

`cueID` is stable for the operation and includes enough source/artifact
identity to prevent cross-stream collisions. `sourceOrdinal`, timing, and
structure are owned by MVForge and are not provider-editable. The envelope is
an operation-local view of the existing canonical cue types, not a second
subtitle model persisted beside them.

The provider receives only `cueID`, source text, source language, target
locale, and bounded context. The validated response returns `cueID` plus
translated text. MVForge merges text back into the canonical cue set and
passes that set to the existing serializer.

For SRT, only cue text changes; SRT remains text/timing-only. For ASS/SSA,
parsing separates dialogue text from immutable event fields, override tags,
styles, drawing/karaoke tokens, escapes, and geometry/layout metadata. Only
eligible human dialogue text is sent to the provider. MVForge reconstructs the
styled representation from its protected structure and canonical cues. A
source-format-preserving output is used for text sidecars: SRT remains SRT,
ASS remains ASS, and SSA remains SSA where the existing serializer supports
that representation. Bitmap translation uses the existing canonical output
representation selected by Asset Info; when no text representation exists,
the default translated sidecar is canonical SRT, with any explicitly selected
ASS output using the existing geometry-aware serializer.

**Alternative rejected:** sending complete ASS/SSA lines to Ollama. This
would allow a provider to rewrite tags, styles, timing, or drawing commands
and would make structural validation depend on model behavior.

### 3. Resolve source language before provider work

The request stores requested source language (`auto`, `eng`, or `jpn`) and
effective source language. Explicit English/Japanese is authoritative for V1
when the selected source is eligible. Auto uses bounded reliable evidence
already available for the selected stream/artifact: trusted stream metadata
and bounded cue-language evidence for text, or the effective configured
language used by the canonical OCR path for bitmap sources.

Auto proceeds only when the evidence is confidently English or Japanese. It
fails before provider work for ambiguity, unsupported evidence, or an
unavailable effective OCR language. The operation does not run OCR merely to
guess a text sidecar's language and does not perform broad media analysis.
Missing title or media metadata is independent of this decision and does not
block translation.

### 4. Define a replaceable provider boundary with Ollama as V1

The provider boundary is a small text-only adapter conceptually equivalent to:

```text
Translate(ctx, TranslationBatch) -> TranslationBatchResult
```

The request contains a bounded list of cue IDs/texts, source language, target
locale, and optional bounded context. The result contains a structured list of
cue IDs and translated text plus bounded provider metrics. The adapter never
receives media paths, bitmap images, ASS structural tokens, credentials, or
unbounded prompts.

The Ollama adapter uses the persisted endpoint and model settings and sends a
structured JSON contract over the configured HTTP endpoint. Endpoint URLs are
validated and may point to a NAS, LAN service, or container name; no implicit
localhost endpoint is selected. Request timeouts and body sizes are bounded,
the operation context is attached to every request, and HTTP/status/JSON
errors are classified as provider failures. CPU-only Ollama is a supported
runtime; no GPU capability check is required.

The provider contract has an explicit version and leaves room for future
adapters selected by configuration. V1 accepts only Ollama, so a future
adapter cannot silently be selected by an unknown provider value.

**Alternative rejected:** embedding Ollama calls in Asset Info or the
serializer. That would prevent provider replacement, make cancellation
inconsistent, and risk a separate request for each output format.

### 5. Batch cues and provide advisory context

Canonical cues are partitioned into bounded batches by cue count and encoded
request size. Neighboring cues are selected from the same canonical sequence
within a bounded window and are marked as context rather than translation
targets. Optional metadata is reduced to bounded fields: title/year,
movie/series name, season/episode, synopsis, and known character names.

The provider receives only target cues plus the permitted context. Context is
never used to change cue boundaries or source-language classification. The
absence of metadata produces an empty context section, not a failed operation.
Batch results are accumulated by cue ID; batching never changes canonical
order or the operation's output identity.

### 6. Validate before serializer fan-out

Validation is a distinct boundary after every provider batch and again over
the complete translated canonical cue set. It requires exactly one result for
each requested ID, rejects missing/duplicate/unexpected IDs, rejects provider
fields other than text, and confirms that cue count, IDs, order, timing,
protected ASS/SSA structure, geometry, semantic role/disposition, and source
artifact identity are unchanged.

Empty text is rejected when the source cue is structurally non-empty unless
the existing canonical subtitle validation explicitly permits that cue. A
failed batch fails the whole requested translation; no partial set is
serializable or publishable.

After validation, canonical serializers produce the requested translated
artifact representation. SRT ignores geometry. ASS/SSA consume the same
translated canonical cue text while preserving the protected structure. No
serializer can invoke OCR or the provider.

### 7. Reuse bitmap OCR and OCR quality/correction as a strict predecessor

For PGS/VobSub, the operation calls the existing canonical bitmap OCR entry
point with the operation context and selected OCR language/mode. Existing
Japanese `jpn`/`jpn_vert` fallback, accurate candidate selection, geometry
correlation, and #39 deterministic/selective correction complete before any
translation batch is built. The selected corrected canonical cue set is the
only translation input.

Translation does not persist or inspect bitmap images, create a prepared
input, call SeConv/Tesseract, or rerun OCR for another serializer. If OCR or
correction fails or is cancelled, translation stops and the source and prior
translated artifacts remain unchanged.

### 8. Define identity, locale naming, and source revision protection

The translation identity is a canonical serialized tuple containing:

```text
translationContractVersion
source artifact/stream identity
source revision/fingerprint
canonical source representation/provenance
requested/effective source language
target locale
provider identity and provider contract/model version
context contract/version and mode
requested output representation
```

The source revision uses the existing bounded source/artifact revision
conventions: embedded streams bind to the source snapshot/stream identity;
external subtitles bind to exact owned artifact path, size, mtime, and full
content hash. The full media file is not hashed as a normal lookup path when a
bounded subtitle/artifact representation is available.

The artifact identity extends existing subtitle naming/ownership with a
stable locale qualifier. `es-419` and `es-ES` are never aliases. The source
artifact is never overwritten, and an existing translated artifact is
considered reusable only when its complete identity and source revision match.

The operation rechecks every source member immediately before the publication
barrier. A mismatch fails closed and cleans only operation-owned staging. A
previous translated artifact is preserved, even if it was generated from an
older revision.

### 9. Preserve atomic publication, cancellation, and retry semantics

All translated outputs are staged and validated before the existing atomic
publication barrier. If an operation requests multiple representations through
the existing canonical output contract, they form one publication set; one
failure or cancellation publishes none of the new members. Existing source and
translated sidecars are outside the current operation's cleanup ownership.

The operation's cancelled work context stops provider/OCR/child processes.
Cleanup uses the existing independent bounded cleanup path, then reports
`cancelled` distinctly from provider/validation failure. A race after the
complete publication barrier follows the existing completion-wins boundary.
Retry starts a new operation identity and re-evaluates the exact source
artifact/revision; it never transitions the old terminal operation back to
running.

### 10. Keep audit data bounded and secret-safe

On terminal completion, bounded internal provenance records provider and model
identity/version, requested/effective source language, target locale, context
source/mode, source revision, operation identity, translation contract
version, validation result, and available request/token/latency metrics.
It does not store endpoint credentials, authorization headers, full prompts,
full provider responses, or duplicated full cue payloads in artifact metadata
or UI responses. Translation ownership/provenance is internal to the managed
artifact and follows existing artifact removal/cleanup ownership; it does not
create media-adjacent audit files.

### 11. Settings, deployment, and compatibility

Provider settings use the existing persisted `AppSetting` path with a
translation-specific value object containing provider, endpoint, model, and
bounded request/context policy. Missing configuration is disabled/unavailable
and produces an actionable Asset Info error; it does not default to localhost
or perform a network call. Existing settings and sidecars require no data
migration.

The feature is additive and can be disabled by leaving the provider
configuration unavailable. Existing extraction, OCR, quality repair, naming,
semantic-role/disposition handling, and publication continue unchanged for
operations that do not explicitly request translation.

## Risks / Trade-offs

- **Provider quality or hallucinated structure** → send text-only bounded
  batches and reject every response that fails cue-ID or structural
  validation; never let model output own subtitle structure.
- **Ollama latency on CPU-only NAS hardware** → bounded batches, progress
  reporting, operation cancellation, configurable endpoint/model, and no
  per-cue requests.
- **Context can expose media metadata to the provider** → context is optional,
  explicitly bounded, advisory, and absent by default when unavailable; audit
  data excludes prompts and responses.
- **Source changes during a long translation** → capture and recheck bounded
  source revisions immediately before publication; fail closed and preserve
  previous artifacts.
- **ASS/SSA syntax is heterogeneous** → use existing parsers/serializers and
  a protected structural representation; unsupported or unsafe structures fail
  validation rather than being flattened silently.
- **Bitmap OCR and translation could duplicate expensive work** → translation
  consumes the existing canonical OCR/correction result and never owns OCR or
  image preparation.
- **A crashed operation can leave staging** → use the existing operation-owned
  cleanup and bounded orphan cleanup; do not delete source or published
  sidecars as incidental cleanup.

## Migration Plan

1. Add persisted provider settings with missing-value-safe parsing and no
   implicit endpoint, then add the provider contract and bounded Ollama adapter.
2. Add the operation-local canonical translation envelope, ASS/SSA protection,
   structured response validation, identity, revision, provenance, and
   serializer integration.
3. Route embedded text, external SRT/ASS, and bitmap OCR results through the
   existing Asset Info subtitle operation, staging, cancellation, retry, and
   publication barrier.
4. Add Asset Info controls and progress/retry presentation only after the
   backend contract is available; do not add Queue/Profile behavior.
5. Roll back by disabling the translation setting/entry point. Existing source
   media and sidecars remain untouched; incomplete staging is removed by the
   existing cleanup path and translation provenance is ignored by older
   subtitle flows.

There are no unresolved architectural questions for V1; future provider
adapters and richer language evidence can be added behind the defined
provider/canonical-cue contracts.
