## Context

See proposal.md and `specs/housekeeping-reconciliation/spec.md`. Existing preview/execute is the sole cleanup lifecycle. Maintenance operations persist exact temporary and backup paths; execution already binds a preview to candidate and filesystem identity. RAW protection is currently summarized as a path set, which loses its record-level explanation.

## Goals / Non-Goals

**Goals:**
- Keep classification, authorization, and removal in the existing Housekeeping pipeline.
- Add read-only evidence to preview and to the existing Settings presentation.
- Recheck current DB and filesystem facts at execution; treat uncertain ownership as blocked.
- Keep RAW directory removal leaf-only and non-recursive.

**Non-Goals:**
- General Library or RAW file deletion, automatic cleanup, changing operation history, database migrations, or production reconciliation.

## Decisions

1. **Separate diagnosis from eligibility.** Build a maintenance candidate from the persisted operation first, then inspect the exact path under the configured Library role. A missing completed temporary is an informational no-op. A present completed temporary is only a possible residual; it becomes manual-only eligible only after operation/asset identity, backup absence, exact path ownership, references, role boundaries, and regular-file identity are proven. Existing failure/interruption retention and recovery semantics stay authoritative.
2. **Narrow Library exception.** Do not add Library to generic stage-artifact role allowlists. Resolve Library containment and its overlap with all protected storage roles in a helper used only for maintenance temporary candidates. Preserve the canonical asset and any backup. Revalidate all evidence immediately before the existing Housekeeping removal primitive.
3. **Expose structured RAW protections.** Retain the set needed by preview/execution, but derive bounded per-candidate diagnostic references from the same authoritative tables and predicates. Report record ID, field/path, status, and whether a reference is current or only suspected stale. Unknown/query errors remain blockers. Never infer staleness from an empty directory and never mutate Queue, Publisher, or AssetRecord history.
4. **Keep response changes additive.** Extend Housekeeping candidate/report JSON and the existing Settings preview UI; do not add persisted fields or a second cleanup API. Manual-only candidates use the existing explicit preview/execute authorization.
5. **Preserve exact filesystem identity.** Bind preview to the current exact file/directory identity and recheck symlink status, regular-file/hardlink safety, root overlap, references, backup dependencies, and literal emptiness at execution. If the platform cannot prove a required identity fact, block.
6. **Test only disposable state.** Use temporary role roots and isolated SQLite fixtures for present/missing/residual/replacement, backup/reference/active-operation, RAW protection provenance, symlink, hardlink where supported, and execution races. Do not inspect or alter NAS state.

## Risks / Trade-offs

- [A table query or file stat fails] → Treat the candidate as blocked and preserve a specific diagnostic.
- [Large history makes reference explanations expensive] → Query only records relevant to the previewed paths where possible; avoid per-row API requests and keep the number of diagnostic queries bounded.
- [Platform filesystem metadata cannot prove hardlink count] → Fail closed for manual file cleanup rather than weakening identity checks.
- [Persisted path is stale but not independently disproven] → Label it suspected stale and keep the directory protected until canonical inventory/reconciliation resolves it.

## Migration Plan

No database migration. Additive response fields tolerate older persisted records. Roll back by disabling the new candidate classification/UI while retaining existing protections; no history needs rewriting.

## Open Questions

None.
