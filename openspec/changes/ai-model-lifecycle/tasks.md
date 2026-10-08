## 1. Extend the shared Ollama provider seam

- [ ] 1.1 Add bounded `/api/ps` discovery and no-prompt `/api/generate` warmup to the #261 provider adapter, preserving its endpoint authority, timeouts, body limits, cancellation, and sanitized failures; verify successful, missing-model, malformed, oversized, offline, and cancelled requests with focused adapter tests.
- [ ] 1.2 Carry the configured `keep_alive` value on Translation `/api/chat` and every other inference request while leaving prompt construction with each feature; verify request bodies and that warmup never contains source text.

## 2. Implement lifecycle coordination

- [ ] 2.1 Implement the seven lifecycle states as a projection of `/api/ps`, bounded warmup results, and MVForge operation leases; verify installed, loaded, ready, idle, in-use, failed, and unavailable cases without a second endpoint or model catalog.
- [ ] 2.2 Add singleflight keyed by shared provider identity and exact model ID, independent waiter cancellation, final-waiter cancellation, five-minute default readiness deadline, bounded backoff, and typed retryable failures; verify concurrent callers share one warmup and cancellation cannot resume stale work.
- [ ] 2.3 Add the lifecycle settings contract with on-demand warmup default, 30-minute keep-alive default, bounded timeout choices, and finite retry/fail policy; adapt to #265 admission decisions without adding local capacity thresholds and verify denied admission makes no Ollama warmup request.

## 3. Persist model waits and integrate Queue scheduling

- [ ] 3.1 Add a bounded frozen model-wait checkpoint to the existing Queue plan for exact model identity, source revision, AI stage, deadline, and cancellation/recovery state; verify additive SQLite migration and recovery against pre-change databases.
- [ ] 3.2 Enter durable `WAITING_FOR_MODEL` only when other Queue gates are clear, keep the job out of active FFmpeg reservations, and restore it through normal claim after revalidating source/model/cancellation; verify a waiting job does not block another runnable job and stale or cancelled work does not resume.
- [ ] 3.3 Checkpoint completed Queue AI stages and release any worker reservation before a model wait; verify resume continues from the saved stage without replaying completed processing or duplicating output.

## 4. Integrate Translation with its existing operation lifecycle

- [ ] 4.1 Persist only bounded Translation wait metadata and operation identity in the existing operation lifecycle, including durable cancellation and publication-commit evidence; verify old operation data remains readable and no prompts, cues, or provider responses are stored.
- [ ] 4.2 Report waiting/warming progress and resume the same operation only after source revision, exact selected model, and cancellation checks; verify restart recovery before inference, safe handling of uncertain in-flight inference, and no duplicate or stale publication.

## 5. Expose runtime state and preserve owner boundaries

- [ ] 5.1 Add `GET /api/settings/ollama/runtime` using #261 installed-model data, bounded `/api/ps` residency/expiry, and MVForge operation tracking; verify an unavailable provider is not reported as an empty catalog and status polling stays bounded.
- [ ] 5.2 Add contract tests proving #223 role/fallback selection and #266 feature selection pass an exact model identity to the lifecycle manager, and #265 alone decides resource admission; verify the manager does not choose models or contact Ollama from the frontend.

## 6. Verify the integrated behavior

- [ ] 6.1 Add focused regressions for timeout, offline provider, model missing, cancellation races, retry bounds, idle expiry, restart recovery, and no automatic downloads, forced eviction, or provider restart; verify each failure remains actionable and does not duplicate provider work or publication.
- [ ] 6.2 Run `./scripts/verify.sh --auto` after implementation and require its final `ALL CHECKS PASSED` result; record any environmental limitation without claiming full validation.
