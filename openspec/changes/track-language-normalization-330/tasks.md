## 1. Backend language contract

- [x] 1.1 Extend the shared track language normalizer with the specified aliases and exact regional tags; add table-driven coverage for casing, whitespace, aliases, blank, `und`, and unknown regions.
- [x] 1.2 Normalize saved Settings, Track Profile, and Asset Defaults language policy and legacy reads; add save/reload tests that retain order, unknown values, and stream-index selections.
- [x] 1.3 Apply normalized matching to Queue audio/subtitle language filters and default selection; test duplicate-language stream indexes and explicit per-asset selections.

## 2. Frontend catalog and existing controls

- [x] 2.1 Provide one shared frontend language normalizer/catalog and test alias, regional-tag, unknown, and undetermined behavior.
- [x] 2.2 Use the shared contract in existing Settings, Track Profiles, Profile Lab, Asset Info, and Preflight language inputs/labels; add focused component or utility regression coverage.

## 3. Queue and repository validation

- [x] 3.1 Verify Batch Preflight freezes alias-resolved track actions/indexes and that later profile edits do not rewrite the frozen plan; retain legacy snapshot behavior.
- [x] 3.2 Run strict OpenSpec validation and `./scripts/verify.sh --auto` through the serialized dupla shared-validation lock; strict OpenSpec passed and auto selected full, with 11/11 checks and `ALL CHECKS PASSED`.
