# Design — Library Path Rename

**Depends on:** `spec.md`

## Existing reuse boundary

### Canonical naming

`backend/internal/handlers/workers.go`

```text
multiEpisodeNameSpecForJob
formatMultiEpisodeOutputRelativePath
episodeSeriesTitle
seasonNumberFromPath
episodeNumberFromAssetGroup
```

`backend/internal/handlers/asset_order.go`

```text
episodeSequencePositions
```

### Closest existing rename flow

`backend/internal/handlers/assets.go`

```text
applyDirectPublicationEpisodeNames
```

It already contains:

```text
canonical target calculation
duplicate-target detection
media rename
owned sidecar rename
reverse-order rollback
```

Split its reusable logic into planning vs execution instead of creating a second rename implementation.

### Sidecar ownership

`backend/internal/handlers/assets.go`

```text
externalSubtitlesForMedia
externalSubtitleNameForStem
```

These already define `.srt` / `.ass` ownership.

### Current-state migration precedents

`backend/internal/handlers/assets.go`

```text
AssetHandler.Rename
AssetHandler.MigratePath
migrateSingleAssetPathOverrides
migrateAssetPathOverrides
```

These show how path changes currently affect:

```text
AssetRecord
ScanResult
metadata/review/conversion overrides
ProfileAssignment
QueueJob path references
DirectPublication
```

Do not copy their DB updates blindly: only current publication references should move; retired/historical publication records remain historical.

### Active-operation checks

```text
QueueHandler.assetHasOpenJob
activeAssetMaintenance
```

## Backend shape

Keep the feature in the existing Assets domain unless implementation size justifies extraction.

Suggested internal boundary:

```go
type LibraryRenamePlan struct {
    LibraryID  uint
    SourcePath string
    TargetPath string
    Assets     []LibraryRenameAssetPlan
    Warnings   []string
    Conflicts  []LibraryRenameConflict
    Hash       string
}

func buildLibraryRenamePlan(...) (LibraryRenamePlan, error)
func applyLibraryRenamePlan(...) error
```

`buildLibraryRenamePlan()` is read-only.

`applyLibraryRenamePlan()` executes a previously reviewed plan after rebuilding and comparing its hash.

## API

Suggested routes:

```text
POST /api/assets/library-rename/preview
POST /api/assets/library-rename/apply
```

Preview request:

```json
{
  "libraryId": 4,
  "path": "/media/library/series/Doctor Who/Season2"
}
```

Apply request:

```json
{
  "libraryId": 4,
  "path": "/media/library/series/Doctor Who/Season2",
  "planHash": "sha256:..."
}
```

The client does not submit authoritative target paths.

## Planning

For one Library path:

```text
load Library
load current AssetRecords under path
resolve canonical target per asset
resolve owned SRT/ASS sidecars
detect conflicts
build deterministic plan hash
```

Do not use `resolveMVFFileDestination()` for this feature; collisions block Apply.

## Apply

```text
rebuild plan
compare planHash
validate all sources/targets
check active work
move media + sidecars
rollback completed moves on failure
migrate current DB state
remove old directories only if empty
```

Prefer the existing `os.Rename` + reverse rollback pattern from `applyDirectPublicationEpisodeNames()`.

## Current-state reconciliation

Reuse/extract the smallest helper from existing rename/migration flows.

For each:

```text
oldPath → newPath
```

update current path keyed state such as:

```text
AssetRecord
ScanResult
metadata/review/conversion overrides
ProfileAssignment
current QueueJob publication references
current DirectPublication PublishedPath
```

Confirm exact QueueJob columns during implementation before changing them.

Do not rewrite retired publication history.

## Frontend

Existing insertion point:

`frontend/src/pages/AssetsPage.tsx`

```text
migrationControls
```

Current UI already has:

```text
Move path to library
[Move path]
```

Add:

```text
[Rename path]
```

for eligible Library groups.

Prefer a focused component:

```text
frontend/src/components/LibraryPathRenameDialog.tsx
```

Dialog shows:

```text
Current
Proposed
Sidecars
Warnings / conflicts
```

## Files expected to change

```text
backend/internal/handlers/assets.go
backend/internal/routes/routes.go
backend/internal/handlers/assets_test.go

frontend/src/api/client.ts
frontend/src/api/types.ts
frontend/src/pages/AssetsPage.tsx
frontend/src/components/LibraryPathRenameDialog.tsx
```

Additional test file only if useful for the new dialog.
