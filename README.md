# MediaForge

MediaForge is an open-source, self-hosted media processing platform for preparing personal media libraries before publishing them to media servers like Jellyfin, Plex, or Emby.

It is not a media server. It is the orchestration layer between raw media files and a final, validated media library.

## Phase 1 Scope

This initial scaffold includes:

* Go REST API with Gin, GORM, and SQLite
* React + Vite + TypeScript frontend
* Material UI dark interface with left navigation
* Initial modules for Dashboard, Libraries, Profiles, Scanner, Queue, Workers, Validation, Publisher, and Settings
* Dockerfiles and Docker Compose
* Seed conversion profiles inspired by the media worker pipeline configuration model

## Phase 2 Scope

The current UI supports the first manual workflow:

* Create destination libraries
* List unprocessed and converted assets from library paths
* Evaluate assets with a deterministic pre-conversion advisor
* Create worker-ready conversion profiles
* Submit queue jobs by choosing media path, library, profile, and priority
* Test the same workflow through Swagger UI

## Phase 3 Scope

The scanner now uses `ffprobe` inside the backend container to inspect readable media files.

By default, Docker Compose mounts MediaForge-controlled host folders into the backend container:

* `./media/raw` -> `/media/raw` as read-only input media
* `./media/library` -> `/media/library` as converted/published media
* `./media/staging` -> `/media/staging` as temporary controlled output
* `./media/originals_archive` -> `/media/originals_archive` as the archive for originals after successful publish
* `./media/reports` -> `/media/reports` as persistent AS-IS, result, and log storage

Create destination library folders on the host first, similar to Jellyfin/Radarr style volume mapping. MediaForge reads
all originals from the single `/media/raw` source root and lists existing destination folders under `/media/library`
when creating or editing libraries.

Place test files under `media/raw` and scan them using container paths such as:

```sh
/media/raw/movies/example.mkv
```

Scanner results are persisted in SQLite and can be used from the UI to queue a conversion manually.

Original retention is configurable in Settings. The conversion worker will use that policy later to preserve successfully
converted originals for the configured number of days before any cleanup is allowed.

For a production-like NAS deployment, see [docs/homelab-ugreen-nas.md](docs/homelab-ugreen-nas.md). A local reset helper is available at
`scripts/reset-v1-preserve-reports.sh`; it preserves `media/reports` and requires explicit confirmation before deleting local working state.

For an installable, release-based Docker deployment that pulls versioned images from GHCR, see
[docs/docker-nas-installation.md](docs/docker-nas-installation.md). The distributable Compose file lives under
[`deploy/nas`](deploy/nas) and keeps the backend private behind the web container.

## Phase 4 Scope

Workers now have the first controlled execution lifecycle:

* Claim the next queued job by priority and creation time
* Mark jobs as running, completed, failed, or canceled
* Track progress, worker name, output path, errors, start time, and finish time
* Operate the lifecycle from the Workers UI
* Test worker lifecycle endpoints through Swagger UI

This phase does not run destructive media conversions yet. It establishes the queue and worker state machine before enabling FFmpeg or HandBrake execution.

## Phase 4.5 Scope

Core configuration is now editable before enabling real conversion execution:

* Libraries can be edited after creation
* Conversion profiles can be edited after creation
* Settings has a real persisted configuration screen
* App settings include staging paths, worker defaults, and validation defaults
* Library types are configurable globally through a friendly editor and consumed by Libraries
* Settings includes software version visibility for runtime and dependency troubleshooting
* Profiles can be imported/exported as JSON
* Profiles show a dry-run worker command preview
* Profile creation now starts with guided, human-readable presets while keeping technical controls in Advanced
* Audio enhancement profiles can be edited for FFmpeg-based loudness normalization, dialogue clarity, and old source cleanup
* Swagger exposes update and settings endpoints for testing

## Phase 5 Scope

Workers can now complete a controlled dry-run execution from a claimed job:

* A running job can generate a planned FFmpeg command without modifying media files
* The planned output path is calculated from the job media path, destination library, and conversion profile
* Dry-run completion stores the command in the job notes and marks progress as complete
* The Workers UI exposes the dry-run action before real FFmpeg or HandBrake execution is enabled

## Phase 6 Scope

Queue jobs now support basic folder batches:

* Queue Folder creates individual jobs that share a common `batchId` and `batchName`
* Single-file jobs remain supported without a batch
* The Queue UI groups jobs by folder batch or single job
* Each group shows aggregate progress, completed count, queued count, running count, and failures
* Groups can be expanded to inspect the individual jobs created from that folder

## Phase 7 Scope

Validation and Publisher now provide the safety layer before real conversion execution:

* Completed jobs can be validated before publishing
* Validation stores status, score, warnings, and check results on the queue job
* Publisher only accepts completed jobs that passed validation or have validation warnings
* Publisher writes the final published path and timestamp to the job
* Overwrites are disabled by default
* Dry-runs do not pass file validation because they do not generate a real output file

This phase prepares the workflow for real conversion:

```text
Raw asset -> Queue -> Worker -> Staging output -> Validation -> Publisher -> Destination library
```

## Future Experimental AI Subtitle Translation

MediaForge should later explore an opt-in AI workflow for assets that do not include Spanish subtitles.

Planned phases:

