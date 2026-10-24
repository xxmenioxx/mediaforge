## ADDED Requirements

### Requirement: French and Korean bitmap OCR runtime capability

The supported MVForge backend runtime image SHALL include Tesseract traineddata
for the canonical French (`fra`) and Korean (`kor`) OCR languages. It SHALL
continue to include English (`eng`), Spanish (`spa`), Japanese (`jpn`), and
vertical Japanese (`jpn_vert`) data. The image build SHALL verify that the
running Tesseract capability list contains all six canonical codes.

For PGS or VobSub conversion, the existing canonical OCR language resolver and
running-runtime capability check SHALL remain authoritative. A resolved `fra`
or `kor` language that is absent or unusable in the running Tesseract runtime
SHALL fail before recognition with the existing actionable missing-language
diagnostic; it SHALL NOT silently substitute another language. Adding traineddata
SHALL NOT change stream identity, cue timing or geometry, or canonical SRT/ASS
sidecar behavior.

#### Scenario: Supported runtime exposes all retained and added OCR languages

- **WHEN** the supported backend image is built
- **THEN** `tesseract --list-langs` in that image lists `eng`, `spa`, `jpn`,
  `jpn_vert`, `fra`, and `kor`, and the build fails if any is absent

#### Scenario: Canonical French and Korean language resolution reaches OCR preflight

- **WHEN** a bitmap subtitle request resolves an existing French alias to
  `fra` or Korean alias to `kor`
- **THEN** the existing running-runtime availability check receives that
  canonical code before recognition and keeps the source stream identity and
  requested SRT/ASS output path unchanged

#### Scenario: Missing French or Korean runtime data remains fail-closed

- **WHEN** the running Tesseract capability report omits the resolved `fra` or
  `kor` language
- **THEN** OCR is rejected before recognition with the existing actionable
  missing-language diagnostic and no alternate language is substituted

#### Scenario: French and Korean recognition models initialize in isolation

- **WHEN** the container smoke invokes `fra` and `kor` against a generated
  blank PGM input
- **THEN** both Tesseract model invocations complete successfully without
  reading real media or claiming OCR accuracy
