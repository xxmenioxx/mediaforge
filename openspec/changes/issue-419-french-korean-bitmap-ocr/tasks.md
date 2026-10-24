# Tasks

- [x] Add French and Korean traineddata to the supported backend image and
  fail the build if any required canonical language is absent.
- [x] Add repeatable container smoke and regression tests for exact runtime
  capabilities and fail-closed missing-language behavior.
- [x] Add a decodable synthetic French/Korean PGS fixture and run real
  SeConv/Tesseract recognition in the supported image. The backend E2E validates
  SRT/ASS sidecars, cue timing, selected stream identity, and bitmap geometry;
  OCR assertions require recognized text without claiming perfect Korean accuracy.
- [x] Rebuild and smoke the version-pinned image, then run focused tests, strict
  OpenSpec, and the canonical verifier.
