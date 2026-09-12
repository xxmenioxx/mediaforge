# Plan — Library Path Rename

**Depends on:** `spec.md`, `design.md`

## 1. Extract planner boundary

From `applyDirectPublicationEpisodeNames()`, separate read-only target planning from filesystem mutation.

Reuse:

```text
canonical naming helpers
episodeSequencePositions
externalSubtitlesForMedia
externalSubtitleNameForStem
```

Keep existing Publish-as-is behavior unchanged.

## 2. Preview API

Add backend Preview that:

```text
validates Library/path
loads affected AssetRecords
builds canonical media + sidecar moves
reports warnings/conflicts
returns deterministic planHash
```

No filesystem or DB mutation.

## 3. Apply API

Rebuild the plan and require the same hash.

Then:

```text
validate all moves
check active work
execute media/sidecar renames
rollback on failure
migrate current path keyed state
cleanup empty source directories
```

## 4. State migration helper

Extract/reuse the smallest common helper from:

```text
AssetHandler.Rename
AssetHandler.MigratePath
migrateSingleAssetPathOverrides
```

Confirm which QueueJob/DirectPublication fields represent current state before updating them.

## 5. Assets UI

Add `Rename path` beside existing path migration controls.

Use a dedicated preview/apply dialog and refresh Assets after success.

## 6. Regression coverage

Focus on:

```text
Doctor Who Season2
partial inventory ordinals
already-canonical no-op
SRT/ASS suffix preservation
unrelated file untouched
collision
stale plan
active Queue/maintenance
current-state migration
historical provenance preserved
```
