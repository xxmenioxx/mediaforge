# Synthetic pipeline smoke

Run one isolated scenario locally:

```bash
./scripts/e2e/run-pipeline-smoke.sh --scenario happy-path
```

The runner uses `mvforge-backend:pipeline-smoke` when it is already present;
otherwise it builds that image from `backend/Dockerfile`. It generates a short
lavfi video fixture at runtime. The baseline is silent so it does not invent an
A/V timestamp relationship; it explicitly freezes the known progressive
cadence and track dispositions. No binary media fixture is checked in.

Each run creates a unique directory directly under the operating system temp
directory (`RUNNER_TEMP` in GitHub Actions). The backend container has no
network, a read-only image filesystem, dropped Linux capabilities, no devices,
and exactly one host bind: that run's temporary sandbox at `/media`. SQLite,
the Library destination, staging, reports, home/cache/temp directories, and both
OCR roots live below that mount. Docker gets no checkout, Docker socket, NAS path,
or production media mount. A fresh SQLite database starts only the smoke job;
the real backend worker, FFmpeg, Validation, and Publisher operate on its
synthetic fixture.

On success, the run container and temporary root are removed. On failure, the
container and sandbox are removed while bounded JSON summaries and logs remain
under the printed `evidence/` path. CI uploads only that directory on failure,
then invokes `cleanup-run-root.py` for its exact generated run ID. Media files
are never uploaded.

`generate-fixtures.sh` exposes the same guarded fixture generator for a known
run sandbox. New scenarios can be registered in `SUPPORTED_SCENARIOS` and added
to the workflow dispatch choice/matrix as their issues are implemented.
