# Task 1 — Fix scope configuration editing

Follow all applicable `AGENTS.md` files.

## Goal
Fix `Configure title`, root Path settings, and child Path configuration so they
load persisted values before editing and only mutate fields explicitly changed.

## Required
- Reuse the existing canonical assignment/configuration APIs.
- Load current Video, Audio, Tracks, Category, and Destination values.
- Preserve `INHERIT`, `EXPLICIT_VALUE`, and `DISABLED`.
- A partial edit must not modify untouched dimensions.
- Cover LogicalGroup and Path scopes.
- Add focused regression tests.

## Done when
Opening an existing scope shows its real persisted state, and changing only one
dimension leaves all other dimensions unchanged.

Inspect first, then implement and validate. Do not stop at a plan.
