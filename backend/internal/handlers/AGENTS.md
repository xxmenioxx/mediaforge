# Media Pipeline Handler Instructions

These instructions apply to the HTTP handlers and the in-process media worker.
This directory contains high-risk filesystem and FFmpeg behavior.

## Required separation

Maintain this flow:

1. Probe and parse source facts.
2. Resolve profile snapshots and asset overrides.
3. Resolve requested encoder against probed capabilities.
4. Build an effective `MediaJobPlan`.
5. Build an FFmpeg argument array.
6. Execute and monitor the process.
7. Probe and validate output.
8. Publish, archive the original, and clean staging.
9. Persist artifacts, logs, paths, decisions, and downgrades.

Do not make the command builder probe hardware, mutate database state, or move
files. Do not let UI labels become the source of encoding policy.

## FFmpeg and media rules

- Start command changes in `media_command.go` and add assertions to
  `media_command_test.go`.
- Map only streams that exist. Omit preservation flags for absent streams.
- Text and bitmap subtitles are different domains; bitmap-to-text requires OCR.
- Crop, scale, SAR, DAR, field order, frame rate, color characteristics, pixel
  format, and bit depth must be treated independently.
- Deinterlacing must use measured parity when conclusive; do not trust stale
  container metadata over distributed analysis.
- Preserve unknown color characteristics unless an explicit, supported color
  policy is selected.
- Never silently replace requested hardware encoding with software. Record the
  effective encoder and downgrade reason.
- Generated command strings in reports are diagnostic renderings only; process
  execution must continue to use argument arrays.

## Filesystem and lifecycle rules

- Validate that paths remain inside configured raw, staging, library, or archive
  roots for the intended operation.
- Cross-device moves need a safe copy/verify/remove strategy; do not assume
  `rename` works across mounts.
- Publishing must not archive the original before the destination output and
  required sidecars are safely available.
- Recovery must not delete unrelated Library sidecars or reports.
- Stage transitions and `stageHistory` must reflect real work, not inferred UI
  progress.
- Queue profile snapshots are immutable evidence. Apply overrides to a cloned
  effective configuration.

## Minimum tests by change

- FFmpeg arguments: `media_command_test.go`.
- Queue/profile snapshots: `queue_test.go`.
- Worker/stage behavior: worker and lifecycle tests.
- Publish/archive/recovery/path behavior: publisher and asset tests.
- Subtitle generation/publication: subtitle transform/OCR tests.
- Analysis algorithms: focused scan, crop, interlace, or color tests.

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
