## 1. Durable model, snapshots, and migration

- [ ] 1.1 Add dedicated request-record and artifact-record models for persistent Compatible Preview recovery metadata; include active/stale/expired-tombstone request state, frozen snapshot version/payload, artifact relationship, current generation token/version, generation state, timestamps, expiration, source-revision evidence, controlled final path, and bounded recovery reason; verify runtime contexts, waiters, timers, goroutines, process state, and unbounded logs are excluded.
- [ ] 1.2 Add both models to the existing SQLite/GORM migration path without modifying or deleting existing preview cache files; verify fresh and upgraded databases migrate successfully and pre-feature databases contain no fabricated request or tombstone records.
- [ ] 1.3 Add transactional persistence helpers for active create/touch, explicit recreate, expiration-to-tombstone, tombstone purge/eviction, artifact generation start, conditional terminal transitions, ready publication, stale/interrupted recovery, and cleanup; verify request and artifact lifecycle state is not duplicated and concurrent updates do not use an AppSetting whole-map read/modify/write cycle.

## 2. Frozen request-to-artifact integration

- [ ] 2.1 Persist the prepared descriptor snapshot at initial persistent request preparation with the minimum fields required by video, source/output frame, inspect, metrics, and existing API output: resolved source/revision, artifact/cache identity, start/duration/mode, normalization, requested/effective profiles, resolved video decision, requested/effective encoders and QSV rate control, and canonical FFmpeg arguments; verify the snapshot is bounded and versioned.
- [ ] 2.2 Restore active request entries from the frozen snapshot without re-resolving ProfileID, mutable profiles, or current policy when the source revision remains valid; verify a profile change after preparation does not change request R from artifact X to artifact Y across simulated restart.
- [ ] 2.3 Keep ephemeral request scopes memory-only and preserve #1's canonical request/artifact identity and shared single-flight ownership; verify equivalent persistent requests reuse the canonical artifact and ephemeral requests disappear after simulated restart.
- [ ] 2.4 Make explicit recreate/retry distinct from resource lookup; verify active equivalent creation preserves its frozen snapshot, expired tombstone creation resets lifecycle/timestamps and prepares current inputs, and stale/interrupted/failed/canceled recreate may replace the relationship.

## 3. Durable generation-attempt protection

- [ ] 3.1 Assign a new durable generation token/version before each artifact generation attempt, including replacement attempts for the same artifact ID; verify failed/canceled/interrupted attempt N remains terminal while explicit retry owns a distinct attempt N+1.
- [ ] 3.2 Make every generation-owned durable transition conditional on artifact ID plus expected generation token/version; verify with deterministic barriers/channels that a late completion from N cannot overwrite N+1, without relying on arbitrary sleeps.
- [ ] 3.3 Retain #1 runtime entry-object identity checks, bounded cancellation handoff, atomic same-directory final publication, and non-joinable terminal replacement; verify the durable token supplements runtime protection rather than creating a second lifecycle engine.

## 4. Startup reconciliation and safe paths

- [ ] 4.1 Add startup reconciliation after migration and before preview routes serve, enforcing the order: expire active rows to tombstones, purge bounded tombstones, load bounded active/tombstone/artifact metadata, validate frozen relationships and paths, reconcile generation tokens with disk, restore runtime entries, then publish routes; verify startup does not scan the media library or probe every preview.
- [ ] 4.2 Rebuild runtime ready entries only from active requests with valid versioned frozen snapshots and final artifacts under the canonical preview cache root whose filenames match the artifact identity and whose files are regular and non-empty; verify zero-byte, partial, staging, log-only, unrelated, and out-of-root paths are rejected without arbitrary file access.
- [ ] 4.3 Reconcile `generating` with the current generation token: promote the same token to ready when the final artifact is valid, otherwise mark that token interrupted without restoring a process owner; verify a later retry creates a new token and a resource GET never starts it implicitly.
- [ ] 4.4 Mark ready metadata with missing/invalid artifacts, unsupported snapshots, or changed source revisions stale/recoverable; verify the old artifact is never served and no mutable profile/policy re-resolution occurs during resource lookup.

## 5. API, frontend, and retry semantics

