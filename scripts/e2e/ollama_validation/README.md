# Ollama qualification sandbox

This harness validates the MVForge #267 OCR AI and #422 Translation paths with synthetic subtitles and a local mock Ollama service. It builds the backend from the requested Git SHA, keeps application state in a unique Docker named volume, and removes only containers, networks, volumes, and images tagged for its run.

The harness does not deploy MVForge, clone `nas-media-stack`, read production SQLite, mount host files, access real media, or contact a real Ollama service. It publishes no host ports. Docker Engine stores the named volume on the engine host; if the selected engine is remote, no Mac temporary path is assumed or bind-mounted.

## Requirements

- Docker Engine with Compose v2 and BuildKit.
- Git history containing base SHA `8bc3a0c147d34350156e4daf499d989b73d3d179`.
- Python 3.9 or newer for the standard-library harness and its negative tests.
- Enough local builder capacity for the supported MVForge backend image. Runtime limits are 2 CPUs/2 GiB for the backend, 2 CPUs/4 GiB for tests, and 0.25 CPU/128 MiB for the mock.

## Run the mock qualification

Run from any directory in this worktree:

```bash
python3 scripts/e2e/ollama_validation_sandbox.py --mode mock --context desktop-linux
```

The selected Docker context is used exactly as named. Remote contexts are rejected unless `--allow-remote-engine` is explicitly supplied; the sandbox data is still a Docker-managed volume created on that engine host. This command is for the mock stage only. Do not pass a production Compose project or endpoint.

Before any service starts, the harness renders Compose, builds the exact-SHA backend image, creates containers without starting them, and verifies effective image IDs/labels, mounts, SQLite path, network membership, ports, capabilities, devices, rootfs mode, and resource limits. It then initializes the unique volume, starts the mock and backend, verifies the temporary SQLite file, and runs:

- Tagged integration tests over `http://ollama-mock:11434` for SRT/ASS translation, structured cue validation, later-batch failure, retry, cancellation, OCR cold-start/warmup/admission, explicit model selection, and unchanged canonical sidecar.
- Existing focused OCR review/approval/invalidation/cancellation and Translation malformed-response/ASS/atomicity regressions.
- A negative TCP egress probe from the internal network.

The test-only mock accepts only the two synthetic model IDs `ocr-sandbox:latest` and `translate-sandbox:latest`. Its counters retain model names and route/concurrency counts, never prompts or subtitle text. The mock is not an Ollama accuracy benchmark and makes no claims about model quality.

Frontend is omitted. The backend review/correction regressions exercise human approval and preserve the existing approval boundary; no UI route is needed for qualification of the provider/lifecycle contract.

## Harness tests

Run the isolated-config negative tests without Docker:

```bash
python3 -m unittest discover -s scripts/e2e/ollama_validation -p 'test_*.py'
```

They reject production bind mounts, Docker socket, host networking, published ports, privileged/device access, external networks, excessive resources, real-provider endpoints in mock mode, production SQLite paths, and mismatched source contexts.

## Real-provider preparation

The only implemented real-provider command is a non-contacting plan:

```bash
python3 scripts/e2e/ollama_validation_sandbox.py \
  --mode real-plan \
  --endpoint 'http://<exact-ollama-host>:11434' \
  --model '<exact-installed-model>'
```

It validates the declaration and emits `executionAllowed: false`; it does not resolve or contact the endpoint, query any database, start a container, warm a model, or perform inference. `actualHostTelemetry` and live Resource Admission remain pending and make the plan NO-GO. A mock or preview admission never authorizes a real request. This change does not provide an executable `run-real` command; enabling one requires a separate reviewed implementation that routes the sandbox application's normal OCR/Translation operations through live #265 admission, uses measured host telemetry, enforces one inference, and proves cancel/timeout cleanup. No real mode was run for this change.

## Cleanup and retained evidence

The harness verifies ownership labels before Compose removes its run-scoped resources. It removes unique image tags by inspected image ID and deletes its local temporary source snapshot only when its run marker still matches. It never runs `docker system prune`, `docker volume prune`, or any command against another Compose project. Docker BuildKit's shared build cache is left untouched and is reported as retained shared engine state.

The command prints a JSON summary with source SHA, checks, tests, image IDs, resource names, and cleanup status. A cleanup ownership mismatch leaves that resource in place and returns a failure so it can be inspected; it is never force-removed.

## NAS GO / NO-GO checklist

### GO only after a separately authorized real run has all of these

- Exact deployed MVForge SHA and supported image identity are independently confirmed.
- The sandbox uses a unique Compose project, private network, engine-host temporary volume, and isolated SQLite; effective mounts show no production library, staging, or config path.
- The exact Ollama endpoint and already-installed model are declared; no model download, daemon restart, or global Ollama setting change is needed.
- Fresh host CPU/RAM/swap telemetry is observed by the sandbox application, and a real positive #265 Resource Admission is recorded for the exact model and operation. A preview, mock, stale reading, or operator assertion does not count.
- The application enforces at most one active inference. The test uses only generated subtitle fixtures and has a verified cancel/timeout path.
- Jellyfin, MVForge, FFmpeg, storage health, and available memory show no critical workload or capacity conflict immediately before the run.
- No production SQLite, library, sidecar, staging path, or Publisher destination is mounted or writable from the sandbox.
- A restoration/cleanup plan lists only the sandbox project resources; cleanup is verified by run ID and labels.

### NO-GO

Stop if any isolation check fails, the Ollama endpoint/model is ambiguous or unavailable, telemetry is missing/stale, live admission is denied or unavailable, concurrency cannot be held to one, critical services are active, storage is unhealthy, or cleanup ownership cannot be proven. Do not turn a mock PASS into a real-provider PASS.
