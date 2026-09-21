## Why

MVForge can currently extract, OCR, normalize, and publish subtitle sidecars,
but Asset Info cannot produce a Spanish sidecar from an English or Japanese
subtitle while preserving the canonical cue structure and publication safety
already used by the subtitle pipeline. A bounded, explicit translation
operation is needed for users who want Spanish subtitles without changing the
source sidecar or introducing a second subtitle job system.

## What Changes

- Add an explicit Asset Info `Translate...` operation for embedded text,
  embedded PGS/VobSub, and eligible external SRT/ASS subtitle artifacts.
- Resolve source language as Auto, English, or Japanese and target locale as
  `es-419` or `es-ES`; Auto requires reliable English/Japanese evidence and
  otherwise requires an explicit source language.
- Add a configurable Ollama provider contract with endpoint and model
  settings, LAN/NAS/container compatibility, CPU-only support, bounded cue
  batching, cancellation, and redacted diagnostics.
- Keep translation disabled by default and require an explicit backend
  readiness test plus enablement before Asset Info can start provider work;
  configuration, readiness, and enablement remain distinct.
- Translate canonical cue text only, validate structured cue-ID responses, and
  serialize the validated canonical translated cues through existing SRT and
  ASS/SSA-aware paths.
- Reuse the existing PGS/VobSub OCR, Japanese orientation fallback, accurate
  selection, and OCR quality/correction lifecycle before translation.
- Publish a separate locale-qualified translated sidecar through existing
  staging, revision, validation, and atomic publication boundaries.
- Record bounded provider, model/version, language, locale, context, source
  revision, operation, and available request-metric provenance without storing
  secrets, credentials, full prompts, or provider responses.

Explicitly out of scope for V1:

- Automatic Queue, Track Profile, Profile Lab, or batch translation policy.
- OpenAI, DeepL, Google, Azure, or any provider implementation other than
  Ollama.
- Translation of arbitrary languages, language guessing without reliable
  evidence, or translation as part of OCR quality repair.
- Changes to the canonical subtitle planner, OCR lifecycle, operation model,
  semantic-role classification, ASS geometry/positioning, or publication
  mechanism.

## Capabilities

### New Capabilities

None. Translation extends the existing canonical subtitle capability.

### Modified Capabilities

- `subtitles`: add explicit Asset Info translation, provider, canonical cue,
  validation, provenance, revision, cancellation, and locale-specific
  publication requirements.

## Impact

- Backend: subtitle operation planning/execution, canonical cue translation
  seam, provider settings/readiness and Ollama adapter, Asset Info translation
  API/capabilities, and bounded audit/provenance persistence.
- Frontend: Asset Info subtitle actions, source/target/provider controls,
  progress/cancellation/retry presentation, and translated artifact status.
- Storage/publication: separate `es-419` and `es-ES` artifact identities,
  operation staging, source revision checks, and existing atomic sidecar
  publication.
- Runtime: configurable Ollama endpoint/model reachable from a NAS, LAN, or
  container; no localhost assumption and no required GPU.
