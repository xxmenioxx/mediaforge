# Documentation Instructions

These instructions apply to files under `docs/`.

## Document classes

- `architecture/`: concise description of implemented code and data flow.
- `guides/`: behavior operators can use today.
- `roadmap/`: proposed or planned work that is not yet available.
- `vision.md` and V2 documents: product direction, not operational truth.
- `assessments/`: asset-specific findings and recommendations.
- `ai/evaluations/`: evidence from local-model experiments.

## Rules

- Label planned behavior explicitly.
- Link architectural claims to concrete source files when useful.
- Prefer short task-oriented documents over copying large source listings.
- Do not include secrets, NAS credentials, private keys, tokens, or complete
  production database contents.
- Commands that mutate media or production state require a warning and a safer
  inspection/dry-run step first.
- Update `docs/architecture/README.md` when adding an architecture topic.
- Move validated operational behavior into `guides/`; do not present roadmap
  proposals as released features.

## Code navigation

The repository-root `AGENTS.md` Serena MCP navigation policy applies when
documentation work requires code investigation. Serena MCP is the primary
mechanism for that investigation: inspect known symbols through symbol
navigation and use reference navigation for callers/usages. Use text search
(`rg`, `grep`, etc.) only for discovery, literals, routes, configuration,
user-visible strings, documentation/comments, or cases semantic navigation
cannot represent; once text search discovers a code symbol, return to Serena.
Avoid repeated broad reads of large files after the relevant symbol is known,
and treat whole-file reads as a last resort. Documentation-only work does not
require semantic code navigation when it is not useful.
