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
