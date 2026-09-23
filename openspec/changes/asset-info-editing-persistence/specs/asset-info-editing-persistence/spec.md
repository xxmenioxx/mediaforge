## Purpose

Defines a coherent per-asset Asset Info editing contract in which metadata and
conversion overrides are reviewed locally and committed together by one explicit
Save action.

## ADDED Requirements

### Requirement: Asset Info has one compound editor document

Asset Info SHALL treat the per-asset editable Category/metadata state and the
existing Asset Override state as one compound editor document. The document
includes conversion fields exposed by Asset Info, stream KEEP/DROP selections,
stream metadata overrides, subtitle sidecar format selections, and subtitle
semantic role overrides. Asset-level metadata not editable in the current UI
SHALL be preserved when the document is saved rather than silently discarded.

Path/group metadata, profile assignments, destination scope configuration,
manual review state, rename operations, subtitle maintenance operations,
snapshot generation, Test Encode, and Queue actions SHALL remain independent
operations with their existing ownership and side effects.

#### Scenario: Compound fields share one edit document

- **WHEN** a user changes an Asset Info Category and a subtitle role before saving
- **THEN** both changes are represented by the same per-asset draft and are eligible for the same Save action

#### Scenario: Independent profile assignment remains separate

- **WHEN** a user changes the asset video, audio, or Track Profile assignment in Asset Info
- **THEN** that operation retains its existing profile-assignment persistence semantics and is not silently folded into the compound metadata/conversion Save

#### Scenario: Path category is not pulled into the asset document

- **WHEN** a user edits a path/group Category control
- **THEN** the path-scoped metadata remains an independent scope operation and is not committed as an asset-level compound edit

### Requirement: Compound controls are draft-only until explicit Save

Changing any compound-editor control MUST update local draft state without
sending a metadata or conversion request and without invalidating or refetching
the Assets inventory. This applies to Category, Asset Override fields, stream
KEEP/DROP, stream metadata, subtitle sidecar formats, and subtitle semantic
roles.

The editor SHALL expose a persisted baseline, a draft, a semantic dirty state,
and a saving state. Dirty SHALL be derived from semantic differences between
normalized baseline and normalized draft values, not from whether a mutation is
pending.

#### Scenario: KEEP/DROP is draft-only

- **WHEN** a user changes an audio or subtitle stream from KEEP to DROP
- **THEN** the local draft becomes dirty and no backend request is made

#### Scenario: Sidecar and role changes are draft-only

- **WHEN** a user changes a subtitle sidecar format and semantic role
- **THEN** both changes remain local to the draft and the unrelated Asset Info controls remain usable

#### Scenario: Category is draft-only

- **WHEN** a user changes the per-asset Category
- **THEN** the Category is shown from the dirty draft and no metadata request or Assets inventory refetch occurs

### Requirement: One explicit Save commits one coherent revision

Asset Info SHALL expose one authoritative Save action for the compound editor.
Save SHALL submit one coherent draft revision containing metadata and conversion
state, even when several controls changed. Duplicate Save submissions for the
same in-flight revision SHALL be prevented.

While Save is in flight, the editor SHALL allow further local edits that do not
violate existing read-only rules. The Save control MAY be disabled while that
request is pending, but unrelated assets and independent operations SHALL NOT
be globally disabled.

#### Scenario: Multiple changes produce one write

- **WHEN** a user keeps one audio stream, drops one subtitle stream, changes a sidecar format, changes a semantic role, and presses Save once
- **THEN** exactly one compound asset-save request is sent for the combined draft revision

#### Scenario: Metadata and conversion are coherent

- **WHEN** a user changes Category and conversion fields before pressing Save
- **THEN** the single Save commits both domains as one coherent revision or commits neither

#### Scenario: Duplicate Save is prevented

- **WHEN** the user submits Save again before the same revision completes
- **THEN** no overlapping write for that editor revision is sent

### Requirement: Save success and failure preserve revision semantics

On successful Save, the backend response SHALL become the editor's persisted
baseline after normalization, and the draft SHALL be clean when it still matches
the saved revision. On failed Save, the persisted baseline and cached asset SHALL
remain unchanged, the local draft SHALL remain intact, and retry SHALL remain
available.

If local edits were made while Save was in flight, the response for the older
revision MUST NOT replace those newer edits. The editor SHALL remain dirty for
the newer revision and SHALL use the response only as the baseline for the
revision that was actually saved.

