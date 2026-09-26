## Why

Subtitle defaults are resolved differently by Asset Info and by the existing Track Profile, Preview, Test Encode, and Queue paths. This makes omitted OCR/output choices behave like frontend heuristics in some cases and prevents users from reliably inheriting the defaults already configured in Settings.

## What Changes

- Define one backend-owned effective subtitle settings contract with the precedence: explicit operation override, asset/profile resolved setting, canonical global Settings, then documented built-in fallback.
- Extend the existing canonical Track Profile/subtitle planning authority so Asset Info and Queue/Profile consumers resolve output format, OCR language and quality, SRT fallback, and styling/positioning from the same effective inputs.
- Define optional request-field semantics so omission inherits while a supplied supported value is explicit; frontend heuristic defaults must not be sent as explicit operation choices.
- Freeze resolved settings at each operation/Queue boundary and preserve existing OCR lifecycle/cache, cancellation, retry, translation, preferred-language, and artifact publication contracts.
- No new user-configurable settings or parallel subtitle resolver are introduced. AssetsPage implementation changes are out of scope for this planning phase.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: Specify effective subtitle setting precedence, explicit-versus-inherited request semantics, and parity across Asset Info and existing Track Profile consumers.

## Impact

- Backend Track Profile resolution, Asset Info extraction/conversion request handling, Queue snapshots, Preview, and Test Encode planning.
- Existing `assetDefaults.tracks` and `subtitleSettings` values remain the configuration sources; existing `ResolvedTrackPlan`, OCR cache/evidence lifecycle, and styling policy remain the execution authorities.
- Focused backend regressions will cover each settings dimension and precedence boundary. No production implementation, persisted schema, migration, UI, translation, or OCR lifecycle change is authorized by this proposal.

Out of scope: new subtitle Settings, new OCR or translation behavior, changes to #114 preferred-language ranking, OCR cache identity/lifecycle redesign, and Asset Info UI implementation.