* Detect missing Spanish subtitles, distinguishing Latin American Spanish and Castilian Spanish when possible
* Extract audio and generate timestamped source-language transcripts
* Translate dialogue naturally, preserving intent, tone, humor, and readability
* Provide a review workspace for editing text, timing, confidence warnings, and preview
* Export reviewed subtitles as `.srt` sidecars, optionally mux them into MKV, or publish/export them to Jellyfin or Plex when configured
* Store provenance such as model, language target, confidence, date, and reviewer

This feature must remain optional, reviewable, and safe by default.

## Future Pipeline Map / Stage Inspector

MediaForge should later show a visual stage map for every worker execution.

The map should display stages such as input path, preflight checks, probe/snapshot, advisor, profile resolution,
conversion plan, transcode/remux, output validation, publish/move, media-server refresh, and final path.

Each stage should expose status, worker name, duration, inputs, outputs, effective configuration, generated command,
logs, warnings, errors, retries, and generated artifacts.

This view should be available from Workers, Queue, History, and Assets, and should also support future AI subtitle
translation stages such as extract audio, transcribe, translate, review, export sidecar, mux, and publish to Jellyfin or Plex.

## Future Queue Management Controls

MediaForge should later add queue actions for canceling jobs, deleting/removing jobs, editing priority, moving jobs up or
down, retrying failed jobs, and requeueing completed jobs with confirmation.

For folder batches, the Queue should support cancel batch, delete batch, edit batch priority, retry failed jobs in batch,
remove one file from a batch, and add batch notes.

Canceling and cleanup should remain separate. Canceling preserves logs and diagnostics by default; cleanup should be an
explicit action and only remove artifacts inside MediaForge-controlled paths such as staging, work, temp, or registered
artifact paths.

## Future Episode Splitter

MediaForge should later include an optional splitter for series, anime, or DVD/Blu-ray sources where multiple episodes are
contained inside a single MKV.

The splitter should detect candidate multi-episode files, read chapters and stream metadata, suggest episode boundaries,
allow manual timestamp edits, preview split points, generate staged MKV outputs, validate each episode, and then allow the
user to queue conversion or publish the staged episodes.

Possible strategies include chapter-based split, manual timestamp split, duration-pattern split, and future AI-assisted
scene or intro/outro detection.

## Future Profile Lab / Comparison Lab

MediaForge should later add a Lab workspace for testing and comparing audio/video profiles before queueing full
conversions.

The Lab should support selecting a raw asset, choosing a start timestamp and sample duration, previewing the original
sample, previewing samples with selected video and audio profiles, comparing estimated size, codecs before/after,
container, bitrate, resolution, HDR, audio layout, language tracks, subtitles, screenshots/frames, and effective worker
commands.

When a result is good enough, the user should be able to save the tuned profile or fork an existing profile into a new
variant.

This can later become part of AI Copilot, where the assistant suggests profile changes, explains tradeoffs, and helps
tune difficult assets while keeping final approval with the user.

Profiles should also be designed for future community sharing through exportable/importable bundles with descriptions,
intended use cases, tool requirements, compatibility notes, versioning, provenance, and safety warnings. Shared profiles
should always be reviewed and tested before being enabled.

## Future Multimedia Library

MediaForge should later add a **Multimedia Library** workspace for browsing assets across registered filesystem libraries
without becoming a media server or metadata scraper.

The workspace should support library tabs or an equivalent library selector, filters by registered library and path,
search, technical media filters, and an aggregate "All libraries" view. External roots must be explicitly registered and
remain read-only during browsing and analysis.

Each asset should show whether MediaForge processed it and, when known, the MediaForge version, job, profile/profile
version, processing date, and source fingerprint. The database and job history are the authoritative provenance record.
MediaForge may also write portable embedded metadata when the output container supports it and a sidecar manifest when it
does not; provenance detection must not depend only on embedded tags because other tools can remove them.

The existing Advisor should be available from this workspace and support filtering or sorting by recommendation. Its
summary should explain whether conversion is worthwhile, estimated space saved or added, compatibility improvements,
quality or restoration opportunities, risks, confidence, and the recommended profile. Analysis remains non-destructive
and conversion always requires the normal queue/approval policy.

## Local Development

MediaForge is Docker-first. You do not need Go, Node.js, or npm installed on the host machine for the normal development path.

### Docker Compose

```sh
docker compose up --build
```

Frontend: `http://localhost:5173`

Backend: `http://localhost:8080`

Swagger UI: `http://localhost:8080/swagger/index.html`

OpenAPI JSON: `http://localhost:8080/openapi.json`

### Runtime Versions

The current development containers use:

* Backend: `golang:1.25-alpine` build image and `alpine:3.22` runtime
* Frontend: `node:22-alpine`

Frontend dependencies are pinned in `package.json` and locked in `package-lock.json`.

## API Endpoints

* `GET /health`
* `GET /openapi.json`
* `GET /swagger/index.html`
* `GET /api/libraries`
* `POST /api/libraries`
* `POST /api/libraries/:id`
* `GET /api/assets`
* `POST /api/advisor/evaluate`
* `GET /api/profiles`
* `POST /api/profiles`
* `POST /api/profiles/:id`
* `GET /api/queue/jobs`
* `POST /api/queue/jobs`
* `GET /api/settings`
* `POST /api/settings/:key`
* `GET /api/system/versions`
* `POST /api/workers/claim`
* `POST /api/workers/jobs/:id/dry-run`
* `POST /api/workers/jobs/:id/status`
* `POST /api/scan`
