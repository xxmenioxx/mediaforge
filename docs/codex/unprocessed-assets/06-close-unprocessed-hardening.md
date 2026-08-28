# Task 6 — Close Unprocessed Assets hardening

Follow all applicable `AGENTS.md` files.

## Goal

Close the remaining correctness and performance issues in `Queue selected` without changing the current Unprocessed UI or domain model.

## Required

1. `Queue selected` must only operate on `status=unprocessed` records.
   - IDs from Converted/Library/Archive must be skipped/rejected explicitly.
   - Add a regression test.

2. Replace per-asset effective-configuration DB scans with one real backend batch resolver.
   - Load required Asset records once.
   - Load SourceGroups once.
   - Load ProfileAssignments once.
   - Load AssetScopeConfigurations once.
   - Resolve N assets in memory using the existing precedence:
     `Asset > Path > LogicalGroup > SourceGroup > Global/default`.
   - Do not create a second inheritance implementation.

3. Reuse the pre-resolved effective configuration during `Queue selected`.
   - Do not resolve the same asset repeatedly during eligibility + Queue preparation.
   - Queue snapshots must still contain each asset's own effective configuration.

4. Confirmation must execute only Asset IDs that were `eligible` in the approved prepare result.
   - Backend must still revalidate those IDs at execute time.
   - A record that was skipped/failed during prepare must not become newly queued just because its state changed before confirmation.
   - Preserve explicit `queued / skipped / failed` results.

5. Preserve:
   - duplicate prevention;
   - Needs Review behavior;
   - missing behavior;
   - natural ordering;
   - per-asset snapshots;
   - partial-success reporting.

## Validation

Add focused tests for:

- non-Unprocessed Asset ID is not queued;
- batch resolver returns correct mixed inheritance results;
- many assets do not cause repeated full assignment/configuration table scans;
- one asset is not effectively resolved multiple times in one Queue prepare/execute path;
- only prepare-eligible IDs are submitted for execution;
- execute still revalidates current state;
- mixed profiles still produce distinct snapshots.

Run all validation required by `AGENTS.md`.

## Done when

`Queue selected` is scoped to Unprocessed, uses one backend batch-resolution path, avoids repeated per-asset resolution work, and confirmation cannot silently expand beyond the set the user approved.

Inspect first, then implement and validate. Do not stop at a plan.
