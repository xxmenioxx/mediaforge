## 1. Backend compound persistence foundation

- [x] 1.1 Extract reusable metadata normalization and conversion input validation/normalization from `UpdateMetadata` and `UpdateConversion`, including stream indexes, subtitle roles/sidecars, cadence/field order, upscale/sharpen/custom height, encoder/rate-control, and existing empty-override semantics; verify the retained endpoint tests still pass without contract changes.
- [x] 1.2 Add the compound per-asset edit input/response contract and route, performing one path resolution/authorization step and returning canonical `path`, normalized `metadata`, and normalized `conversion`; verify handler tests cover valid payloads, invalid payloads, and path authorization errors.
- [x] 1.3 Implement one transactional read-modify-write operation over the existing metadata and conversion AppSetting maps, with a shared process-local guard used by compound and retained compatibility writes; verify a forced second-domain failure rolls back the first domain.
- [x] 1.4 Add a backend regression that starts compound saves for two different asset paths concurrently and verifies both entries survive; verify no stale full-map write loses either asset.
- [x] 1.5 Add or update compatibility endpoint tests to verify `/api/assets/metadata` and `/api/assets/conversion` retain their existing request/response behavior while using shared normalization and serialized map writes.

## 2. Frontend editor state and API integration

- [x] 2.1 Add frontend types and API client support for the compound Asset Info request/response, including normalized metadata and conversion state; verify TypeScript/build coverage for the new contract.
- [x] 2.2 Add focused pure editor-state helpers or reducer logic for normalized persisted baseline, draft, semantic dirty detection, captured Save revision, success reconciliation, and failed-save retention; verify semantic equality ignores equivalent empty/order/timestamp-only differences.
- [x] 2.3 Wire Asset Info Category and all existing Asset Override, KEEP/DROP, stream metadata, sidecar, and semantic-role handlers to draft-only updates; verify Category, KEEP/DROP, sidecar, and role changes make the draft dirty without calling either compatibility mutation.
- [x] 2.4 Replace separate Asset Info Save behavior with one compound Save action and local saving/error/success state; verify multiple edits issue exactly one compound request containing coherent metadata and conversion values.
- [x] 2.5 Reconcile Save responses against the captured revision and preserve edits made during Save; verify success cleans the matching draft, failure retains the draft and baseline, and an older response does not clobber a newer dirty revision.
- [x] 2.6 Guard the persisted-baseline effect and asset initialization so clean editors follow authoritative updates, dirty editors survive unrelated Assets refreshes, and changing assets initializes from the new asset; verify background query/prop refresh and asset-switch behavior.
- [x] 2.7 Add close/switch confirmation for dirty Asset Info state and verify declining preserves the dialog/draft while confirming discards only local state without an autosave.

## 3. Targeted cache and blocking behavior

- [x] 3.1 Add a pure `AssetInventory` patch helper that updates the matching asset in flat status arrays, group asset arrays, and nested source-group logical-group path asset arrays while preserving unrelated branches and aggregates; verify all representations are patched in focused helper tests.
- [x] 3.2 Apply the compound Save response with targeted `setQueryData(['assets'], ...)` and remove normal-path full Assets invalidation; verify Asset A updates, Asset B remains unchanged, and no `['assets']` invalidation/refetch is required.
- [x] 3.3 Remove group-wide observation of per-asset track-selection mutation state and replace broad `updateConversion.isPending` disabling with local compound-editor saving/read-only rules; verify saving Asset A does not disable Asset B or unrelated folder/Queue controls.

## 4. Queue, recommendations, reset, and operation boundaries

- [x] 4.1 Remove the hidden pre-Queue conversion write and make per-asset Queue refuse a dirty compound draft with a clear Save-first explanation; verify dirty Queue creates no save request and no job, while clean Queue uses persisted state.
- [x] 4.2 Change Advisor/Profile recommendations to update the conversion draft without persistence and change Reset to clear only conversion draft fields until Save; verify recommendation and Reset tests observe no backend write before Save.
- [x] 4.3 Preserve independent ProfileAssignment, AssetScopeConfiguration, review, rename, snapshot, subtitle maintenance, Test Encode, path metadata, and Queue operation semantics; verify focused existing tests and read-only/open-job rules remain authoritative.

## 5. Focused validation and handoff

- [x] 5.1 Add focused frontend coverage for semantic dirty detection, draft-only Category/KEEP-DROP/sidecar/role changes, one compound Save, failure/retry, edit-during-save, background refresh, close/switch confirmation, targeted cache patch, no full invalidation, independent rows, dirty Queue, recommendation, and Reset behavior; verify with the repository's focused frontend test command.
- [x] 5.2 Add focused backend coverage for compound save, atomic rollback, validation/normalization reuse, concurrent different-asset saves, and compatibility endpoints; verify with the affected Go test package and `GOCACHE=/tmp/mvforge-go-cache` when needed.
- [x] 5.3 Run `openspec validate asset-info-editing-persistence --type change --strict` and verify it reports a valid change; when implementation is applied, run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED` before marking implementation tasks complete.
