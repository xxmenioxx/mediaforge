## 1. Backend lifecycle and safety

- [x] 1.1 Record `publishing` at the successful fenced Publisher claim, leave publication commit in that stage, and verify no archive-pending path records `completed` early.
- [x] 1.2 Record `archiving_original` at the successful archive lease claim and `completed` only with a persisted terminal outcome; verify stale leases cannot commit either stage or Queue archive path.
- [x] 1.3 Extend canonical active-work checks for published jobs with archive `pending`/`archiving`; verify lifecycle capabilities and Rename reject the exact source without filesystem or provenance changes.
- [x] 1.4 Cover Publisher recovery, archive retries, failure, not-applicable, path blank while active, and canonical AssetRecord linkage with focused Go tests.

## 2. Frontend lifecycle and refresh

- [x] 2.1 Poll Queue jobs and Assets while publication/archive work is nonterminal, and verify polling stops for complete, failed, and not-applicable states.
- [x] 2.2 Show independent archive status and failure summary in Queue, Job Details, and Asset Info; verify archived originals appear only from the refreshed canonical projection.
- [x] 2.3 Render only persisted stage-history timestamps, show unvisited historical stages as not recorded, and show active publishing/archiving without changing Queue status.
- [x] 2.4 Add focused frontend coverage for archive states, polling boundaries, terminal states, historical timeline evidence, and automatic Asset Info refresh.

## 3. Validation

- [x] 3.1 Validate the change with `openspec validate publisher-archive-lifecycle --type change --strict`.
- [x] 3.2 Run `./scripts/verify.sh --auto` and record its final check count and result (`9/9`, `ALL CHECKS PASSED`).

## 4. Converted asset Rename and sidecar integrity

- [x] 4.1 Guard manual Rename through the canonical active-work predicate so exact RAW and published paths plus source asset identity cover pending/active original archival; return 409 before any mutation.
- [x] 4.2 Verify complete, failed, and not-applicable archive outcomes release only this temporary restriction while preserving existing Rename policy.
- [x] 4.3 Preserve recognized SRT/ASS/SSA ownership, suffixes, collision preflight, filesystem rollback, current AssetRecord/ScanResult/Queue/profile/override references, and immutable Publisher/archive evidence.
- [x] 4.4 Verify Asset Info still resolves the canonical archived original after the converted path is renamed; document NFO/artwork ownership as a separate follow-up because no canonical association exists.
- [x] 4.5 Add focused regressions for distinct RAW/PublishedPath blocking, terminal Rename, sidecar move failure rollback, and sidecar collision atomicity.

## 5. READY refresh and UI feedback

- [x] 5.1 Poll READY publications only when the loaded AutoPublisher setting is enabled; explain the temporary archival Rename block and verify Asset Info re-enables Rename after terminal refresh.
- [x] 5.2 Run focused Go and frontend lifecycle tests, strict OpenSpec validation, and `./scripts/verify.sh --auto`.
