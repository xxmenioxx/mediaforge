## Why

English PGS/VobSub OCR can read uppercase `I` as `|`, while the current quality gate sends every unmatched bar to review. Existing word-level OCR vocabulary checks cannot express phrase context such as `|'m`, `| can't`, or `am |`; users need safe, editable rules and a way to re-evaluate published OCR without repeating bitmap recognition.

## What Changes

- Add a bounded, persistent, language-scoped OCR dictionary with CRUD, search/filter, validation, priority, enablement, built-in/user distinction, and test-match preview.
- Add conservative literal/context rules, including the English starter cases from issue #262. Rule matching must honor explicit token boundaries and must never globally replace `|` with `I`.
- Feed validated dictionary candidates through the existing canonical OCR analyzer and correction flow, including Translation preparation, without bypassing the existing OCR review gate.
- Preview re-evaluation against saved canonical OCR evidence, then apply accepted changes through existing revision checks, SRT/ASS serializers, atomic publication, and correction provenance. PGS/VobSub OCR is not rerun for this flow.
- Provide an isolated dictionary management component. Settings page integration remains with issue #266.

Out of scope: LLM inference or training, learning rules automatically from user edits, an OCR review navigation/modal redesign, global `|` replacement, and Settings integration owned by #266.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: add language-scoped contextual OCR dictionary management, safe deterministic matching, and previewed re-evaluation of existing canonical OCR evidence.

## Impact

- Backend: SQLite persistence and migration, bounded dictionary CRUD/search/preview API, and integration with existing OCR quality analysis, correction, and Translation preparation.
- Subtitle lifecycle: reuse canonical cue identity, published revision checks, SRT/ASS serialization, atomic publication, and bounded snapshot/provenance ownership.
- Frontend: an isolated dictionary component and API types/client with focused component tests; no Settings page or route integration.
- Compatibility: English rules are scoped to canonical English OCR language. Unsupported or unknown languages receive no English rules; unmatched bars remain reviewable.
