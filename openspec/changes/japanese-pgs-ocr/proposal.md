## Why

Japanese PGS OCR can reach SeConv/Tesseract and still fail with zero
recognized text, while the current runtime and diagnostics do not establish
whether the selected Japanese language data is usable or whether recognition
genuinely found nothing. This makes horizontal/vertical Japanese failures
ambiguous and prevents a safe, bounded orientation retry.

## What Changes

- Preflight the effective OCR language against the languages actually reported
  by the running Tesseract binary before starting Japanese OCR, while keeping
  the runtime image's exposed Japanese options aligned with installed
  traineddata.
- Classify unavailable language/runtime support separately from a completed
  OCR attempt that produced zero recognized cues, and include the effective OCR
  language in diagnostics.
- For Japanese only, allow one bounded `jpn` ↔ `jpn_vert` fallback after a
  genuine zero-recognition result when the alternate language is available;
  reuse the already extracted PGS/bitmap input and existing OCR workspace.
- Feed the first successful canonical Japanese recognition result to the
  existing SRT, ASS, or logical `both` serializers without triggering another
  OCR pass.
- Report a concise actionable failure when neither available Japanese
  orientation recognizes text, while preserving existing behavior for all
  other languages and OCR cases.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Define runtime OCR-language preflight, Japanese orientation
  fallback, diagnostic classification, and single-result serializer reuse for
  canonical PGS/VobSub conversion.

## Impact

- Backend canonical bitmap OCR orchestration, SeConv/Tesseract command
  execution, zero-recognition classification, diagnostics, and SRT/ASS fan-out.
- Backend image dependency/traineddata configuration and runtime capability
  probing for `jpn` and `jpn_vert`.
- Focused unit and integration-style tests using disposable command fixtures;
  no real media, OCR host dependency, hardware, or NAS mount is required.
- No database or Queue migration. Existing canonical subtitle planning,
  artifact identity, sidecar naming/discovery, OCR modes, and non-Japanese OCR
  semantics remain unchanged.
- Out of scope: arbitrary-language fallback, retrying a failed OCR attempt as
  a generic recovery mechanism, new OCR engines, visual subtitle styling
  reconstruction, new sidecar discovery/matching, or a parallel subtitle
  pipeline.
