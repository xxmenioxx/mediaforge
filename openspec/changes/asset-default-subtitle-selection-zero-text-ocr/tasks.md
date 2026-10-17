## 1. Preserve resolved subtitle selection

- [x] 1.1 Restrict Video Profile externalization to a frozen Track plan or explicit stream-index selection; verify inherited, explicit-empty, and selected-subset regressions in `media_command_test.go`.
- [x] 1.2 Verify that excluded streams and unrequested SRT/ASS representations do not create subtitle artifact work or alter FFmpeg mappings in focused handler tests.

## 2. Classify bitmap OCR outcomes

- [x] 2.1 Extend bounded PGS SUP parsing with event/object/cue counts; verify valid zero-cue, cue-present, and malformed/unknown fixtures.
- [x] 2.2 Stop before OCR for proven zero-cue PGS inputs and return a required-artifact failure; verify fake Tesseract/SeConv tools are not invoked and no sidecar is created.
- [x] 2.3 Preserve zero-recognition stream, language, mode, available count, and unknown-evidence diagnostics; verify this stays distinct from missing language/runtime/tool failures and does not publish an empty artifact.

## 3. Validate the scoped change

- [x] 3.1 Run focused subtitle and Queue planning tests and validate the OpenSpec change with strict validation.
- [x] 3.2 Run `./scripts/verify.sh --auto` through the serialized shared-validation lock and confirm `ALL CHECKS PASSED`.
