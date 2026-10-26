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

## 4. Trusted observation and canonical admission increment

- [x] 4.1 Add a bounded read-only Ed25519 snapshot reader with exact collector/component identities, capacity-domain binding, schema/value checks, and separately timestamped RAM, swap, CPU, provider residency, and critical-service observations.
- [x] 4.2 Wire production routes to the trusted source and existing #265 evaluator; reject unsigned observations and evict cached evidence on source failure without changing lifecycle ownership.
- [x] 4.3 Document host telemetry collection, Ollama residency, critical-service guard, key custody, and exact-endpoint egress as distinct trust boundaries; keep `real-preflight` NO-GO.
- [x] 4.4 Add negative tests for stale RAM/CPU/swap/provider/guard data, forged signatures, wrong identities/domain, invalid values, unavailable files, unsigned publication, and no lease/fallback.
- [x] 4.5 Run focused Go tests, OpenSpec strict, `git diff --check`, and `./scripts/verify.sh --auto`; confirm no production/NAS access or real inference.

## 5. Fixture-only Linux observer core

- [x] 5.1 Implement bounded parsers for Linux RAM/swap and two-sample CPU counters, with negative synthetic fixtures.
- [x] 5.2 Decode pre-supplied Ollama `/api/ps` JSON without network contact, and return `unavailable` for unknown critical service evidence.
- [x] 5.3 Serialize the #431 Ed25519 envelope using a supplied private key, without implementing key custody, file publication, or a daemon.
- [x] 5.4 Document that no host attestation, production service probes, provisioned signer or real Ollama qualification is provided.
- [x] 5.5 Run focused Go tests, strict OpenSpec and canonical verifier, and confirm remote CI/Smoke PASS before considering integration.

## 6. Attributed guard and atomic handoff (synthetic only)

- [x] 6.1 Add independently attributed, freshness-checked critical-service evidence with fail-closed semantics.
- [x] 6.2 Add conservative parsing of bounded Jellyfin session JSON supplied via fixture, with no network client or live attribution.
- [x] 6.3 Add temporary-file + fsync + rename publication for already signed snapshots, without keys or provisioned service.
- [x] 6.4 Add synthetic negative tests for unknown/stale identity, active playback, malformed Jellyfin JSON, and destination safety.
- [x] 6.5 Document missing source authority, session coverage, secure directory/key custody, replay controls, and real-preflight NO-GO.
- [ ] 6.6 Complete focused Go tests, OpenSpec strict, verify.sh --auto and green CI/Smoke on the final SHA.