#### Scenario: Save success becomes the baseline

- **WHEN** a compound Save succeeds and no newer local edit exists
- **THEN** the normalized response is the persisted baseline, the draft is updated to that response, and the editor is clean

#### Scenario: Failed Save retains the draft

- **WHEN** the compound Save fails
- **THEN** the previous persisted state remains visible as the baseline, the user's draft remains visible, and the user can retry

#### Scenario: Edit during Save is not clobbered

- **WHEN** revision A is saving and the user changes another field for revision B before response A arrives
- **THEN** response A does not erase revision B and the editor remains dirty for revision B

### Requirement: Authoritative refreshes do not overwrite dirty drafts

Opening Asset Info for an asset SHALL initialize its baseline and draft from
that asset's authoritative persisted state. Switching to a different asset
SHALL initialize the new editor from the new asset. For the same asset, an
authoritative inventory update MAY refresh the baseline and draft only while
the editor is clean. Unrelated background Assets refreshes MUST NOT overwrite a
dirty draft.

There SHALL be no durable unsaved-draft store in this change. Closing and
reopening may discard a draft only after the user confirms the discard.

#### Scenario: Clean editor follows authoritative refresh

- **WHEN** the current editor is clean and the current asset receives a new persisted metadata or conversion state
- **THEN** the baseline and draft reflect the normalized authoritative state

#### Scenario: Dirty editor survives background refresh

- **WHEN** the current editor is dirty and the Assets query refreshes for an unrelated reason
- **THEN** the current draft and dirty state remain unchanged

#### Scenario: Asset switch reinitializes the editor

- **WHEN** the user switches from one asset to another after accepting any discard confirmation
- **THEN** the new asset's persisted metadata and conversion state become the new baseline and draft

### Requirement: Closing or switching with unsaved changes requires confirmation

If Asset Info has unsaved compound-editor changes, closing the dialog or
switching to another asset SHALL ask the user to confirm discarding them. A
negative response SHALL keep the editor open and preserve the draft. A positive
response SHALL discard only the in-memory draft and SHALL NOT write it to the
backend.

#### Scenario: Close is cancelled

- **WHEN** the user attempts to close a dirty Asset Info dialog and declines confirmation
- **THEN** the dialog stays open with the draft intact

#### Scenario: Close discards without autosave

- **WHEN** the user confirms closing a dirty Asset Info dialog
- **THEN** the draft is discarded, no save request is sent, and reopening starts from persisted state

### Requirement: Compound persistence is atomic and normalized

The system SHALL provide one authoritative per-asset compound-save operation
whose input contains metadata and conversion state and whose successful response
contains the normalized persisted `path`, `metadata`, and `conversion` state.
The operation SHALL persist both existing override domains atomically from the
caller's perspective: a failure in either domain SHALL expose no partial
compound edit.

The operation SHALL reuse the existing metadata normalization and the existing
conversion validation/normalization rules, including applicable upscale,
sharpen, custom-height, cadence/field-order, stream-index, subtitle-role,
subtitle-sidecar, encoder, rate-control, and other conversion fields. It SHALL
not weaken validation or create a second conversion-normalization contract.

No new database table is required by this contract; the existing metadata and
conversion AppSetting stores remain the persistence domains.

#### Scenario: Successful compound response is authoritative

- **WHEN** a valid compound draft is saved
- **THEN** the response contains the canonical path and normalized persisted metadata and conversion state used by the editor and cache

#### Scenario: Validation failure saves nothing

- **WHEN** a compound request contains a value rejected by existing conversion or metadata rules
- **THEN** the request fails with the existing validation error behavior and neither domain is changed

#### Scenario: Mid-save failure rolls back both domains

- **WHEN** persistence of one compound domain fails after the other domain has been prepared
- **THEN** the caller receives an error and the metadata and conversion state remain at their pre-request values

### Requirement: Concurrent asset saves preserve independent entries

Concurrent per-asset saves SHALL serialize or otherwise coordinate the
read-modify-write of the existing metadata and conversion AppSetting maps so a
save for asset A cannot overwrite a concurrently saved entry for asset B that
started from a stale map. The guarantee applies to the compound operation and
to retained compatibility endpoint writes that touch these stores.

#### Scenario: Different assets save concurrently

- **WHEN** asset A and asset B are saved concurrently
- **THEN** both normalized persisted entries survive and can be read back independently

