## Why

Real Ollama qualification for OCR AI and Translation is blocked until the application can be exercised in a sandbox that cannot reach production storage or the production Compose project. The existing Pipeline Smoke constraints provide a useful safety baseline, but an isolated mock provider and auditable Docker resource boundary are still missing.

## What Changes

- Add a reproducible Compose harness with an exact-SHA MVForge backend, isolated SQLite/data volume, private mock Ollama, resource limits, and ownership labels.
- Add network integration coverage for SRT/ASS Translation and OCR AI cold-start, explicit model selection, admission, and non-mutating suggestions.
- Validate effective mounts, images, networks, ports, and resource limits before starting the application or running tests.
- Add a separate opt-in real-Ollama preflight contract that refuses to treat simulated admission as authorization for inference.
- Document safe commands, cleanup, evidence, and a NAS GO/NO-GO checklist.

## Capabilities

### New Capabilities
- `ollama-validation-sandbox`: Isolated, bounded, mock-backed qualification for OCR AI and Translation, with a separately gated real-provider preparation path.

### Modified Capabilities
- None.

## Impact

Adds scripts under `scripts/e2e/ollama_validation/`, a tagged backend integration test, and a new OpenSpec capability. It does not change production APIs, models, NAS configuration, or Ollama settings. The sandbox uses Docker-managed, run-scoped resources and synthetic subtitle fixtures only.
