## Why

Built-in Video Profile presets currently overlap between database seeding and Guided Setup, while every persisted profile is editable and assignable. A single versioned template catalog will make built-ins safe to improve without silently changing user profiles or historical Queue executions.

## What Changes

- Add a canonical backend registry and read-only API for built-in Video Profile Templates, including the presets that currently exist only in Guided Setup.
- Create independent editable User Video Profiles from templates, retain template key/version provenance, and offer Blank creation.
- Make Guided Setup read the canonical registry instead of defining its own preset profiles.
- Stop seeding built-in templates as assignable database profiles and migrate existing seeded profiles without changing their IDs, current assignments, or stored Queue snapshots.
- Keep templates unavailable to assignment and preserve existing user-created profiles.
- Out of scope: Autopilot, quality optimization, visual cohort policies, automatic profile upgrades, #46 Batch Preflight, V1.x/V2 redesign, and compliance/legal work.

## Capabilities

### New Capabilities

- `profile-templates`: Versioned read-only built-in Video Profile Templates, independent User Video Profile creation and assignment, Guided Setup discovery, and compatible migration behavior.

### Modified Capabilities

None.

## Impact

- Backend: profile registry/API, profile persistence/provenance, assignment validation, startup seeding, and SQLite migration.
- Frontend: profile API types/client and the Profiles page Guided Setup and creation flow.
- Tests: template registry and clone behavior, immutability and assignment rejection, migration and Queue snapshot compatibility, and Guided Setup API consumption.
