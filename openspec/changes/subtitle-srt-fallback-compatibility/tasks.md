## 1. Setting contract and persistence

- [ ] 1.1 Add the persisted `generateSRTFallbackForASS` setting through the existing settings model/API with a missing-value default of `false`, and verify save/load and legacy-default tests.
- [ ] 1.2 Expose the setting in the existing subtitle settings surface without adding client capability or playback-policy logic, and verify the default and persisted toggle in focused frontend coverage.

## 2. Canonical effective-format resolution

- [ ] 2.1 Extend the canonical subtitle planning inputs to resolve the fallback setting before `ResolvedTrackPlan` construction, and verify ASS-on, SRT-on, ASS+SRT-on, and ASS-off behavior.
- [ ] 2.2 Normalize and deduplicate effective formats while preserving explicit requests, stream/rule precedence, semantic roles, forced/SDH metadata, and logical-transform identity; verify focused resolver tests.
- [ ] 2.3 Verify Asset Info, Track Profiles, Profile Lab, and Test Encode use the same effective-format result and show the added SRT consequence wherever ASS selection is exposed.

## 3. Native subtitle representations

- [ ] 3.1 Preserve native ASS and SSA artifacts byte/style semantics while adding an SRT fallback through the existing canonical conversion path, and verify original-plus-SRT extraction tests.
- [ ] 3.2 Verify fallback naming, sidecar metadata, role/forced/SDH qualifiers, and existing artifact satisfaction behavior for native ASS/SSA outputs.

## 4. Bitmap OCR and canonical serializer fan-out

- [ ] 4.1 Represent bitmap ASS+SRT fallback as one logical transformation using the existing shared OCR lifecycle, and verify exactly one OCR/evidence execution feeds both serializers.
- [ ] 4.2 Verify Japanese fallback, accurate selection, OCR quality corrections, canonical cue text, and bitmap geometry are applied before both SRT and ASS serialization.
- [ ] 4.3 Verify cancellation, shared-flight behavior, and the existing atomic all-output publication barrier prevent partial fallback publication.

## 5. Queue freeze and worker consumption

- [ ] 5.1 Freeze effective fallback outputs and relevant provenance in the Queue `ResolvedTrackPlan` snapshot, and verify Workers consume only the snapshot.
- [ ] 5.2 Add a regression proving settings changed after enqueue/freeze do not change the job's subtitle outputs, while a newly created retry resolves the current setting.

## 6. Cross-entry-point and compatibility regression coverage

- [ ] 6.1 Add focused backend coverage for persistence, normalization/deduplication, native ASS/SSA, bitmap OCR sharing, Asset Info, Track Profiles, Lab/Test Encode, Queue freeze, stale settings, and logical `both` behavior.
- [ ] 6.2 Add focused frontend coverage for effective SRT visibility and setting persistence in existing subtitle-output controls.
- [ ] 6.3 Run `openspec validate subtitle-srt-fallback-compatibility --type change --strict` and reconcile this checklist only after all implementation and regression evidence is complete.
