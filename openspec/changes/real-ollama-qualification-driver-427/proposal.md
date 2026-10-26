## Why

The mock-only sandbox from #426 can exercise the OCR and Translation application paths, but it cannot safely qualify the shared NAS Ollama service. The repository has no authenticated source for all required NAS telemetry and no verified endpoint-restricted egress path, so a real qualification command must remain an explicit NO-GO until those authorities exist.

## What Changes

- Extend the sandbox CLI with a non-contacting real-provider preflight requiring exact endpoint, model, workflow, full source SHA, and run ID.
- Report missing trust, telemetry, Resource Admission, residency, and restricted-egress authorities as bounded BLOCKED reasons; never treat a plan or caller-supplied evidence as authorization.
- Add a read-only Ed25519-signed observation snapshot reader for RAM, swap, CPU, Ollama residency, and the critical-service guard; validate freshness, capacity domain, and configured source identities before feeding the existing #265 admission evaluator.
- Wire production route admission to that reader and reject unsigned published observations in trusted-source mode. Missing configuration or evidence remains UNAVAILABLE.
- Keep all real warmup and inference commands unavailable while the real authority and egress dependencies remain unresolved.
- Document the threat model, signed snapshot contract, provider/critical-service source design, current evidence gaps, future GO conditions, and safe operator workflow.
- Add negative tests while keeping the existing mock Compose qualification unchanged.

## Capabilities

### New Capabilities
- `ollama-real-qualification`: non-contacting, fail-closed qualification preflight for an exact future Ollama target.

### Modified Capabilities

None. The contract owned by #265 and the sandbox capability from #426 remain unchanged.

## Impact

- Affected code: `scripts/e2e/ollama_validation_sandbox.py`, its focused Python harness tests, `backend/internal/airesource`, production route wiring, and sandbox runbook documentation.
- Affected planning artifacts: this change's OpenSpec capability.
- No database schema, Compose production stack, external dependency, or runtime model behavior changes.
- The backend reader and admission integration are implemented, but no NAS snapshot publisher/key provisioning, real provider or critical-service observer, endpoint-restricted egress, sandbox image/SHA binding, or operator authorization is included. The command remains NO-GO rather than accepting synthetic or caller-supplied evidence.
