# Tasks — Library Path Rename

**Depends on:** `spec.md`, `design.md`, `plan.md`

## Task 1 — Extract read-only plan boundary

Refactor `applyDirectPublicationEpisodeNames()` only as needed so canonical target calculation can be reused without mutating files.

Add focused tests proving existing Publish-as-is naming behavior is unchanged.

## Task 2 — Preview

Implement:

```text
Library/path
→ AssetRecords
→ canonical media targets
→ owned SRT/ASS targets
→ warnings/conflicts
→ planHash
```

Add route + API tests.

## Task 3 — Apply

Implement:

```text
rebuild + hash compare
active-operation checks
all-target validation
media/sidecar moves
rollback
```

Do not use `.mvf` collision fallback.

## Task 4 — Reconcile current state

Extract/reuse current path migration behavior.

Cover:

```text
AssetRecord
ScanResult
path keyed overrides
ProfileAssignment
current publication references
DirectPublication current path
```

Preserve retired/historical publication records.

## Task 5 — Frontend

Add:

```text
Rename path
```

to Library path controls.

Create `LibraryPathRenameDialog` for Preview/Apply.

## Task 6 — Regression

Required cases:

```text
Doctor Who Season2 → Season 02 / S02E##
partial inventory keeps complete-inventory ordinals
already canonical → no-op
SRT/ASS follow media stem
unrelated files stay
collision blocked
stale plan blocked
active Queue/maintenance blocked
current state follows rename
historical provenance remains unchanged
```
