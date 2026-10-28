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

Before any service starts, the harness renders Compose, builds the exact-SHA backend image, creates containers without starting them, and verifies effective image IDs/labels, mounts, SQLite path, network membership, ports, capabilities, devices, rootfs mode, and resource limits. It then initializes the unique volume and starts the mock and backend. Before executing tests, it creates the detached `docker compose run` one-off test-runner and sends that actual container through the same effective-container validator, including mount allowlist, private network, positive CPU/memory/PID limits, dropped capabilities, read-only rootfs, and `no-new-privileges`. Only after the runner passes inspection does it verify the temporary SQLite file and run:

- Tagged integration tests over `http://ollama-mock:11434` for SRT/ASS translation, structured cue validation, later-batch failure, retry, cancellation, OCR cold-start/warmup/admission, explicit model selection, and unchanged canonical sidecar.
- Existing focused OCR review/approval/invalidation/cancellation and Translation malformed-response/ASS/atomicity regressions.
- A negative TCP egress probe from the internal network.

The test-only mock accepts only the two synthetic model IDs `ocr-sandbox:latest` and `translate-sandbox:latest`. Its counters retain model names and route/concurrency counts, never prompts or subtitle text. The mock is not an Ollama accuracy benchmark and makes no claims about model quality.

The tagged integration tests call compiled production handler/provider code directly inside the test-runner. The isolated backend container is checked over HTTP at `/health`, but the tests do **not** exercise full HTTP route orchestration for OCR or Translation. This sandbox omits the frontend; the separate #287 workflow below covers browser journeys against a synthetic API fixture, not the Go API. Existing review/correction regressions cover approval and preserve the human-approval boundary.

## Issue #287 extended OCR/Translation/UI suite

`.github/workflows/extended-e2e.yml` is non-PR-gating and runs nightly at
04:35 UTC, on manual dispatch, and on pushes to `main`.

The OCR and Translation job runs the synthetic Ollama sandbox. Its tagged
integration tests execute MVForge Go handlers and provider code with a local
mock provider and synthetic subtitle cues. The Playwright job covers three
browser journeys: Assets to Queue/job status/Validation, Queue to Validation
Review, and Validation PASS to Publisher.

The browser journeys use `scripts/e2e/ui_fixture_backend.py`, a read-only
synthetic HTTP fixture. They verify frontend routing, rendering, and requests
against fixture responses; they do not run the Go API, SQLite, Queue workers,
or Publisher. OCR and Translation tests exercise handler/provider code but do
not exercise their full HTTP route orchestration. Real Ollama inference and
production data are excluded.

## Harness tests

Run the isolated-config negative tests without Docker:

```bash
python3 -m unittest discover -s scripts/e2e/ollama_validation -p 'test_*.py'
```

They reject production bind mounts, Docker socket, host networking, published ports, privileged/device access, external networks, excessive resources, real-provider endpoints in mock mode, production SQLite paths, and mismatched source contexts.

## Real-provider preflight (non-contacting, currently NO-GO)

The only real-provider command is a non-contacting preflight:

```bash
python3 scripts/e2e/ollama_validation_sandbox.py \
  --mode real-preflight \
  --endpoint 'http://<exact-ollama-host>:11434' \
  --model '<exact-installed-model>' \
  --workflow translation-ass \
  --source-sha '<full-lowercase-40-character-git-sha>' \
  --run-id '<12-to-32-character-lowercase-hex-id>'
```

The command validates only the declared target syntax and returns `BLOCKED`, `executionAllowed: false`, `providerContacted: false`, and `realInferenceExecuted: false`. The legacy `real-plan` spelling is an alias with the same behavior. It does not resolve or contact the endpoint, query any database, start a container, warm a model, or perform inference. It does not accept evidence files, Preview results, mock decisions, or caller-supplied admission booleans.

The backend now has a signed snapshot reader wired to production `airesource` admission. It reads a bounded JSON envelope, verifies an Ed25519 signature with a configured public key, checks the configured capacity domain and exact collector/host/provider/critical source IDs, and passes fresh RAM, swap, CPU, provider-residency, and critical-service observations to the existing #265 evaluator. In trusted-source mode, unsigned `PublishObservation` calls are rejected and any failed source refresh clears the cached observation. It does not contact Ollama, collect NAS telemetry, validate the truthfulness of a signer’s inputs, or prove endpoint identity.

