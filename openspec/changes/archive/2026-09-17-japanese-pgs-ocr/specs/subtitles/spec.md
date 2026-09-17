## ADDED Requirements

### Requirement: Runtime Japanese OCR capability and bounded orientation fallback

For a canonical converted PGS or VobSub subtitle request, the system SHALL
resolve the effective OCR language and verify that the running OCR runtime
actually supports that language before starting recognition. The runtime image
SHALL provide the Japanese traineddata required by the Japanese options it
exposes, but the runtime capability result SHALL remain authoritative. A
missing or unusable language/runtime capability SHALL be reported separately
from a completed OCR attempt that recognized zero subtitle text, and
diagnostics SHALL include the effective OCR language used or rejected.

When a SeConv recognition process exits, the system SHALL inspect bounded
SeConv stdout/stderr and exit diagnostics before classifying the result. A
non-zero exit SHALL become `zero_recognition` only when the strict known
canonical SeConv zero-recognition signal is present and no cancellation,
malformed-input, crash, or unrelated-failure diagnostic takes precedence. A
non-zero exit without that strict signal SHALL remain a tool/input failure.

When the effective language is `jpn` or `jpn_vert` and the complete language
attempt ends with the canonical genuine zero-recognition result, the system
MAY perform at most one bounded fallback as a distinct opposite Japanese
language/orientation attempt, only when that fallback language is available in
the runtime. The fallback budget SHALL count distinct language/orientation
attempts, not raw SeConv process invocations. Existing raw, clean, and
accurate OCR-mode semantics SHALL remain authoritative inside each language
attempt, including any subprocesses required by that mode. The fallback SHALL
reuse the already extracted bitmap/PGS input and the existing OCR workspace,
SHALL remain within Japanese language data, and SHALL NOT run for missing
language support, tool failure, malformed input, cancellation, or other
non-zero-recognition failures. If either orientation recognizes text, that
successful canonical recognition result SHALL feed SRT, ASS, or logical `both`
serialization without causing another OCR pass. Serializers SHALL never start
or retry OCR. If neither available orientation recognizes text, the operation
SHALL return a concise actionable recognition failure that identifies the
attempted Japanese language or languages and distinguishes it from missing
runtime support. OCR behavior for non-Japanese languages SHALL remain
unchanged.

#### Scenario: Supported effective Japanese language passes runtime preflight

- **WHEN** a PGS or VobSub conversion resolves effective OCR language `jpn` or
  `jpn_vert` and the running Tesseract capability report contains that exact
  language
- **THEN** OCR starts with the effective language, diagnostics identify it, and
  the canonical subtitle pipeline continues normally

#### Scenario: Missing effective Japanese language fails before OCR

- **WHEN** a Japanese conversion resolves an effective language that the
  running Tesseract capability report does not contain
- **THEN** the operation fails before SeConv/Tesseract recognition starts with
  an actionable missing-language/runtime diagnostic naming the effective
  language

#### Scenario: Runtime support is not inferred from package intent

- **WHEN** the container or host configuration suggests a Japanese package is
  installed but the running Tesseract capability report does not list the
  requested language
- **THEN** the capability report controls the result and the operation reports
  missing runtime support rather than attempting OCR with an unusable language

#### Scenario: Genuine Japanese zero-recognition is distinct from missing support

- **WHEN** the effective Japanese language passes preflight, OCR runs against
  the extracted bitmap input, SeConv emits the strict known
  zero-recognition signal in bounded diagnostics, and the process may exit
  non-zero without a cancellation, malformed-input, crash, or unrelated
  failure diagnostic
- **THEN** the operation reports a zero-recognition diagnostic that names the
  effective language and does not report the result as missing language data

#### Scenario: Non-zero SeConv exit without the strict signal remains a failure

- **WHEN** SeConv exits non-zero without the strict known zero-recognition
  signal, or its bounded diagnostics identify cancellation, malformed input, a
  crash, or another unrelated failure
- **THEN** the operation reports the existing tool/input or cancellation
  classification, does not relabel it as zero recognition, and does not start
  a Japanese orientation fallback

#### Scenario: Horizontal Japanese falls back once to vertical Japanese

- **WHEN** `jpn` is available and produces the canonical genuine
  zero-recognition result while `jpn_vert` is also available
- **THEN** the operation performs at most one distinct `jpn_vert`
  language/orientation attempt using the already extracted bitmap/PGS input;
  any raw/clean/accurate subprocesses within that attempt do not consume extra
  fallback budget, and a successful fallback result becomes the canonical
  recognized cue result

#### Scenario: Vertical Japanese falls back once to horizontal Japanese

- **WHEN** `jpn_vert` is available and produces the canonical genuine
  zero-recognition result while `jpn` is also available
- **THEN** the operation performs at most one distinct `jpn`
  language/orientation attempt using the already extracted bitmap/PGS input;
  any raw/clean/accurate subprocesses within that attempt do not consume extra
  fallback budget, and a successful fallback result becomes the canonical
  recognized cue result

#### Scenario: Japanese fallback is not attempted for unavailable alternate data

- **WHEN** the primary Japanese language is available but recognition returns
  genuine zero recognition and the alternate Japanese orientation is absent
  from the runtime capability report
- **THEN** no fallback OCR is started and the operation returns a concise
  zero-recognition diagnostic that identifies the unavailable alternate

#### Scenario: Japanese fallback is not a generic OCR retry

- **WHEN** OCR fails because the selected language is unavailable, a child tool
  exits with an error without the strict zero-recognition signal, the bitmap
  input is malformed, work is cancelled, or OCR reports a result other than
  genuine zero recognition
- **THEN** the operation does not invoke the opposite Japanese orientation as a
  fallback and preserves the existing failure classification

#### Scenario: One successful Japanese recognition result feeds all requested formats

- **WHEN** Japanese PGS or VobSub OCR succeeds with the primary language or its
  one allowed orientation fallback and the request is SRT, ASS, or logical
  `both`
- **THEN** the canonical recognized cues produce the requested artifacts, a
  logical `both` request shares one recognition result, and no serializer
  starts or retries a duplicate OCR pass

#### Scenario: Neither Japanese orientation recognizes text

- **WHEN** both available Japanese orientations complete with genuine
  zero-recognition results
- **THEN** the operation fails with a concise actionable message that names the
  effective/attempted Japanese languages and advises checking subtitle
  orientation or source content, without presenting the failure as missing
  runtime language support

#### Scenario: Non-Japanese OCR semantics remain unchanged

- **WHEN** a PGS or VobSub conversion resolves a non-Japanese OCR language
- **THEN** the existing canonical OCR language selection, recognition, cleanup,
  failure classification, and SRT/ASS output behavior remain unchanged and no
  Japanese orientation fallback is attempted
