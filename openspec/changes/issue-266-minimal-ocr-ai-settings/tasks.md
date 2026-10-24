## 1. Settings contract

- [x] 1.1 Add the independent `ocrAISettings` registry value and idempotent disabled default migration.
- [x] 1.2 Add the minimal OCR AI card using the shared Ollama catalog and readiness APIs.
- [x] 1.3 Require a non-empty valid model for enabled OCR AI Settings API writes.

## 2. Regression coverage

- [x] 2.1 Cover normalization, invalid values, migration, and preservation of existing Translation configuration.
- [x] 2.2 Cover defaults, independent selection, missing/offline model states, draft preservation through refetch/navigation and central save/refetch, and disabling without model requests.
- [x] 2.3 Cover enabled-model API validation, persistence/reload, atomic batch rejection, and Translation preservation.

## 3. Validation

- [x] 3.1 Run focused backend and frontend tests.
- [x] 3.2 Validate this OpenSpec change in strict mode.
