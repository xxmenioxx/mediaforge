## 1. Harness and isolation

- [x] 1.1 Add a Compose harness with run-scoped labels, isolated SQLite/data volume, internal-only network, bounded containers, and no host mounts; verify rendered config rejects negative isolation fixtures.
- [x] 1.2 Add pre-start Docker inspection for source image, effective mounts, ports, networks, resources, SQLite path/write access, and owned cleanup; verify it stops before startup on unsafe resources.
- [x] 1.3 Build the backend from an exact-SHA source snapshot and test runner from the worktree; verify each image label and image ID before startup.
- [x] 1.4 Reuse the effective-container validator for the actual `docker compose run` test-runner and fail closed before tests on any mount, network, capability, security-option, or resource-limit mismatch.

## 2. Mock qualification workflows

- [x] 2.1 Add an Ollama mock with readiness, cold-start, structured chat, controlled negative modes, and content-free counters; verify only run-scoped mock traffic is accepted.
- [x] 2.2 Add tagged Translation SRT/ASS integration coverage for schema, IDs, ordering, timestamps, ASS structure, later-batch failure, retry, and cancellation; verify no partial output is returned.
- [x] 2.3 Add tagged OCR AI integration coverage for explicit model, dictionary provenance, suspicious cue bounds, mock admission, EnsureReady warmup, review suggestion, and unchanged canonical sidecar; run existing approval/invalidation/cancellation regressions.
- [x] 2.4 Add negative effective-inspection tests for one-off extra mounts/Docker socket, unbounded limits, unexpected capabilities, missing no-new-privileges, and unexpected/external networks.

## 3. Real-provider preparation and delivery

- [x] 3.1 Add a non-contacting real-plan interface requiring exact endpoint/model and refusing execution until fresh telemetry plus live Resource Admission are available; verify it never connects to the declared endpoint.
- [x] 3.2 Document architecture, safe commands, cleanup, remote-engine volume placement, and the NAS GO/NO-GO checklist; validate OpenSpec strict.
- [x] 3.3 Run local mock evidence and the complete repository verifier, then commit, push, and publish a Draft PR; record SHA, resource cleanup, PASS/FAIL/SKIPPED, and pending NAS requirements.
