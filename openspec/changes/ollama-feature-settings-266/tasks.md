## 1. Provider settings

- [x] 1.1 Reuse backend connection, installed-model, readiness, and runtime-status APIs and verify they are shown through one provider surface.
- [x] 1.2 Show installed, resident, ready, unavailable, and disconnected states without auto-selecting or changing a feature model; cover the states in frontend tests.

## 2. Independent feature configuration

- [x] 2.1 Preserve Translation and OCR AI values while adding Copilot enabled/model/preference settings with a disabled default; verify normalization, migration, and independent persistence in backend tests.
- [x] 2.2 Add separate Translation, OCR AI, and Copilot selectors that permit the same model in more than one feature; verify persistence and defaults in frontend tests.
- [x] 2.3 Keep save boundaries and drafts scoped by Settings domain; verify navigation, provider refresh, and unrelated Jellyfin draft preservation.

## 3. Settings organization and safety

- [x] 3.1 Keep subtitle compatibility/styling and OCR Dictionary access in the Subtitles domain without duplicate cards; verify section routing and rendering.
- [x] 3.2 Verify saving or selecting a model does not trigger inference, model download, or implicit feature enablement.

## 4. Validation

- [x] 4.1 Run focused backend and frontend Settings tests, including the isolated Ollama API module.
- [x] 4.2 Run OpenSpec strict and `./scripts/verify.sh --auto` under the shared-validation lock after the final file-ownership refactor.
