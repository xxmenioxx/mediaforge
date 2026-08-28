# Task 4 — Queue selected backend design

Follow all applicable `AGENTS.md` files.

PLAN ONLY. Do not modify files.

## Goal
Design the smallest change that makes `Queue selected` backend-driven.

## Required behavior
- Frontend sends selected Asset IDs.
- Backend resolves effective config and eligibility.
- Support already queued, Needs Review, missing, invalid/blocked.
- A Path with no direct assignment is valid when inherited config is valid.
- Execution revalidates state and creates per-asset snapshots.
- Preserve natural LogicalGroup → Path → Asset ordering.
- Return explicit queued/skipped/failed results for partial success.
- Reuse existing Queue creation/snapshot logic; do not create a parallel
  pipeline.

## Output
Return only:
1. existing functions/endpoints to reuse;
2. exact files to change;
3. minimal API contract changes;
4. test plan;
5. risks.

Do not implement.
