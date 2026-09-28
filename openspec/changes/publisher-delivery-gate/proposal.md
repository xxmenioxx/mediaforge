## Purpose

Define a crash-safe Publisher delivery gate from an eligible, immutable Queue
Result to a committed Library asset, while keeping validation, frozen Queue
intent, and original archival under their existing authorities.

## What Changes

- Give each eligible Queue execution and canonical Result a stable publication
  identity, explicit delivery state, and compact immutable attempt evidence.
- Require Publisher handoff to bind to the exact #53 `ValidationRun` and any
  eligible override, and to use the destination Library and publish mode frozen
  by Queue/ExecutionPlan.
- Materialize media and required sidecars through destination-side temporary
  files and atomic per-file renames, with durable conflict, replacement, and
  interrupted-attempt recovery rules.
- Mark publication successful only after final files, required sidecars, and
  publication provenance are committed; archive the original afterward through
  #174's existing lifecycle records.
- Emit one canonical `AssetPublished` event identity after publication and the
  original-archive outcome are recorded. Subscriber failures do not reverse
  publication.
- Extend Publisher and History with exception-first, persisted summaries that
  do not scan media or roots while pages render or poll.

## Capabilities

### New Capabilities

- `publisher-delivery-gate`: Durable, idempotent delivery of an eligible Queue
  Result to its frozen Library destination, with recovery and publication audit.

## Non-goals

- Replacing or duplicating #53 `ValidationRun`, Result identity, rule findings,
  or override authority.
- Selecting or recalculating destination policy from current Settings in
  Publisher, or adding another Queue or general-purpose job scheduler.
- Adding an Asset lifecycle or archive model, changing #174 original-retention
  policy, or creating a third inventory location.
- Adding Jellyfin-specific behavior; #56 may later subscribe to the generic
  `AssetPublished` event.
- Computing full-file hashes solely for publication identity or UI display.
- Encoding, repairing, rescanning, or revalidating media as part of delivery.

## Impact

The change extends the existing backend Publisher boundary and its persisted
Queue/History summaries. It introduces publication-specific identity,
attempt/recovery evidence, and an event outbox record, while reusing QueueJob,
ExecutionPlan, #53 Result/Validation identities, and #174 AssetRecord archive
provenance. Publisher and History views read stored, bounded summaries.
