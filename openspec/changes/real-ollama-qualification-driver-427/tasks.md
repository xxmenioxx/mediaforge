## 1. Threat model and source audit

- [x] 1.1 Audit #426 sandbox isolation, `airesource` evidence/admission contracts, and model lifecycle ownership; record missing authentic NAS telemetry and endpoint-restricted egress in `design.md`.
- [x] 1.2 Define explicit NO-GO conditions and source/admission/network trust boundaries in the new OpenSpec capability.

## 2. Non-contacting preflight

- [x] 2.1 Require exact endpoint, model, workflow, full source SHA, and run ID for `real-preflight`/`real-plan`; return bounded `BLOCKED` reasons and never authorize execution.
- [x] 2.2 Add negative tests for missing/ambiguous identity, unavailable telemetry and critical-service authority, unavailable live admission, unverified egress, absent authorization, and unsupported real execution; prove no provider connection is attempted.
- [x] 2.3 Update the sandbox runbook with the current NO-GO, exact preflight command, future GO conditions, and no-run limitation.

## 3. Validation and delivery

- [x] 3.1 Run focused preflight tests and the existing isolated mock sandbox; confirm no real endpoint, NAS Docker Engine, production database, or filesystem is accessed.
- [x] 3.2 Validate OpenSpec strict, then run `git diff --check` and `./scripts/verify.sh --full`.
