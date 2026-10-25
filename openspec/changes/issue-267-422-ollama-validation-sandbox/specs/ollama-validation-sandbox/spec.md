## Purpose

Provides a reproducible and auditable isolation boundary for qualifying MVForge OCR AI and Translation workflows with synthetic fixtures and a mock Ollama provider before any separately authorized real-provider run.

## ADDED Requirements

### Requirement: Mock qualification is isolated from production resources
The harness MUST run the requested MVForge source snapshot with temporary SQLite and data storage in run-scoped Docker resources. It MUST reject unexpected mounts, image identity, network membership, host exposure, or resource configuration before starting application operations.

#### Scenario: Valid isolated mock run
- **WHEN** the rendered Compose project has the expected run labels, exact source SHA, single run-scoped data volume, internal-only network, and bounded containers
- **THEN** the harness starts only its own resources and directs all provider traffic to the Ollama mock

#### Scenario: Unsafe resource configuration
- **WHEN** any container has a production path, Docker socket, host network, unexpected published port, privileged mode, device, unowned mount, or mismatched image/source label
- **THEN** the harness stops before application startup or test execution and preserves resources it cannot prove it owns

### Requirement: Mock provider workflows preserve canonical contracts
The mock-backed tests MUST exercise the production Translation provider and OCR AI lifecycle with bounded synthetic data. OCR suggestions MUST remain separate from canonical subtitle content until human approval; translation MUST reject malformed or incomplete batches without returning partial outputs.

#### Scenario: SRT and ASS Translation over the mock network
- **WHEN** synthetic English SRT and ASS cues are translated through the Ollama provider using the sandbox mock
- **THEN** the structured cue contract is validated and original cue order, timestamps, source bytes, and protected ASS structure are preserved

#### Scenario: OCR AI cold start over the mock network
- **WHEN** OCR AI uses its explicitly selected model, the model is not resident, and mock admission grants readiness and inference
- **THEN** the lifecycle performs bounded warmup before inference, records the OCR AI consumer, and returns a revision-bound suggestion without changing canonical OCR sidecars

#### Scenario: Invalid provider output or cancellation
- **WHEN** the mock returns malformed, incomplete, delayed, or canceled output
- **THEN** MVForge rejects it, returns no partial translated artifact or automatic OCR correction, and releases test admission resources

### Requirement: Real Ollama planning never authorizes inference
The harness MUST require an exact endpoint and model for a real-provider plan and MUST report execution as disallowed until a separate reviewed driver can observe fresh host telemetry and positive live Resource Admission from the sandbox application. Simulated admission, previews, or caller-supplied assertions MUST NOT authorize inference.

#### Scenario: Real plan without execution
- **WHEN** an operator supplies the endpoint and model to the real-plan command
- **THEN** the harness validates and reports a sanitized plan without contacting the endpoint, reading production SQLite/configuration, or executing inference

#### Scenario: Real plan without live admission
- **WHEN** the real plan is requested without live telemetry and Resource Admission evidence
- **THEN** it returns `executionAllowed: false`, performs no model request, and leaves production models, settings, databases, and media untouched
