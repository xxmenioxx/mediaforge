## Why

Track language identity is currently interpreted differently by subtitle rules, audio selection, saved profile preferences, and frontend controls. Equivalent tags can therefore select different streams before a Queue plan is frozen, while changing persisted selections by language would risk collapsing distinct stream identities.

## What Changes

- Define one canonical, testable normalization contract for embedded and external media track languages, including the existing language set and aliases, explicit `ko-KR`/`fr-FR` tags, casing, blanks, and `und`.
- Apply normalization to profile language filters/defaults, Settings preferred languages, asset defaults, subtitle rules, and new Queue/Preflight resolution while retaining source metadata and index-based explicit selections.
- Make existing language labels and inputs use the same frontend catalog and canonicalization behavior.
- Preserve saved profiles and frozen Queue executions without a database migration or rewriting active snapshots.

Out of scope: translation providers, OCR language-pack support, new languages, automatic reprocessing, and changes to stream capabilities or output naming policy.

## Capabilities

### New Capabilities
- `track-language-normalization`: Shared language identity and policy behavior across track metadata, settings, profiles, Asset Info, and Queue planning.

### Modified Capabilities
- None.

## Impact

- Backend track resolution and settings normalization in `backend/internal/handlers`; profile/default/preference APIs continue using their existing JSON fields.
- Frontend language utilities and existing Settings, Track Profiles, Profile Lab, Asset Info, and Queue/Preflight labels.
- Existing Queue snapshots and source metadata remain immutable; aliases are normalized when new decisions are resolved, and explicit audio/subtitle stream indexes remain authoritative.
- No new dependency, database column, or media probe is required.
