# MVForge Development Instructions

MVForge is a local/self-hosted media analysis, conversion, validation, and
publication platform. Treat source media and provenance as protected data.

## Context and scope

- Repository code is the source of truth for implementation/bug-fix work.
- `docs/guides/` describes implemented behavior.
- `docs/roadmap/`, `docs/vision.md`, and V2 docs describe planned behavior and
  MUST NOT be presented as implemented.
- Use `docs/architecture/README.md` only to select one relevant architecture
  document, and read it only when needed.
- More specific `AGENTS.md` files inside `backend/`,
  `backend/internal/handlers/`, `frontend/`, and `docs/` also apply.
- Preserve existing user changes and avoid unrelated rewrites.

Repository areas:
- `backend/`: Go API, scheduler, workers, SQLite, FFmpeg execution.
- `frontend/`: React, TypeScript, MUI, TanStack Query.
- `docs/`: guides, architecture, decisions, roadmap.
- `scripts/`: maintenance/evaluation helpers; inspect before running.

## Engineering invariants

- Preserve source media and provenance.
- Do not modify media while developing/testing unless explicitly requested on
  disposable test media.
- Build external commands with argument arrays, never shell concatenation.
- Preserve public APIs unless the task changes them.
- Keep requested and effective encoder configuration separate.
- Encoder capabilities come from worker/runtime probes; never invent them.
- Record encoder fallbacks/downgrades.
- Do not equate CRF, ICQ, LA-ICQ, QSV global quality, NVENC CQ, and
  VideoToolbox bitrate/quality values.
- Keep parsing, policy, command construction, and execution independently
  testable.
- Add/update focused tests for changed behavior.

Asset/processing rules:
- Backend is authoritative for effective configuration, eligibility, and Queue
  planning; do not duplicate those decisions in React.
- Precedence:
  `Asset > Path > LogicalGroup > SourceGroup > Global/default`.
- Preserve `INHERIT`, `EXPLICIT_VALUE`, and `DISABLED / EXPLICIT_NONE` as
  distinct states.
- `SourceGroup` is source/raw; `Library` is publication/destination.
  SourceGroup is an active configuration/navigation scope only for Unprocessed
  assets; lifecycle records may retain it for provenance.
- Queue lifecycle boundaries are `QUEUE = lightweight intent`,
  `CLAIM = atomic ownership`, `PREPARING = current effective configuration plus
  required analysis and immutable freeze`, and `PROCESSING = frozen execution`.
  Configuration changes before claim affect preparation; changes after freeze
  MUST NOT alter that execution.
- Keep `AssetRecord.status` as inventory/lifecycle state. Assets/Library derives
  `QUEUED`, `PREPARING`, and `PROCESSING` visibility from existing
  `QueueJob.status`/`stage` without per-asset Queue requests.
- Preview, Test Encode, and Queue reuse canonical backend resolvers.
- Avoid N+1 asset/batch API patterns; prefer backend batch operations.

## Safety and hygiene

- Never use `rm -rf`, `git reset --hard`, force push, or destructive broad-path
  commands.
- Do not edit NAS/production databases for local-code tasks.
- Do not delete originals, reports, logs, snapshots, or sidecars as incidental
  cleanup.
- Validate filesystem targets before copy/move/rename/archive/restore/publish/
  delete operations.
- Prefer dry runs, temp directories, reversible operations, and fixtures.
- Preserve `gofmt` on changed Go files.
- Do not run global frontend lint unless explicitly required; the repo has a
  known lint baseline.
- Do not claim NAS/production validation unless performed.

## Serena MCP navigation

"Serena" means the configured Serena MCP tools, not the `serena` CLI. Do not
invoke or discover the CLI for normal repository work.

Serena MCP is the primary code-navigation mechanism for repository
investigation. Start with Serena whenever the target is code and the relevant
language/file is supported. Do not begin with broad `rg`, `grep`, `sed`, or
whole-file reads when symbol navigation can answer the question.

Use this order:

1. For a known function, method, type, field, constant, or test, use
   `find_symbol` first.
2. Use `find_referencing_symbols` when callers/usages are needed to understand
   impact or behavior.
3. If the relevant symbol is unknown, use the symbol-overview capability
   available in the installed Serena version to identify candidates before
   reading implementation bodies.
4. Use Serena diagnostics capabilities for file/symbol diagnostics when
   available.
5. Use `rg` or another targeted text search only for literals, routes, config
   keys, user-visible text, comments/docs, generated names, or when semantic
   navigation cannot represent the query.
6. When text search discovers a code symbol, immediately return to Serena and
   inspect that symbol semantically.
7. Read narrow line ranges only when symbol-level context is insufficient.
8. Read an entire source file only as a last resort.

Repeated broad reads are not an acceptable substitute for semantic navigation.
Once a relevant symbol is known, do not repeatedly read the surrounding large
file (for example `assets.go`, `workers.go`, or large test files). Inspect the
symbol and only the directly required callers, callees, helpers, or tests.

Do not depend on optional or version-specific Serena tool names beyond the
capabilities actually exposed by the configured MCP server. Prefer available
symbol, reference, overview, and diagnostics capabilities.

For scoped tasks:
- Treat named files/functions/symbols as sufficient initial context.
- Inspect the target symbol first, then only the callers, callees, helpers, or
  tests required by the task.
- For tests, locate the relevant test symbol directly rather than reading the
  complete test file.
- If a text search is used only to discover a symbol name, do not continue
  exploring the file textually after the symbol is known.
- Do not read `~/.codex/memories/MEMORY.md`, README, architecture, or guides
  unless the task requires them.
- Do not reread unchanged code.
- Do not explore unrelated areas unless a dependency, failing check, or task
  requirement points there.

Output budget:
- default symbol/reference reads: <= 10000 chars;
- overview reads: <= 12000 chars;
- avoid 20000+ chars unless a specific symbol truly requires it;
- for large functions, get location/signature first, then references/callees,
  then only the required helper bodies.

If Serena MCP is unavailable, fails, or cannot analyze the relevant
file/language, state that limitation briefly before falling back. Use the
minimum targeted built-in navigation needed and do not default to broad
whole-file reads.

## Repository validation

Primary command:

```bash
./scripts/verify.sh --auto
```

Use `--full` only for release-sensitive/cross-cutting changes,
infrastructure/shared build configuration changes, or when explicitly
requested.

Rules:
1. Let `verify.sh --auto` choose scope and focused checks.
2. Do not run repository-wide test/vet/build/lint commands individually when
   canonical verification passes.
3. If verification fails, diagnose only the reported failing check. If the
   verifier output is sufficient, fix directly; otherwise rerun only that check.
4. Keep extra failure output to ~60 lines.
5. After a fix, rerun the same canonical verifier scope.
6. Validation succeeds only with `ALL CHECKS PASSED`.

## Token budget and self-review

Minimize repository reads and tool output.

- Do not print/read full files or full diffs by default.
- Do not summarize large tool output back to yourself; extract only the
  decision-relevant result.
- Successful logs need no replay.
- After successful verification run only:

```bash
git diff --stat
git status --short
```

`verify.sh` already runs `git diff --check`; do not repeat it.

Do NOT run a full `git diff` as routine review. If inspection is required,
limit it to a relevant file/hunk with minimal context, e.g.:

```bash
git diff -U2 -- path/to/file
```

Completion reports should contain only validation scope/check count,
noteworthy warnings, and material limitations unless more detail is requested.
