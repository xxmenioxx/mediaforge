# Backend Instructions

These instructions extend the repository-level `AGENTS.md` for Go code under
`backend/`.

## Responsibilities

- Gin handlers expose the HTTP API.
- GORM models and migrations persist state in SQLite.
- Scheduler code plans work and reserves worker resources.
- Handler-side worker code builds plans, launches FFmpeg, tracks progress,
  validates output, and publishes it.
- Runtime and capability packages report what the current host can execute.

See `docs/architecture/overview.md`, `queue-lifecycle.md`, and
`conversion-pipeline.md` before changing cross-cutting behavior.

## Go rules

- Pass `context.Context` through cancellable or long-running operations.
- Use `exec.CommandContext` and argument slices for external tools.
- Never derive authorization for a path from a filename alone.
- Wrap errors with operation context while preserving the underlying error.
- Keep HTTP binding/response code separate from domain calculations where
  practical.
- Avoid package-global mutable state unless synchronization and lifecycle are
  explicit.
- Changes to persisted models require a migration/backfill strategy and tests
  for existing databases.
- SQLite operations that represent one lifecycle transition should be atomic.

## Tests

- Prefer table-driven unit tests for parsing and policy.
- Use temporary directories and disposable SQLite databases for filesystem and
  persistence tests.
- Do not require real media, GPU hardware, network access, or NAS mounts in unit
  tests.
- Hardware-specific behavior must be covered with probe fixtures or explicit
  capability inputs.

Run `gofmt`, `go test ./...`, and `go vet ./...` from `backend/`.

## Code navigation

The repository-root `AGENTS.md` Serena MCP navigation policy applies here. Serena
MCP is the primary mechanism for code investigation: inspect known symbols
through symbol navigation and use reference navigation for callers/usages.
Use text search (`rg`, `grep`, etc.) only for discovery, literals, routes,
configuration, user-visible strings, documentation/comments, or cases semantic
navigation cannot represent; once text search discovers a code symbol, return
to Serena. Avoid repeated broad reads of large files after the relevant symbol
is known, and treat whole-file reads as a last resort. Documentation-only work
does not require semantic code navigation when it is not useful.
