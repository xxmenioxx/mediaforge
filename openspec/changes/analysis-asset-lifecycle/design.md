# Design: Analysis as Asset Evidence

## Current implementation and authorities

See `proposal.md` for motivation and `specs/asset-analysis/spec.md` for the
behavior contract.

- `frontend/src/App.tsx` routes `/analysis` to `AnalysisPage`; `/scanner`
  redirects there. `AppLayout` presents Analysis beside Assets and Profile
  LAB.
- `AnalysisPage` can start the existing snapshot operation, create Queue jobs,
  and read/write user decisions in the `analysisRecords` app setting. That
  setting is a legacy annotation source, not canonical media evidence.
- `AssetRow` in `frontend/src/pages/AssetsPage.tsx` owns Asset Info. Overview
  reads the inventory/lifecycle projection. The persisted snapshot query is
  enabled only while Asset Info is open on an evidence-dependent tab; explicit
  refresh starts the existing snapshot operation. Advisor is independently
  queried only when its explicit surface is open.
- Backend `models.ScanResult` and `ScannerHandler` own persisted technical
  evidence, snapshot identity, reuse, and stale-component refresh. The asset
  lifecycle projection currently exposes only a bounded `hasAnalysis`
  summary and refresh capability.
- `QueueHandler` owns selected-asset preview/eligibility and the later claim,
  preparation, effective-resolution, and immutable freeze. Queue already
  captures selected analysis needed by execution in its existing frozen
  profile/execution plan.
- `HistoryPage`, Queue jobs, execution plans, reports, and artifacts are the
  existing result/history authorities. They remain the correlation chain; #52
  must not add another report store.

## Goals / Non-Goals

**Goals:**

- Move source-evidence interaction to lazy Asset Info and preserve its
  persisted-only Overview.
- Carry the same evidence status and identity through Batch Preflight and the
  existing Queue freeze boundary.
- Preserve source evidence identity across execution and legacy route/history
  migration.

**Non-Goals:**

- A second analyzer, persisted analysis model, cache, or report store.
- Automatic scanning from Overview, inventory rendering, Batch Preflight, or
  History.
- Replacing Queue, Advisor, Profile LAB, or the future #53 Validation lifecycle.
- Deleting or rewriting legacy decisions, historical snapshots, jobs, reports,
  or source media.

## Decisions

### 1. Asset Info is the only interactive source-evidence surface

Move the standalone page's asset evidence interactions into the existing lazy
Analysis/Technical Snapshot section in Asset Info. The section may display the
canonical snapshot and its AS-IS/direct-play comparison, and provide an
explicit Analyze or Refresh action. Reuse the existing snapshot query,
operation lifecycle, cache, and server endpoint. Do not run analysis as a
side-effect of opening Asset Info, rendering Overview, expanding inventory,
opening a batch dialog, or viewing History.

Alternative considered: keep the standalone page and link it from Assets.
That preserves two entry workflows and keeps user decisions disconnected from
the asset lifecycle.

### 2. Evidence state is a projection over canonical persisted evidence

Expose a backend-authoritative status for the current asset identity:

| State | Meaning |
| --- | --- |
| `missing` | No canonical snapshot exists for the current source identity. |
| `ready` | A persisted snapshot exists and its source/policy identity is current. |
| `stale` | A prior snapshot exists, but one or more identity or evidence components require refresh. |
| `unavailable` | The asset/evidence cannot currently be read or classified; include a bounded reason when available. |

The identity must follow scanner cache rules and existing source revision and
policy evidence. Path equality alone is insufficient. Unknown legacy identity
stays unavailable or stale; it is not relabeled current. Preserve each
component's existing status and provenance, including inconclusive and failed
sub-analysis values. A successful partial refresh must not imply that every
component is ready.

Alternative considered: infer freshness in React from timestamps or path.
Freshness is backend policy and source identity; frontend inference could show
replaced media as current and disagree with Queue preparation.

### 3. Analysis is orchestration over existing evidence, not a new domain model

The Analysis section presents existing `ScanResult` components and links to
existing evidence. Scan/ffprobe, AS-IS, Advisor, cadence/GOP, HDR/color, audio,
and subtitle evidence keep their current owners and formats. Snapshot
invalidation, reuse, refresh, and operation state remain in the scanner. Do not
introduce an `AnalysisResult`, parallel analyzer, duplicate cache, or an
Asset-Info-only evidence generation path. Advisor keeps its own explicit
request and response; LAB keeps its own sampling/experiment lifecycle.

Alternative considered: normalize every evidence family into a new Analysis
record. That duplicates existing `ScanResult`, reports, Advisor findings, and
Queue snapshots while adding another synchronization authority.

### 4. Batch Preflight reads the same evidence contract

The existing selected-assets preview remains the authority for per-asset
eligibility and resolved configuration. Batch Preflight should batch-resolve
the same canonical evidence status/identity used by Asset Info and report
which evidence is ready, stale, missing, or unavailable for each eligible
asset. Preflight is read-only: it must not start scans or silently refresh
evidence. It must not create a separate per-row query pattern or infer
eligibility from an Analysis badge.

