## 1. Admission contract and authority boundaries

- [ ] 1.1 Add the provider-neutral admission contract with `MODEL_LOAD` and `INFERENCE` purposes, four decision states, opaque lease/wait handles, and stable request identity; verify contract tests cover every status and a package dependency check proves no lifecycle or provider-transport import.
- [ ] 1.2 Define injected host, resident-model, estimate, and critical-service observation interfaces; verify fakes can exercise missing, stale, healthy, and pressure states without provider calls or duplicate host-state storage.

## 2. Policy, estimates, and capacity evaluation

- [ ] 2.1 Persist validated AI resource settings with defaults of one resident, one inference, one model load, protection enabled, a five-minute admission wait, and the provisional configurable 6 GiB reserve; verify settings initialization, updates, invalid values, and upgrades preserve unrelated settings.
- [ ] 2.2 Add provider-neutral per-model resource profiles for resident footprint, load peak, and context-sensitive per-inference/KV-cache demand with source and revision; verify unknown or incompatible estimates return `UNAVAILABLE` and never become zero-cost estimates.
- [ ] 2.3 Implement dry-run and live capacity evaluation over fresh telemetry, configured limits, resident models (including external residents), estimates, and the #256 critical-service guard; verify `ADMITTED`, `WAITING`, `DENIED`, and `UNAVAILABLE` reasons without acquiring a lease in preview mode.

## 3. Fair waits and recoverable leases

- [ ] 3.1 Add the SQLite admission journal for stable request identities, wait order/deadlines, purpose-scoped leases, expiry, and release state; verify duplicate requests reacquire the same logical lease and duplicate release cannot free another request's capacity.
- [ ] 3.2 Implement bounded FIFO wait lanes for model loads and inference with fair turn-taking, event-driven reevaluation, cancellation, and five-minute default expiry; verify fairness, cancellation/grant races, expiry, and that no Queue or FFmpeg worker is held while waiting.
- [ ] 3.3 Reconcile journaled leases and provider residents on restart before issuing grants; verify stale/unavailable telemetry blocks grants, active inference reservations survive restart through deadline plus cleanup grace, and lifecycle recovery reattaches without duplicate reservations.

## 4. Consumer and Settings integration

- [ ] 4.1 Wire canonical `RuntimeState` host evidence, the #261/#264 provider resident census, and the #256 critical-service decision into the injected observation boundary; verify missing required sources fail closed without an AI-specific poller or competing service-priority policy.
- [ ] 4.2 Integrate the #264 lifecycle port so every warmup and every inference, including a resident model, requires the matching positive lease; verify unavailable, waiting, denied, cancellation, recovery, and release outcomes issue no unauthorized `/api/generate` or `/api/chat` request.
- [ ] 4.3 Add Resource Protection controls, capacity preview, counters, and bounded diagnostics to Settings; verify MVForge policy is displayed separately from actual Ollama server values, unknown values stay unknown, and saving policy never changes Docker or requests an Ollama restart.

## 5. NAS regressions and validation

- [ ] 5.1 Add realistic 24 GiB fixtures for defaults, 6 GiB reserve, model-load headroom, in-flight inference, Jellyfin/critical-service pressure, external resident models, missing estimates, and stale/unknown memory; verify each expected decision and absence of model-affecting work.
- [ ] 5.2 Add a 64 GiB/two-resident fixture plus independent inference-limit, model-load-limit, fairness, cancellation, lease release, and restart-recovery regressions; verify higher configured residency is admitted only when the same headroom and critical-service checks pass.
- [ ] 5.3 Calibrate the provisional 6 GiB reserve and model estimates against #256 NAS measurements, then run `openspec validate ai-resource-protection --type change --strict` and `./scripts/verify.sh --auto`; record the measured policy basis and require the verifier's final `ALL CHECKS PASSED` result.
