## Context

The current `models.Profile` table is the persisted source used by profile editing, assignment validation, and Queue preparation. `database.Seed` currently inserts three base Video Profiles and four recommended Video Profiles; `seedRecommendedProfiles` can also add rows to an existing database. `ProfilesPage` separately defines eight Guided Setup presets, four of which are not represented by those seven seeded rows. Queue execution snapshots are persisted independently of the editable Profile row.

## Goals / Non-Goals

**Goals:**

- Maintain one backend-owned registry containing all current seeded templates and all current Guided Setup choices.
- Keep registry entries read-only and outside the assignable `models.Profile` table.
- Clone template definitions into ordinary persisted User Profiles with immutable template key/version lineage.
- Keep existing profile IDs, assignments, saved definitions, and Queue snapshots stable.

**Non-Goals:**

- Changing profile resolution, assignment precedence, Queue planning, or profile quality policy.
- Automatically upgrading existing user profiles when a template changes.
- Removing or rewriting Queue snapshots.

## Decisions

1. **Use one static backend registry and read-only catalog API.** A typed registry holds key, version, name, category, description, definition, and optional legacy seed identity. The API lists registry entries and clones by key/version. Templates have no database IDs, so existing ID-based assignment resolution cannot assign them. The registry includes all seven currently seeded definitions and the four additional frontend-only Guided Setup definitions; the frontend no longer owns preset definitions.

2. **Represent a clone as a normal `models.Profile` plus lineage fields.** A clone copies the complete definition into a new database row, uses its requested name/scope, and stores `templateKey` and `templateVersion`. Normal profile create remains the Blank path and has no lineage. Update APIs do not accept lineage fields, so provenance is preserved while the definition remains editable. A registry version change changes only future clone output.

3. **Migrate existing seeded records in place.** Add provenance fields and identify legacy seed rows using their stable source marker and seed preset identity. Tag those rows with the corresponding template key/version without changing their configuration, name, ID, ProfileVersion, assignment rows, or Queue data. This retains an assigned seeded profile as its existing User Profile equivalent. Remove both the empty-database seed list and repeat seeding of recommended profiles; new installations receive templates only through the registry.

4. **Keep Queue snapshot migration separate from template lineage.** The profile migration changes only template provenance fields. It does not touch `profile_snapshot`, `profile_version`, or profile resolution. Existing snapshots remain frozen execution evidence; existing snapshot backfill behavior for legacy records remains in its current order.

5. **Drive Guided Setup from API data.** The page loads the catalog through the frontend API client, renders the returned choices, and calls the clone operation. Blank creation continues through the ordinary profile create flow. A failed catalog request is shown as an error rather than falling back to client-side built-ins.

## Risks / Trade-offs

- [Legacy identity may be missing or edited] → Match only known seed identities with the seed source marker; leave all other profiles and their behavior untouched. Tests cover each recognized seed identity.
- [A template update can change future output] → Require explicit version increments and persist the cloned version on each user profile.
- [A clone could be confused with a template in UI] → Templates are returned by a dedicated endpoint and never appear in the persisted profile list until cloned.
- [Catalog unavailable during Guided Setup] → Show the request error and keep Blank profile creation available.

## Migration Plan

1. Auto-migrate lineage columns, then tag recognized legacy seeded profiles in place before Queue snapshot backfill runs.
2. Stop seeding built-in profiles; serve their definitions and frontend-only presets from the registry.
3. Continue loading existing assignments and frozen Queue data without rewriting their IDs or snapshots.
4. On rollback, the added lineage columns are harmless; persisted User Profile rows, assignments, and Queue snapshots remain valid for the old code.

## Open Questions

None.
