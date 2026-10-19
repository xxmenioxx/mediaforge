## Why

Issue #368 follows the preventive fix #366 (merged through PR #367): selected-assets Queue Preflight reused mutable custom track ProfileData, causing an accepted job to freeze a TrackProfileSnapshot resolved for a different source asset. Some already-completed conversions therefore received false track, sidecar, and manifest FAIL findings. Those immutable snapshots and prior ValidationRuns cannot be corrected by deploying the prevention patch or by running the unmodified validator again.

The canonical Validation gate currently reads the frozen job's track snapshot and required subtitle artifacts; Publisher separately rechecks the job's required sidecars. A new ValidationRun alone is insufficient if Publisher continues to derive package requirements from a corrupted historical snapshot.

## What changes

- Introduce an explicit, bounded, **read-only historical reconciliation preview** which establishes independent, execution-time authority for the per-asset intent, exact Result identity, required artifacts, and any unresolved findings.
- Create an **append-only, versioned reconciliation record** only after a deliberate operator commit with a reason, current-identity rechecks and proof that the original track expectations can be reconstructed without guessing.
- Evaluate an explicit new ValidationRun using a separate immutable reconciled expectation contract, while preserving canonical frozen Queue/ExecutionPlan/Result binding and every original ValidationRun.
- Extend the existing manual Publisher handoff to verify the same reconciled package and actual sidecars, without editing historical job snapshots or auto-publishing.

## Non-goals

- No re-encoding, rewriting media, recreating sidecars, patching SQLite snapshots or ResultArtifacts, changing global Validation policies, or blanket overrides.
- No reconstruction from today's global defaults or solely from what the worker happened to output.
- No automatic recovery based on shared path names, expected stream counts, or a favorable score.
- No retroactive PASS for unrelated findings such as unknown video FPS; insufficient independent evidence remains REVIEW_REQUIRED / BLOCKED.
- No production NAS investigation, dry-run, deployment or remediation as part of the design-only PR.
- No change to normal new-job Validation or automatic Publisher behavior.

## Relationship

- #366 / PR #367: prevention for future Queue candidates.
- #368: safe, separately approved reconciliation of historical results.
- Existing Validation quality gate, Result identity and Publisher handoff contracts remain authoritative.
