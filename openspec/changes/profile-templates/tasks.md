## 1. Backend Template Catalog and Compatibility

- [x] 1.1 Add a typed backend registry containing all seven seeded Video Profiles and four frontend-only Guided Setup presets; verify each entry has stable key/version/name/category/description/definition and registry tests cover completeness.
- [x] 1.2 Add read-only catalog and clone APIs, store immutable template key/version provenance on created User Profiles, and keep assignments ID-based; verify clone independence, template immutability, unsupported-version rejection, and direct template assignment rejection with focused handler tests.
- [x] 1.3 Migrate recognized seeded profiles in place and remove built-in profile seeding; verify all seven legacy identities, preserved definitions/IDs/assignments/Queue snapshot bytes, and no built-in Profile rows on a new installation with database migration tests.

## 2. Guided Setup

- [x] 2.1 Add frontend template types and API calls, render Guided Setup from the backend catalog, provide Blank creation, and show catalog errors without a hard-coded fallback; verify the page uses the catalog response.
- [x] 2.2 Add focused frontend tests for the canonical catalog request and Guided Setup template choices, including the Blank option; run ESLint on changed frontend files and the frontend build.

## 3. Validation and Completion

- [x] 3.1 Run focused backend tests for registry, clone, assignment, migration, and Queue snapshot preservation; fix failures and rerun the same focused checks.
- [x] 3.2 Run `openspec validate profile-templates --strict` and `./scripts/verify.sh --auto`; mark completed OpenSpec tasks only after their behavior and verification pass.