- [ ] 5.1 Add stable lifecycle response codes and retryability/recreate fields for unknown, expired, stale-artifact, interrupted, failed, and canceled states across video, frame, inspect, and metrics resources; verify ready responses retain existing media behavior and normal lifecycle states do not return generic 500 errors.
- [ ] 5.2 Implement deterministic expired-vs-unknown behavior: active expiration returns a tombstone-backed `410 expired`, tombstone purge returns `404 unknown_request`, and resource lookup never revives a tombstone; verify the transition across restart and cleanup.
- [ ] 5.3 Update explicit create/recreate handling so the same normalized request can reuse its deterministic ID after expiration, reset active timestamps/state, and resolve current profile/source configuration; verify explicit recreate after profile or source changes may reference a new artifact identity while the old frozen relationship is not silently mutated.
- [ ] 5.4 Update frontend request polling/resource handling to keep valid IDs across restart, stop on explicit recovery/terminal responses, clear expired/unknown IDs, and offer recreate/retry for interrupted or stale states; verify the frontend does not depend on table/storage details or hide restart recovery as a timeout.
- [ ] 5.5 Define and test cancellation after restart for ready, interrupted, failed, canceled, and newly retried requests; verify no endpoint claims to cancel a dead FFmpeg process and new runtime cancellation remains owned by #1.

## 6. TTL, tombstone, and cleanup coordination

- [ ] 6.1 Apply the existing two-hour access-based active-request retention and 500 active-record bound, transition expiry/active-capacity eviction to tombstones, and enforce two-hour/500 tombstone retention; verify tombstone purge or capacity eviction makes later lookup unknown without retaining historical audit data.
- [ ] 6.2 Reconcile persistent artifact rows with existing age/item/size cache cleanup and active consumer/generation protections; verify expiring one request does not delete a valid artifact referenced by another active request and tombstones do not keep artifacts alive indefinitely.
- [ ] 6.3 Make cleanup and publication crash-tolerant by recording safe state before deletion where possible and relying on token-aware startup reconciliation for filesystem/database mismatches; verify cleanup races cannot leave a ready advertisement for a missing file.

## 7. Focused regression coverage

- [ ] 7.1 Add backend tests using a persistent SQLite fixture and controlled preview root for ready reuse/no new encode, shared artifact reuse, complete-file promotion, missing/unsafe/zero-byte/partial artifacts, and bounded startup.
- [ ] 7.2 Add expired-vs-unknown tests: request expiration returns `410 expired` during tombstone retention, purge returns `404 unknown_request`, and POST creation of the same deterministic normalized request reactivates the ID with a fresh lifecycle.
- [ ] 7.3 Add deterministic generation-race tests where generation N becomes terminal/replaced, N+1 starts, and a late durable completion from N is rejected while N+1 remains intact; use channels/barriers or injected synchronization, never arbitrary sleeps.
- [ ] 7.4 Add frozen-relationship tests where ProfileID 7 resolves to artifact X, profile 7 changes, registries restart with unchanged source revision, and request R still restores X; then verify expiration plus explicit recreate may resolve current profile configuration to artifact Y.
- [ ] 7.5 Add source-revision tests where R references X/S1, the source changes to S2, restart/use returns stale without serving X, and explicit recreate may establish a current relationship.
- [ ] 7.6 Add backend lifecycle tests for interrupted, failed, canceled, explicit retry, shared references, cleanup races, and crash-window mismatches; verify each test exercises real request/artifact entry points rather than only helper determinism.
- [ ] 7.7 Add frontend tests for ready ID continuity, explicit interrupted/stale retry UI, expired/unknown reset behavior, recreate after expiration, and cancellation semantics; verify existing request URL and successful media contracts remain unchanged.

## 8. Validation and handoff

- [ ] 8.1 Run focused backend/frontend tests for persistence, tombstones, frozen snapshots, generation-token protection, reconciliation, API recovery, cleanup, and path safety; verify all scenarios in the capability spec pass.
- [ ] 8.2 Run `openspec validate compatible-preview-request-persistence --type change --strict` and verify the change reports valid with proposal, design, spec, and task artifacts present.
- [ ] 8.3 Run `./scripts/verify.sh --auto` for the implementation change, inspect only reported failures, and verify `ALL CHECKS PASSED` before marking implementation tasks complete.
