# Frontend Instructions

These instructions extend the repository-level `AGENTS.md` for the React and
TypeScript application under `frontend/`.

## Architecture

- `src/api/`: HTTP client and shared API types.
- `src/pages/`: route-level workflows.
- `src/components/`: reusable presentation and interaction components.
- `src/utils/`: pure mappings and policy helpers shared by pages.
- `src/theme/`: Material UI theme defaults.

The backend response is authoritative. Keep `src/api/types.ts` synchronized
with API changes, but do not invent fields that the backend does not return.

## UI rules

- Preserve requested and effective encoder values as distinct concepts.
- Named quality presets must initialize all represented controls atomically.
- Asset overrides, profiles, and LAB should share mapping utilities rather than
  duplicate encoder policy.
- Destructive or recovery actions need explicit scope, disabled-state reasons,
  and backend confirmation.
- Use responsive Material UI breakpoints. Important navigation and primary
  actions must remain available on mobile.
- Wide technical tables may scroll horizontally, but the page and navigation
  must not overflow the viewport.
- Do not update several related form fields through stale sequential React
  state closures; calculate the next object and commit it once.
- TanStack Query keys and invalidations must reflect the resources changed by a
  mutation.

## Validation

Available scripts are:

```bash
npm run build
npm run lint
```

There is currently no `npm test` or `npm run typecheck` script. Build runs
TypeScript compilation. Global lint has known pre-existing failures; do not add
new failures, run ESLint on changed files, and report the baseline honestly.

## Large frontend files

The following files are large and must not be read in full during exploration:

- `src/pages/ProfileLabPage.tsx`
- `src/pages/AssetsPage.tsx`

For these files:

1. Use Serena to locate the required symbols.
2. Inspect only the required symbol bodies.
3. Use `find_referencing_symbols` for dependencies/usages.
4. Use raw line reads only when Serena cannot resolve the required React/TypeScript construct.
5. After editing, inspect only the modified symbol and direct dependents.

Do not browse large test files speculatively.
Locate the relevant test or describe the exact behavior first.

## Code navigation

The repository-root `AGENTS.md` Serena MCP navigation policy applies here.
Serena MCP is the primary mechanism for code investigation: inspect known
symbols through symbol navigation and use reference navigation for
callers/usages. Use text search (`rg`, `grep`, etc.) only for discovery,
literals, routes, configuration, user-visible strings, documentation/comments,
or cases semantic navigation cannot represent; once text search discovers a
code symbol, return to Serena. Avoid repeated broad reads of large files after
the relevant symbol is known, and treat whole-file reads as a last resort.
Documentation-only work does not require semantic code navigation when it is
not useful.

For large handler files, locate the exact function first and inspect only its
body and direct dependencies.
