# Synthetic pipeline smoke

Run one isolated scenario locally:

```bash
./scripts/e2e/run-pipeline-smoke.sh --scenario happy-path
./scripts/e2e/run-pipeline-smoke.sh --scenario track-actions
./scripts/e2e/run-pipeline-smoke.sh --scenario language-normalization
./scripts/e2e/run-pipeline-smoke.sh --scenario language-normalization-exclusions
./scripts/e2e/run-pipeline-smoke.sh --scenario cadence-progressive
./scripts/e2e/run-pipeline-smoke.sh --scenario cadence-ivtc
./scripts/e2e/run-pipeline-smoke.sh --scenario cadence-ambiguous
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

The `language-normalization` scenario generates four AAC streams tagged
`ko`, `kor`, `ko-KR`, and `eng`, plus four SRT streams tagged `fr`, `fra`,
`fre`, and `fr-FR`. The path-scoped Track Profile requests the Korean and
French aliases by language without stream indexes. The smoke verifies every
source index and original alias with ffprobe, freezes all three Korean streams
as canonical `kor` and all four French streams as canonical `fra`, and checks
the encoded audio aliases and eight index-bound SRT/ASS sidecars through Result,
Validation, and isolated Publisher. It also confirms the original fixture hash
is unchanged when archived.

The `language-normalization-exclusions` scenario uses the same aliases and
language defaults, then saves explicit Asset Tracks selection `[1, 3]`. Queue
must freeze those two Korean streams and exclude Korean stream 2 plus English
stream 4. It runs the same FFmpeg, Result, Validation, and Publisher checks,
including the French sidecar identities. Both scenarios use generated media,
databases, and filesystem state inside the temporary sandbox; no binary media
fixture is checked in.

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

Issue #364 added a cadence-analysis fixture command for a multistream H.264
hard-telecine source. Issue #285 extends the same isolated runner with three
full synthetic scenarios, with no additional Docker image or separate workflow:

```bash
python3 scripts/e2e/pipeline_smoke.py generate-cadence --sandbox "$MVFORGE_SMOKE_RUN_ROOT/sandbox"
```

`cadence-progressive` verifies that automatic cadence resolution freezes and
preserves 24000/1001. `cadence-ivtc` generates a CPU-only 29.97 hard-telecine
source and verifies the frozen IVTC field order/filter chain, 24000/1001 Result,
frame count, and Frame Fidelity. `cadence-ambiguous` concatenates telecined and
native synthetic sections and verifies the expected claim-time rejection. The
current Queue API represents it as `status=failed`, `stage=failed`, and an
`errorMessage`; it has no typed rejection-code field. The backend also emits the
structured `auto_worker_claim_failed` event. The smoke counts this negative
scenario as PASS only when the error says effective output FPS is unknown and
cadence review is required, and when persisted state proves no executable
snapshot, ExecutionPlan, worker output, Result, conversion command, or Publisher
record was created. The fixture generator uses FFmpeg to create the synthetic
input; the Queue worker must not launch a conversion for this scenario.
The observed `executionNumber=1` and `queued → claimed → failed` history record
the worker's claim attempt, not a conversion execution.

Review required during cadence resolution is a pre-Queue decision: it prevents
an expected FPS from being guessed. `UNVERIFIED/REVIEW_REQUIRED` is a
post-execution Validation/Frame Fidelity verdict that requires a completed job
and canonical Result. For C, no `ValidationRun` is started and the harness
reports `NOT_RUN_BY_DESIGN`; it does not manufacture a Validation verdict.
Unexpected Queue states, missing review evidence, execution artifacts, FFmpeg
conversion, publication, or ValidationRun fail the smoke.

**Proposed clarification for issue #285 (not applied to the issue):**

> For unresolved cadence rejected during claim-time Queue preflight, pass the
> negative smoke only when the job records the explicit unknown-FPS/review
> reason and no executable snapshot, ExecutionPlan, worker conversion, Result,
> or publication exists. ValidationRun is `NOT_RUN` because Validation requires
> a completed job and canonical Result. Reserve
> `UNVERIFIED/REVIEW_REQUIRED` for post-execution Validation results.

The progressive and IVTC checks bind Queue snapshot, active ExecutionPlan,
executed command in QueueJob notes, Result ffprobe evidence, and Validation
report. All generated media stays under that run's validated temporary root;
no GPU or NAS path is used.
