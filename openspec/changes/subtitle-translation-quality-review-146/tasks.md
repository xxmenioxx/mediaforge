## 1. Deterministic Analysis

- [x] 1.1 Add pure bounded cue-quality checks for empty/shortened output, numeric preservation, and duplicate unrelated translations; verify with deterministic fixtures.
- [x] 1.2 Integrate analysis after canonical structural validation and expose bounded operation evidence; verify malformed structure still fails before quality findings and publication.
- [x] 1.3 Verify canonical OCR uncertainty is surfaced separately, gates translation before provider work, and is never treated as a translation correction or quality approval; bitmap cue text remains outside the text-cue analyzer.

## 2. Focused Review

- [x] 2.1 Add the translation quality review API contract, persisted translation-linked review record, and exact asset/source identity list and reload endpoints.
- [x] 2.2 Add focused Asset Info cue review with keep/manual edit behavior; edits use identity checks, canonical serialization, staging, and the existing Publisher.
- [x] 2.3 Add an optional separately configured Ollama reviewer model using the existing transport; suggestions remain advisory and are not auto-applied.
- [x] 2.4 Verify saved review rediscovery is scoped to matching asset and source identity and can resume after operation memory is lost.

## 3. Validation

- [x] 3.1 Run focused translation backend and Asset Info tests with fixtures/mocks only; verify no real provider, media, production SQLite, or NAS access.
- [x] 3.2 Run `openspec validate subtitle-translation-quality-review-146 --strict` and `./scripts/verify.sh --auto` through the shared validation lock; verify canonical result reports `ALL CHECKS PASSED`.
