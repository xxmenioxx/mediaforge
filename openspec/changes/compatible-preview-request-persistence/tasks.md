## 1. Durable model and migration

- [ ] 1.1 Add dedicated request-record and artifact-record models for persistent Compatible Preview recovery metadata, including indexed identities, lifecycle state, timestamps, expiration, source-revision evidence, controlled final path, and bounded recovery reason; verify the model contract is separate from runtime contexts, waiters, timers, and goroutines.
- [ ] 1.2 Add both models to the existing SQLite/GORM migration path without modifying or deleting existing preview cache files; verify a fresh and upgraded database migrate successfully and pre-feature databases contain no fabricated request records.
- [ ] 1.3 Add transactional persistence helpers for create/touch, artifact generation start, ready publication, failed/canceled completion, stale/interrupted recovery, expiration, and cleanup; verify concurrent updates do not use an AppSetting whole-map read/modify/write cycle and preserve request-to-shared-artifact references.

## 2. Runtime integration with #1 lifecycle

- [ ] 2.1 Persist non-ephemeral request identity and normalized canonical request metadata through the existing request registry while leaving ephemeral request scopes memory-only; verify repeated equivalent requests reuse the canonical request/artifact identity and ephemeral requests are not restored.
- [ ] 2.2 Persist the existing #1 artifact ID, encode signature, final cache path, retention policy, and source-revision evidence without introducing another identity algorithm or lifecycle engine; verify operation details and diagnostic verbosity do not fork artifact identity.
- [ ] 2.3 Connect durable transitions to the existing single-flight artifact manager, atomic final publication, bounded cancellation handoff, and terminal replacement rules; verify failed/canceled generations remain non-joinable and a retry starts only through normal #1 acquisition.

## 3. Startup reconciliation and safe paths

- [ ] 3.1 Add startup reconciliation after database migration and before preview routes serve, with bounded metadata paging and a reconciliation gate shared with cleanup; verify the application can start with no persisted rows and does not scan the media library or probe every preview.
- [ ] 3.2 Rebuild runtime ready entries only from unexpired records whose final path is under the canonical preview cache root, matches the artifact identity, and is a regular non-empty published file; verify zero-byte, partial, staging, log-only, unrelated, and out-of-root paths are rejected without arbitrary file access.
- [ ] 3.3 Normalize persisted `generating` records to ready only when the canonical final artifact is valid, otherwise to interrupted/recoverable without restoring a process owner; verify a dead generation is never joinable and a later retry can start a fresh generation.
- [ ] 3.4 Reconcile ready metadata whose artifact disappeared or changed source revision to stale/recovery state and preserve failed/canceled terminal records until expiration; verify startup never advertises a missing or invalid artifact as ready.

## 4. API and frontend recovery behavior

- [ ] 4.1 Add stable lifecycle response codes and retryability fields for unknown, expired, stale-artifact, interrupted, failed, and canceled request states across video, frame, inspect, and metrics resources; verify normal lifecycle conditions do not return generic 500 responses and successful ready responses retain existing media behavior.
- [ ] 4.2 Update frontend request polling/resource handling to keep valid IDs across restart, stop on explicit recovery/terminal responses, clear expired or unknown IDs, and offer recreate/retry for interrupted or stale states; verify the frontend does not depend on table/storage details or hide restart recovery as a timeout.
- [ ] 4.3 Define and test cancellation after restart for ready, interrupted, failed, canceled, and newly retried requests; verify no endpoint claims to cancel a dead FFmpeg process and new runtime cancellation remains owned by #1.

## 5. TTL and cleanup coordination

- [ ] 5.1 Apply the existing two-hour access-based request retention and bounded request count to durable rows using absolute expiration timestamps; verify expiration while the backend is stopped returns an explicit expired state and does not become permanent preview history.
- [ ] 5.2 Reconcile persistent artifact rows with the existing age/item/size cache cleanup and active consumer/generation protections; verify expiring one request does not delete a valid artifact still referenced by another request.
- [ ] 5.3 Make cleanup and publication crash-tolerant by recording safe state before deletion where possible and relying on startup reconciliation for filesystem/database mismatches; verify cleanup races cannot leave a ready advertisement for a missing file.

## 6. Focused regression coverage

- [ ] 6.1 Add backend restart tests using a persistent SQLite fixture and controlled preview root for ready reuse/no new encode, shared artifact reuse, persisted generating interruption, complete-file promotion, missing artifact, unsafe path, zero-byte/partial file, and source-revision mismatch.
- [ ] 6.2 Add backend lifecycle tests for failed/canceled deterministic responses, retry after terminal state, expiration across downtime, bounded cleanup, crash-window mismatches, and startup behavior with many metadata rows; verify each test exercises the real request/artifact entry points rather than only helper determinism.
- [ ] 6.3 Add frontend tests for ready ID continuity, explicit interrupted/stale retry UI, expired/unknown reset behavior, and cancellation semantics; verify existing request URL and successful media contracts remain unchanged.

## 7. Validation and handoff

- [ ] 7.1 Run focused backend/frontend tests for persistence, reconciliation, API recovery, cleanup, and path safety; verify all scenarios in the capability spec pass.
- [ ] 7.2 Run `openspec validate compatible-preview-request-persistence --type change --strict` and verify the change reports valid with proposal, design, spec, and task artifacts present.
- [ ] 7.3 Run `./scripts/verify.sh --auto` for the implementation change, inspect only reported failures, and verify `ALL CHECKS PASSED` before marking implementation tasks complete.
