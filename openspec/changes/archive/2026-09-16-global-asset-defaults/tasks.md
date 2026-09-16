## 1. Asset Defaults model and persistence

- [x] 1.1 Add the versioned Asset Defaults setting model with independent Video, Audio, and Tracks source states (`custom` or existing profile reference), compatibility-preserving initialization, validation, and API types; verify settings round-trip and invalid profile/reference handling.
- [x] 1.2 Add Settings → Asset Defaults UI with separate Video General, Audio General, and Tracks General sections; verify each section clearly shows Custom or Profile and profile selection stores a reference without duplicating profile fields.

## 2. Canonical effective resolution

- [x] 2.1 Integrate Asset Defaults into backend Video, Audio, and Track resolution at the existing fallback boundary, preserving `Asset > Path > LogicalGroup > SourceGroup > Global/default` and explicit disabled states; verify focused precedence regressions for each domain.
- [x] 2.2 Resolve Custom or referenced Video defaults with runtime-supported encoders plus Copy and optional non-primary video-track removal; verify capability-driven choices and resolved video stream decisions.
- [x] 2.3 Resolve Custom or referenced Audio defaults for compatibility-track enablement, quality, channel count, resolved source stream, and preferred default language; verify explicit asset/path/profile audio decisions override the global default.
- [x] 2.4 Resolve Custom or referenced Tracks defaults for subtitle disposition, SRT/ASS/both format selection, existing OCR mode, attachment policy, and explicit font policy; verify existing canonical `subtitles` planning and ASS/font warnings remain authoritative.

## 3. Canonical subtitle integration

- [x] 3.1 Extend the existing subtitle plan input to represent one logical `both` request with two output artifacts sharing one normalized cue/OCR result; verify text and bitmap sources invoke extraction/OCR once and render valid SRT and ASS outputs.
- [x] 3.2 Verify unsupported OCR/tool and invalid profile/default cases fail explicitly without misleading subtitle artifacts or changes to source media; preserve existing sidecar identity, validation, and rollback behavior.

## 4. Entry-point and Queue lifecycle integration

- [x] 4.1 Make Preview, Test Encode, and Queue consume the same resolved Asset Defaults result and canonical subtitle plan; verify equivalent unassigned assets report identical effective Video, Audio, Track, and subtitle metadata.
- [x] 4.2 Persist the resolved defaults and canonical subtitle artifact plan in newly captured Queue snapshots, while keeping existing snapshots and legacy subtitle plans authoritative; verify settings edits affect future jobs only.
- [x] 4.3 Ensure Worker preparation, execution, and retry consume the frozen Queue snapshot without re-resolving changed Asset Defaults; verify retry and legacy-snapshot regressions.

## 5. Regression coverage and validation

- [x] 5.1 Add focused backend tests for Custom/Profile selection, all domain precedence cases, runtime encoder filtering, video-track removal, audio compatibility-track settings, language defaults, subtitle SRT/ASS/both single-OCR behavior, attachment/font policies, invalid references, and compatibility defaults.
- [x] 5.2 Add focused frontend/API tests for Asset Defaults source-state UX, profile-reference persistence, and parity-facing response fields without duplicating backend policy.
- [x] 5.3 Run `./scripts/verify.sh --auto` and verify it reports `ALL CHECKS PASSED`.
