## 1. Runtime capability and language contract

- [ ] 1.1 Add a runtime Tesseract language-capability probe for exact effective OCR identifiers, ensure the deployed image data exposes `jpn` and `jpn_vert`, and reject missing or unusable languages before SeConv with effective-language diagnostics.
- [ ] 1.2 Add structured OCR outcome classification for language-unavailable, genuine-zero-recognition, and tool/input/validation failures; verify that strict zero-recognition matching does not mask malformed input or generic tool failures.

## 2. Canonical Japanese recognition fallback

- [ ] 2.1 Thread the probed capability set and effective-language metadata through the canonical PGS/VobSub OCR path without changing non-Japanese language normalization or OCR modes; verify requested, effective, attempted, and successful language diagnostics.
- [ ] 2.2 Implement one bounded `jpn` ↔ `jpn_vert` fallback only after genuine zero recognition and only when the alternate language is available, reusing the extracted PGS/bitmap input and operation-owned workspace; verify extraction and OCR invocation counts.
- [ ] 2.3 Return a concise actionable failure when both available orientations produce genuine zero recognition, and preserve existing failure and cancellation semantics; verify missing-language, generic-failure, cancellation, and neither-orientation cases.

## 3. Canonical output fan-out

- [ ] 3.1 Ensure one successful Japanese recognition result feeds SRT, ASS, and logical `both` serializers and records the fallback language without duplicate serializer-triggered OCR; verify artifact contents and command counts.
- [ ] 3.2 Preserve existing sidecar identity, staging/publication, OCR cleanup modes, and non-Japanese behavior; verify the existing canonical subtitle and OCR regressions remain green.

## 4. Runtime/image and validation

- [ ] 4.1 Update runtime dependency and traineddata validation for supported architectures, and verify the built/runtime Tesseract lists the exact exposed languages and SeConv accepts those identifiers.
- [ ] 4.2 Add focused backend regression coverage using disposable ffprobe/ffmpeg/SeConv/Tesseract fixtures for PGS/VobSub language preflight, diagnostics, zero-recognition distinction, orientation fallback and reuse, neither-orientation failure, and SRT/ASS/`both` single-pass fan-out.
- [ ] 4.3 Run `./scripts/verify.sh --auto` and verify that validation reports `ALL CHECKS PASSED`.
