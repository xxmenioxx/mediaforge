## 1. Backend review contract

- [x] 1.1 Add exact sidecar eligibility, source type/language, revision binding, deterministic categories, and bounded Quick/Extended review operation; verify focused backend tests cover SRT, ASS, SSA and cancellation.
- [x] 1.2 Add a proofreading-specific Ollama request/response adapter over shared provider settings; verify malformed, missing, duplicated, wrong-hash, wrong-language, and protected-control responses fail closed.
- [x] 1.3 Reuse canonical OCR findings when exact evidence exists and merge by cue identity; verify imported/translated sidecars are not mislabeled as OCR.

## 2. Explicit decision and publication

- [x] 2.1 Add revision-checked batch decisions with diff/count confirmation inputs, internal prior revision retention, provenance, atomic apply, and explicit restore; verify stale, failed-stage, rollback, and restore cases.
- [x] 2.2 Add Asset Info API routes and types for review/apply/restore through the existing subtitle operation lifecycle; verify cancellation and unavailable-provider outcomes preserve source bytes.

## 3. Asset Info workflow

- [x] 3.1 Add Review with AI to eligible text sidecars and a summary-first dialog with filters, side-by-side candidates, selected decisions, exact batch preview, and extended-mode cost warning; verify focused UI integration tests.

## 4. Validation

- [x] 4.1 Add English, Spanish, Japanese, mixed-language, mojibake, legitimate-symbol, SRT, ASS, and SSA regression cases without changing existing OCR/Translation behavior; verify focused backend and frontend checks.
- [x] 4.2 Run the issue-specific test commands and `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto`; record actual outcomes.