Queue submission remains a lightweight intent. At claim/preparation, Queue
loads or refreshes whatever canonical evidence is required, resolves the
effective plan, and atomically freezes that plan with an immutable reference
and bounded copy/manifest of the evidence actually consumed. Evidence not used
to resolve/explain the accepted plan need not be copied into the job snapshot.
The freeze records source identity, evidence component/version identity, and
whether evidence was reused/refreshed/unavailable. Changes after freeze cannot
rewrite the plan or evidence reference.

Alternative considered: scan every selected asset when the batch dialog opens.
This makes a read-only preview expensive and gives it side effects that can
race the later claim/preparation boundary.

### 5. Preserve an explicit History correlation chain

Per-asset History connects the source snapshot identity and its components to
the frozen plan, execution result, and a future #53 Validation record when one
exists. Each link names its existing authority and stable record ID. Historical
plans/results retain their source evidence identity even if the current asset
is rescanned, renamed, moved, or replaced. Missing links remain pending,
unavailable, or unresolved rather than being inferred from path/time alone.

This issue does not add Validation status or policy. Until #53 supplies a
canonical record, History shows the Validation integration as pending or
unavailable without implying success or failure.

Alternative considered: copy the entire current scan into every job. The plan
should retain only evidence that it consumes or needs to explain, with a stable
canonical reference where sufficient; this avoids unbounded Queue snapshot
growth.

### 6. Retire the standalone UI with reversible compatibility

Remove the Analysis item and standalone page workflow. Redirect `/analysis`
and `/scanner` to RAW Assets. If an old URL carries an asset path, preserve it
as an encoded Asset Info deep link that opens Overview first; do not auto-run
analysis. If the asset is not resolvable in current inventory, retain the
search/context and show a recoverable not-found/unavailable state rather than
opening an arbitrary path. Keep existing scan and supported backend API
contracts.

The `analysisRecords` setting contains user-authored decisions/notes as well
as snapshots. Preserve it during migration. Surface a legacy decision in the
asset History only when a deterministic source identity match is available;
show unmatched records as unlinked legacy history until the user can associate
them safely. Never discard them, manufacture an AssetRecord, or promote their
embedded scan under a different source identity. No bulk path-only migration.

Alternative considered: return 404 for `/analysis` and `/scanner`. Redirects
preserve bookmarks and application notifications while letting the user reach
the canonical asset surface without changing stored media state.

## Data and lifecycle flow

```text
RAW / Library inventory
        │ persisted identity only
        ▼
Asset Info Overview ── zero media/evidence work
        │ user opens Analysis section
        ▼
canonical ScanResult projection ── missing / ready / stale / unavailable
        │ explicit analyze/refresh only
        ▼
existing scanner operation and persisted evidence
        ├── Asset Info Analysis
        └── Batch Preflight (read-only, batch-resolved)
                 │ Queue intent → claim/preparation
                 ▼
           immutable frozen plan + evidence identity
                 ▼
           execution result → History → future #53 Validation link
```

## Risks / Trade-offs

- [Historical annotation identity is incomplete] → Keep ambiguous
  `analysisRecords` unlinked and read-only; do not infer an asset from a reused
  path.
- [A status summary is too coarse for partial scans] → Preserve component
  outcomes and stale-component detail; `ready` means the reusable snapshot
  identity is valid, not that every optional detector succeeded.
- [Queue provenance grows snapshots] → Freeze only components actually used or
  needed to explain the accepted plan and prefer an immutable canonical
  reference when sufficient.
- [Resource-constrained NAS] → Asset Info and Batch Preflight use bounded
  persisted batch reads; only explicit Analysis actions may launch existing
  scanner work.

## Migration Plan

- Keep `/analysis` and `/scanner` as redirect compatibility routes for one
  migration cycle; preserve query context and do not run analysis during the
  redirect.
- Preserve the existing scanner routes/operation records and `analysisPolicy`
  settings consumed by those routes and settings UI.
- Read legacy `analysisRecords` without changing or deleting them. Deterministic
  matched entries may be projected as historical annotations; all others stay
  accessible as unlinked legacy records. Any later deletion/retention policy
  requires separate explicit scope.
- Do not rewrite old Queue snapshots/results or stamp present-day source
  identity onto historical evidence.

Rollback is reversible: restore the standalone route/menu if needed while
leaving canonical snapshots and compatibility redirects/data intact. Do not
delete legacy settings or historical records during either direction.

## Implementation Checks

- Do not implement #53 Validation behavior; consume only a future link contract
  if/when that issue defines one.
- Do not merge Advisor or LAB into the Analysis capability or trigger either
  automatically.
- Do not add media work to list, batch preview, Overview, or History render.
- During implementation, verify whether the existing frozen execution plan
  already captures enough component identity for each resolver consumer. Add
  only the missing immutable provenance fields; do not duplicate the evidence
  payload when a stable canonical reference suffices.
- Inventory all legacy Analysis deep-link shapes and saved decision variants
  before implementing their compatibility adapter. An unmatchable record
  remains preserved and explicitly unlinked; this does not change the selected
  route or migration contract.
