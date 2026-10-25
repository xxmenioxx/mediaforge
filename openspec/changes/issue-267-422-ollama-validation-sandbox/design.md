## Context

Issue #267 and #422 require real-provider evidence, but the first qualification stage must validate application behavior without contacting an Ollama service or touching production state. `scripts/e2e/pipeline_smoke.py` already establishes bounded temp storage, synthetic data, and restricted containers. This change applies those constraints to Compose while allowing only the backend, test runner, and mock Ollama to communicate on one internal network.

## Design

### Ownership and source identity

- The backend image is built from a temporary source snapshot created by `git archive` at the requested base SHA. The snapshot is independent of uncommitted harness changes.
- A separate Go test-runner image is built from the current worktree and contains the tagged integration test. It cannot access the Docker socket.
- Image, container, network, and volume labels include the run ID, sandbox owner, and source SHA. Cleanup checks those labels before removing anything.

### Docker boundary

- Compose uses a unique project name, one private network with `internal: true`, and one run-scoped Docker-managed volume. A Docker-managed volume places the temporary directories on the selected Docker Engine host, so remote engines do not receive invalid client-side bind paths.
- The backend and test runner mount only the unique volume at `/media`. SQLite, XDG, OCR, media, staging, raw, reports, cache, sidecars, work, and temp paths live under that mount. The Ollama mock has no volume.
- Containers run with read-only root filesystems, `no-new-privileges`, all capabilities dropped, no devices, no host networking, no published ports, bounded CPU/memory/PIDs, and no privileged mode. API port 8080 is reachable only by peers on the private network.
- The harness validates rendered Compose configuration first, then builds images and creates containers without starting them. It inspects effective mounts, image IDs/labels, volume/network labels, and resource limits before starting the volume initializer, mock, or backend.
- The backend health endpoint is checked from the test runner. Test traffic uses only `http://ollama-mock:11434`; the internal network has no external route. Any explicitly supplied forbidden endpoint is probed from the sandbox and must be unreachable.
- Cleanup is restricted to the unique Compose project and resources bearing the expected run labels. It never invokes broad Docker pruning or accesses another Compose project.

### Mock-backed workflows

- The mock implements bounded Ollama readiness, installed/resident model, warmup, chat, and test-admin endpoints. It records route counts, model names, and concurrency only; it never retains prompts or subtitle text.
- Translation integration tests call the production `ollamaTranslationProvider` and canonical text translation executor for synthetic SRT/ASS, including structured output, cue IDs/order, timestamps, and protected ASS tags. Negative modes cover malformed output, a later batch failure, retry, and cancellation. Existing focused unit tests remain responsible for diagnostics categories and atomic publication boundaries.
- OCR integration tests use a temporary SQLite database with an explicit OCR model distinct from the Translation model, a synthetic canonical OCR fixture with dictionary provenance, a cold model, test admission, the production `EnsureReady`/`acquireInference` lifecycle, and the production proofreading provider. They verify warmup then inference, the OCR consumer identity, bounded suspicious cues, review suggestion identity, and unchanged canonical sidecar/provenance. Existing API/review tests cover human approval and invalidation; the harness runs those tests as part of the focused validation.
- The mock admission implementation is test-only. It is evidence of application integration behavior and cannot authorize real inference.

### Real-provider preparation

- A separate `real-plan` command requires an exact HTTP(S) endpoint and model name and emits a sanitized plan only. It does not contact Ollama, read any MVForge database, or execute a model operation.
- The command always emits `executionAllowed: false`, marks live telemetry and admission as pending, and never resolves or contacts the declared host. No `run-real` command is enabled by this change.
- A future execution driver requires explicit operator confirmation, exact endpoint/model values, fresh host telemetry, and an actual positive Resource Admission result from the sandbox application's #265 operation path. A mock/preview decision or a caller-supplied boolean is rejected. It must enforce one in-flight inference and cancellation/timeout cleanup over the isolated SQLite/volume and synthetic fixtures.
- The NAS checklist remains NO-GO until the sandbox application can observe live telemetry and a real admission grant. A prepared plan alone is not authorization.

## Resource and compatibility impact

Default mock limits are 2 CPUs/2 GiB for the backend, 2 CPUs/4 GiB for the Go test runner, and 0.25 CPU/128 MiB for the mock, each with bounded PIDs. There are no model downloads, external inference calls, production mounts, persistent database changes, or application API changes. The backend build is based on the requested source SHA; test-only files are excluded from that image.

## Failure behavior

Any unexpected mount, image label/SHA, network membership, published port, resource limit, production path, or cleanup ownership mismatch stops the run before application operations. A mock response failure is a test failure unless the specific negative test asserts that rejection. A real-provider plan never reports GO from simulated admission.

## Open Questions

None. Real execution remains opt-in and is deliberately not performed by this change.
