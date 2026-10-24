# Tasks

- [x] Add French and Korean traineddata to the supported backend image and
  fail the build if any required canonical language is absent.
- [x] Add repeatable container smoke and regression tests for exact runtime
  capabilities and fail-closed missing-language behavior.
- [ ] Add a decodable synthetic PGS/VobSub fixture and run SeConv recognition;
  the current repository has no such fixture/generator, and this is explicitly
  BLOCKED rather than substituted with direct Tesseract model initialization.
- [x] Rebuild and smoke the version-pinned image, then run focused tests, strict
  OpenSpec, and the canonical verifier.
