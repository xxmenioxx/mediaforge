## Why

Permanent deletion of an archived original can repeatedly rebuild lifecycle projections for the whole asset catalog, so latency grows with inventory size and batch size. After Publisher confirms a new publication, Assets can also retain the original path until its inventory query refreshes.

## What Changes

- Resolve Archive deletion identity from bounded related records while reusing the canonical lifecycle resolver and preserving a fresh pre-unlink fence.
- Keep the current Library publication path as the asset primary path and expose the archived original separately as provenance.
- Refresh Assets when a previously observed Queue job becomes PUBLISHED, and show successful delete feedback without waiting for the full inventory refetch.
- Record backend deletion phases and frontend request, refresh, and feedback timings without adding paths to timing events.
- Keep deletion synchronous, confirmed, fail-closed on ambiguous identity, and protective of Library media and provenance.

## Capabilities

### New Capabilities

- archive-lifecycle: defines the current published path, separate original provenance, and safe permanent deletion behavior for archived originals.

### Modified Capabilities

None.

## Impact

Backend Archive handlers, lifecycle projection queries, QueueJob indexes, and focused handler tests/benchmark; frontend Assets query refresh and delete timing utilities/tests; no public API shape change or media-processing pipeline change. Existing QueueJob and AssetRecord data remains valid.
