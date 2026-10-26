## Why

The mock-only sandbox from #426 can exercise the OCR and Translation application paths, but it cannot safely qualify the shared NAS Ollama service. The repository has no authenticated source for all required NAS telemetry and no verified endpoint-restricted egress path, so a real qualification command must remain an explicit NO-GO until those authorities exist.

## What Changes

- Extend the sandbox CLI with a non-contacting real-provider preflight requiring exact endpoint, model, workflow, full source SHA, and run ID.
- Report missing trust, telemetry, Resource Admission, residency, and restricted-egress authorities as bounded BLOCKED reasons; never treat a plan or caller-supplied evidence as authorization.
- Keep all real warmup and inference commands unavailable while the real authority and egress dependencies remain unresolved.
- Document the threat model, current evidence gaps, future GO conditions, and safe operator workflow.
- Add negative tests while keeping the existing mock Compose qualification unchanged.

## Capabilities

### New Capabilities
- `ollama-real-qualification`: non-contacting, fail-closed qualification preflight for an exact future Ollama target.

### Modified Capabilities

None. The contract owned by #265 and the sandbox capability from #426 remain unchanged.

## Impact

- Affected code: `scripts/e2e/ollama_validation_sandbox.py`, its focused Python harness tests, and sandbox runbook documentation.
- Affected planning artifacts: this change's OpenSpec capability.
- No backend admission policy, provider lifecycle, database schema, Compose production stack, external dependency, or runtime model behavior changes.
- Real telemetry and endpoint egress remain unavailable; the command will report NO-GO rather than fabricate or import those signals.