The reader configuration is supplied only by explicit runtime environment settings: `MVFORGE_TRUSTED_OBSERVATION_PATH`, `MVFORGE_TRUSTED_OBSERVATION_PUBLIC_KEY_PATH`, `MVFORGE_TRUSTED_CAPACITY_DOMAIN`, `MVFORGE_TRUSTED_COLLECTOR_ID`, `MVFORGE_TRUSTED_HOST_SOURCE_ID`, `MVFORGE_TRUSTED_PROVIDER_SOURCE_ID`, and `MVFORGE_TRUSTED_CRITICAL_SOURCE_ID`. The snapshot payload’s exact embedded JSON bytes are signed with Ed25519; the application receives only the public key. A future host observer must independently collect RAM/swap and CPU counters, obtain residency through a read-only Ollama observation, and report Jellyfin/MVForge/FFmpeg/worker activity through least-privilege host signals. It must not receive a Docker socket, alter Ollama state, or access media libraries. No such NAS observer, key, or snapshot is installed or provisioned by this change.

The qualification CLI still has no verified provider-to-endpoint binding, no enforceable route restricted to the exact declared endpoint, no sandbox image/SHA proof, and no moment-of-run authorization. A signed observation or positive #265 lease alone cannot clear those separate gates. Therefore the preflight remains intentionally **NO-GO** and there is no `run-real` mode. No real endpoint or NAS Docker Engine was contacted.

## Cleanup and retained evidence

The harness verifies ownership labels before Compose removes its run-scoped resources. It removes unique image tags by inspected image ID and deletes its local temporary source snapshot only when its run marker still matches. It never runs `docker system prune`, `docker volume prune`, or any command against another Compose project. Docker BuildKit's shared build cache is left untouched and is reported as retained shared engine state.

The command prints a JSON summary with source SHA, checks, tests, image IDs, resource names, and cleanup status. A cleanup ownership mismatch leaves that resource in place and returns a failure so it can be inspected; it is never force-removed.

## NAS GO / NO-GO checklist

### GO conditions for a separately reviewed future implementation

- A reviewed source provides fresh, authenticated RAM, swap, CPU, actual capacity-domain, Ollama residency, and critical-service observations for the shared NAS; observation provenance and age are checked by the application.
- The source SHA is bound to the exact backend image and sandbox configuration that the qualification will execute.
- The isolated application obtains real positive #265 `MODEL_LOAD` and `INFERENCE` leases for the exact provider configuration, model, workflow, request identity, and capacity domain. `WAITING`, `DENIED`, `UNAVAILABLE`, Preview, and mock evidence remain NO-GO.
- A network boundary is tested to allow only the exact declared Ollama endpoint and to deny production networks, Docker engines, and arbitrary egress.
- A separate moment-of-run operator authorization is bound to exact endpoint, model, workflow, source SHA, and run ID; model install/residency is verified read-only without downloads, eviction, or daemon changes.
- The sandbox uses a unique Compose project, private network, engine-host temporary volume, isolated SQLite, synthetic fixtures, and inspected effective mounts/resources; there is no production library, staging, config, DB, or Publisher destination mounted.
- The canonical lifecycle enforces one total inference, purpose-scoped lease release, cancellation/timeout cleanup, unchanged Translation cue/ASS structure, human OCR review, and publication only within the temporary sandbox.
- Jellyfin, MVForge, FFmpeg, storage health, and available memory are clear immediately before and during the separately authorized run; cleanup is verified by run ID and ownership labels.

### NO-GO

Stop if any isolation check fails, a trusted signal is unavailable or stale, the capacity domain/provider/model is mismatched, either live admission is not granted, exact-endpoint egress cannot be proven, authorization is absent, concurrency cannot be held to one, critical services are active, storage is unhealthy, or cleanup ownership cannot be proven. Do not turn a mock PASS, Preview, or this preflight into a real-provider PASS. The current repository state satisfies NO-GO; the checklist is not executable authorization.
