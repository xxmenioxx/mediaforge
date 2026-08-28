# Task 3 — Remove effective-config N+1 requests

Follow all applicable `AGENTS.md` files.

## Goal
Remove the one-effective-config-request-per-asset pattern in Unprocessed.

## Required
- Reuse the canonical backend effective-configuration resolver.
- Add/reuse one batch retrieval path keyed by Asset IDs, or avoid fetching
  effective configuration until actually needed.
- Do not rebuild inheritance in React.
- Do not fetch heavy config for collapsed content unnecessarily.
- Integrate cache/invalidation with existing query patterns.
- Prove with tests that many assets do not cause N HTTP requests.

Do not redesign Queue selected in this task.

Inspect first, then implement and validate. Do not stop at a plan.
