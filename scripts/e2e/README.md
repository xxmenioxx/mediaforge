# Synthetic pipeline smoke

Run one isolated scenario locally:

```bash
./scripts/e2e/run-pipeline-smoke.sh --scenario happy-path
./scripts/e2e/run-pipeline-smoke.sh --scenario track-actions
```

The runner uses `mvforge-backend:pipeline-smoke` when it is already present;
otherwise it builds that image from `backend/Dockerfile`. The `happy-path`
scenario generates a short MKV at runtime: progressive 24000/1001 H.264 video,
two deterministic AAC sine tracks, one SRT subtitle, and three chapters. Its
frozen Profile and Track snapshots explicitly keep both audio tracks, the
subtitle, and chapters. No binary media fixture is checked in.

The smoke verifies the exact stream and chapter counts at fixture, Result, and
published-package stages. It requires PASS for the artifact identity, manifest,
integrity, execution binding, duration, video properties, audio actions,
subtitle actions, and chapter actions Validation findings. The canonical
`publication.readiness` finding is NOT_APPLICABLE in the immutable ValidationRun
because Publisher performs readiness and destination checks afterward; the
smoke then requires an ALLOWED Validation handoff and a published Publisher
record bound to the same Result artifact and ValidationRun. It also compares
the Result and published file hashes.

The `track-actions` scenario uses the same deterministic two-audio, embedded
SRT, three-chapter source. Its frozen Track plan keeps audio stream 1, removes
audio stream 2, extracts subtitle stream 3 as required SRT and ASS sidecars, and
keeps chapters. It checks exact output audio identity and count, no embedded
subtitle or extra audio, valid sidecar text, exact Result manifest identities,
the `sidecars.package` Validation finding, and the Result chapter evidence. It
also temporarily removes the required ASS artifact to confirm Publisher blocks
the incomplete package, restores it, and retries the same publication to prove
that the complete package reaches the isolated Library.

Each scenario invocation gets a unique temporary root, SQLite database, and
filesystem. Add future scenarios to the scenario registry with their own
fixture expectations; keep them sequential in the existing smoke job so the
backend Docker image is built once and reused.

Each run creates a unique directory directly under the operating system temp
directory (`RUNNER_TEMP` in GitHub Actions). The backend container has no
network, a read-only image filesystem, dropped Linux capabilities, no devices,
and exactly one host bind: that run's temporary sandbox at `/media`. It runs as
the host UID/GID so the Linux CI runner can write that temporary bind without
root. SQLite,
the Library destination, staging, reports, home/cache/temp directories, and both
OCR roots live below that mount. Docker gets no checkout, Docker socket, NAS path,
or production media mount. A fresh SQLite database starts only the smoke job;
the real backend worker, FFmpeg, Validation, and Publisher operate on its
synthetic fixture.

The run writes an explicit `pipelineAutomation` setting into that temporary
SQLite database: review and worker execution are enabled, while auto-analysis,
auto-validation, and auto-publishing are disabled. The harness invokes
Validation and Publisher itself after verifying each prior stage. If QueueJob
does not finish, the bounded evidence includes its persisted ExecutionPlan and
review decision so a waiting policy is visible in CI.

The temporary runtime policy uses `desktop_safe` with zero minimum disk
reserves and permits direct workspace fallback. This keeps a synthetic run
independent of NAS detection and CI runner free-space reserves; all paths still
resolve below the one `/media` sandbox mount.

On success, the run container and temporary root are removed. On failure, the
container and sandbox are removed while bounded Result, ValidationRun,
ExecutionPlan, ffprobe, and log evidence remain under the printed `evidence/`
path. CI uploads only that directory on failure, then invokes
`cleanup-run-root.py` for its exact generated run ID. Media files are never
uploaded.

`generate-fixtures.sh` exposes the same guarded fixture generator for a known
run sandbox. New scenarios can be registered in `SUPPORTED_SCENARIOS` and added
to the workflow dispatch choice/matrix as their issues are implemented.

Issue #364 also adds a cadence-analysis fixture command for a multistream H.264
hard-telecine source. It uses the same image and temporary sandbox, with no
additional Docker build or separate Pipeline Smoke workflow:

```bash
python3 scripts/e2e/pipeline_smoke.py generate-cadence --sandbox "$MVFORGE_SMOKE_RUN_ROOT/sandbox"
```

The unit/integration regressions exercise its shared backend authorities using
synthetic scan evidence. #285 can consume this fixture in the existing smoke
sequence when it adds full output-fidelity checks; do not create another image
build for cadence scenarios.
