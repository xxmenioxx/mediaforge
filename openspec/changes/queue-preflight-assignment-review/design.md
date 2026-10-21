## Context

See proposal.md for motivation and specs/queue-preflight-assignment-review/spec.md for the observable contract. `PreflightSelectedAssets` already uses the shared `assetConfigurationResolver`, whose target order lets Asset override Path, Logical Group, and Source Group before Global/default fallback. The persisted candidate already carries `ProfileResolution`, a frozen `ProfileSnapshot`, supplemental Audio/Track snapshots, and a destination snapshot. `EditBatchPreflight` preserves these values; `UpdateBatchPreflight` preserves snapshots when assignment inputs are unchanged.

## Goals / Non-Goals

**Goals:** expose existing canonical evidence clearly; detect changes in unmodified assignments before acceptance; refresh only through an explicit operator action; preserve the reviewed-to-accepted frozen-plan boundary.

**Non-Goals:** redefine scope precedence, add a parallel frontend resolver, migrate persisted models, or modify accepted Queue work.

## Decisions

### Reuse candidate evidence and current inventories

Keep the backend canonical resolver and frozen snapshots as authorities. Enrich the candidate response with read-only assignment-change evidence computed in one batch from current asset records and one resolver load. Reuse the existing Profile, Library, and Settings queries at the Assets/Queue/Scanner owners for names and options; do not query once per candidate asset. The UI presents inventory labels as names, while the candidate's frozen snapshots remain authoritative for the reviewed version and effective configuration.

### Fence stale assignments and make refresh explicit

Compare each stored canonical assignment with current effective resolution by domain, including source, target path, selection, and stable profile/library identity. A `queue_batch_preflight_edit` choice is deliberate and is excluded from canonical refresh for that domain. A stale or unavailable unmodified choice blocks acceptance and ordinary resolution with actionable detail. Add an explicit re-resolve intent to the existing candidate update request; refresh current canonical assignments only for domains without a deliberate candidate override. Keep unchanged snapshots where their effective identity remains the same.

### Review changed values before acceptance

After a successful explicit refresh or named selector change, compare the previous candidate evidence with the new response. Show before-to-after profile/version, assignment source/path, supplemental profile, and destination details for changed domains. Require an explicit review acknowledgement before enabling Accept & Queue. Clear that acknowledgement when another resolution changes the candidate.

### Keep missing identities visible

Do not coerce a missing profile or Library to the first available inventory entry. Preserve its ID/key in candidate diagnostics, show the frozen candidate name/path when present, and block acceptance or re-resolution when the referenced assignment is unavailable. A deliberate replacement remains a candidate-only edit and receives the same before/after review.

## Risks / Trade-offs

- Rechecking assignments adds bounded database reads to candidate GET/Edit/Accept → load asset records and the shared resolver once per request, then compare in memory; it does not probe or scan media.
- Profile definitions can change while their assignment IDs remain the same → preserve the existing frozen candidate snapshot unless the assignment itself changes or the operator explicitly edits it.
- Inventory names may be unavailable during loading or after deletion → distinguish loading from a confirmed missing entry and retain the stable ID/key as diagnostic text.

## Migration Plan

No persisted schema migration is required. The response additions are optional for older clients; the explicit re-resolve request flag defaults to false. Existing candidates continue to load with their stored snapshots, and a changed/unavailable canonical assignment is surfaced when the candidate is next read or accepted.