### Requirement: Successful Save patches targeted inventory cache state

After a successful compound Save, the current React Query Assets cache SHALL be
patched from the response for the affected normalized path. The patch SHALL
update every rendered representation of that asset in the current
`AssetInventory`, including flat status arrays, group asset arrays, and nested
source-group logical-group path asset arrays, while preserving unrelated assets
and group aggregates.

The normal compound-save success path MUST NOT invalidate or refetch the whole
`['assets']` query. A later ordinary inventory refresh remains allowed.

#### Scenario: Asset A receives a targeted patch

- **WHEN** Asset A's compound Save succeeds while Asset B is also present in the cached inventory
- **THEN** all cached Asset A representations expose the returned metadata and conversion state, Asset B is unchanged, and no full Assets invalidation is required

#### Scenario: Cache patch preserves rendering identity

- **WHEN** a cached inventory contains the same asset in multiple nested representations
- **THEN** the affected arrays/objects receive referentially safe updates for React rendering while unrelated branches are preserved

### Requirement: Compound Save does not globally block controls

Saving one asset's compound document SHALL NOT disable editing another asset,
another folder, or unrelated Queue controls because of a group-wide mutation
state. Compound controls for the same asset SHALL remain editable during Save
unless an existing read-only or concrete conflicting-operation rule applies.

#### Scenario: Independent assets remain usable

- **WHEN** Asset A is saving
- **THEN** Asset B's Asset Info and Queue controls remain available subject to Asset B's own state

#### Scenario: Group track mutation does not block unrelated controls

- **WHEN** a compound editor Save is pending for one asset in a group
- **THEN** group-wide mutation observation is not used to disable unrelated asset rows or Queue controls

### Requirement: Queue consumes only persisted compound state

Queue SHALL never silently persist an unsaved compound draft. If the current
Asset Info draft is dirty, the per-asset Queue action SHALL refuse to queue and
explain that the user must Save first because unsaved changes are not queued.
When Queue proceeds, eligibility and the queued configuration SHALL be derived
from persisted state plus the independently persisted profile/scope selections,
not from an invisible draft.

#### Scenario: Queue with dirty draft is blocked

- **WHEN** the user attempts to Queue an asset with unsaved Category or conversion changes
- **THEN** no conversion or compound-save request is made, no Queue job is created, and the user is told to Save first

#### Scenario: Queue uses the last persisted state

- **WHEN** the asset has no dirty compound draft and the user queues it
- **THEN** Queue consumes the persisted asset configuration and does not perform a hidden pre-Queue Asset Override write

### Requirement: Recommendations and Reset are draft operations

Applying Advisor/Profile recommendations SHALL update the compound conversion
draft and mark it dirty without backend persistence. Reset SHALL clear the
relevant conversion draft fields and mark the compound document dirty until
Save. Reset SHALL NOT reset metadata unless that behavior is explicitly added
to the compound editor contract.

#### Scenario: Recommendations require review and Save

- **WHEN** the user applies recommendations from Asset Info
- **THEN** the recommendations appear in the local draft, no persistence occurs, and Save is required to commit them

#### Scenario: Reset waits for Save

- **WHEN** the user presses Reset for conversion overrides
- **THEN** conversion draft fields reset locally, metadata remains unchanged, and no backend write occurs until Save

### Requirement: Read-only and compatibility behavior remains authoritative

Existing read-only rules for converted/archive/published-as-is/accepted assets
and assets with open Queue jobs SHALL continue to govern compound editing. The
existing Test Encode message and behavior that excludes unsaved Asset Override
changes SHALL remain true. The existing metadata and conversion endpoints MAY
remain available for other callers and SHALL continue to accept their existing
contracts if retained; adopting the compound endpoint for Asset Info SHALL NOT
break unrelated callers.

#### Scenario: Open job remains read-only

- **WHEN** an asset has an open Queue job under the existing read-only rules
- **THEN** Asset Info compound controls remain unavailable as they are today and the new draft contract does not bypass that protection

#### Scenario: Test Encode excludes unsaved overrides

- **WHEN** the user changes an Asset Override without saving and starts Test Encode
- **THEN** Test Encode continues to communicate and use the persisted effective configuration rather than the unsaved draft

#### Scenario: Compatibility caller remains supported

- **WHEN** an unrelated caller uses the retained metadata or conversion endpoint
- **THEN** its existing request/response contract continues to work, subject to the shared normalization and serialized store-write rules
