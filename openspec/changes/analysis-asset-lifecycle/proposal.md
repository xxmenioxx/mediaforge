# Analysis as Asset Evidence

## Why

Issue #21 made RAW and Library the only inventory locations and established
Asset Info as the per-asset lifecycle surface. Analysis still exists as a
standalone top-level workflow, while Asset Info already exposes a lazy
Technical Snapshot section backed by the persisted canonical `ScanResult`.
That split invites duplicate scans and hides the relationship between source
evidence, the plan Queue accepted, and the resulting execution.

## What Changes

- Make existing persisted source evidence discoverable and actionable from the
  asset's lazy Analysis section in Asset Info.
- Keep Overview persisted-only: opening Asset Info or rendering Overview does
  not probe media, refresh a scan, start Advisor, or run other heavy work.
- Present evidence as `missing`, `ready`, `stale`, or `unavailable`, with an
  explicit user action to create or refresh evidence.
- Reuse the existing scan/ffprobe, AS-IS, Advisor, cadence/GOP, HDR/color,
  audio, and subtitle evidence authorities; add no parallel analysis model or
  pipeline.
- Have Batch Preflight consume the same per-asset evidence and status contract
  used by Asset Info. Queue remains authoritative for accepted/frozen execution
  state and freezes the evidence identity and payload needed to explain its
  accepted plan.
- Connect source evidence, frozen plan, execution result, and the future #53
  Validation record in per-asset History without claiming to implement #53.
- Remove the standalone Analysis navigation/page while safely redirecting old
  Analysis and Scanner routes and preserving legacy saved decisions and links.
- Keep Profile LAB and Advisor as separate existing capabilities.

## Capabilities

### New Capabilities

- `asset-analysis`: Lazy asset evidence, its states and identity, explicit
  refresh, preflight consumption, Queue freeze provenance, History links, and
  standalone Analysis route compatibility.

### Modified Capabilities

None. The completed #21 `assets` delta is not yet published under
`openspec/specs/`; this change keeps the new Analysis contract in one capability
instead of creating a misleading delta against a nonexistent base spec.

## Boundaries

- #21 Assets lifecycle remains authoritative for RAW/Library inventory,
  persisted Overview, and asset identity.
- Queue owns claim, preparation, effective configuration, and immutable plan
  freeze. #52 adds evidence provenance to that existing boundary; it does not
  move preparation into the UI or create a second Queue state machine.
- #53 owns post-conversion Validation behavior, states, and policy. #52 only
  preserves the link point and represents absent future Validation as pending
  or unavailable.
- Advisor remains an explicit, separate recommendation capability. Profile
  LAB remains a separate experimentation capability.
- Existing scan APIs, `ScanResult`, operation tracking, persisted reports, and
  supported API callers remain canonical. Removing the standalone page does
  not require deleting those backend contracts.
- Existing user-authored Analysis decisions/notes must remain recoverable.
  Match and expose them as legacy History annotations only when their source
  identity can be established; otherwise retain them visibly as unlinked legacy
  records. Never attach old evidence to a new asset solely because its path was
  reused.

## Impact

Frontend routing/navigation and Asset Info; backend asset evidence projection,
Batch Preflight responses, Queue frozen-plan provenance, and History links;
compatibility treatment of `analysisRecords` and old `/analysis` and
`/scanner` URLs. No new media-analysis engine, evidence store, workflow state
machine, Validation implementation, or changes to source media.
