## 0. Implementation branch prerequisite

- [ ] 0.1 Before any production implementation, merge current
  `origin/main` at `d05a86b5af8e9c74365e4f36c96e5500fbeeb1c4` into the
  implementation branch or create a fresh v2 implementation branch from that
  main and carry this approved OpenSpec forward; verify the implementation
  branch is zero commits behind main, preserve merged #3 Frame Fidelity/Profile
  LAB changes, and do not rewrite or force-push the published #15 branch.

## 1. Backend lifecycle and HTTP contract

- [ ] 1.1 Add typed status/action envelopes and route contracts for `idle`,
  `generating`, `ready`, `failed`, `canceled`, `interrupted`, `stale_artifact`,
  `expired`, and `unknown_request`; verify `/status` uses the exact 200/410/404
  projection while resource/start actions use 202/200/409/410/404.
- [ ] 1.2 Add runtime-only `ownerId`/lease bookkeeping keyed by
  `(requestId, ownerId)` with one active reference, 45-second expiry,
  idempotent renewal, exactly-once cancel/expiry release, terminal cleanup, and
  no persistence or restart restoration; verify the mapping does not affect
  request IDs, artifact IDs, encode signatures, or frozen request data.
- [ ] 1.3 Split artifact begin/join from bounded HTTP waiting while preserving
  #1 single-flight, waiter accounting, zero-consumer grace, atomic publication,
  and bounded diagnostics; use the 5-second resource wait only as a response
  budget and never as an FFmpeg context deadline.
- [ ] 1.4 Implement the atomic generating handoff: reuse a valid logical lease
  or create one before releasing the final HTTP waiter, include lease/status/
  renew/cancel identity in every followable `202`, and verify no zero-consumer
  interval occurs between waiter release and logical ownership.
- [ ] 1.5 Add the registered start/renew and cancel actions, including stable
  frontend `ownerId` retry behavior, 15-second renewal target, `Retry-After: 2`,
  shared-artifact references, active cancel returning `leaseReleased=true`,
  repeated cancel returning HTTP 200 with `leaseReleased=false`, distinct
  `invalid_lease`/`lease_expired` conflicts, and zero-consumer cancellation
  after the lease expires.
- [ ] 1.6 Update registered video, output-frame, inspection, and metrics
  resources to use the same handoff and envelope while preserving source-frame
  behavior; verify ready/cache-hit behavior is immediate and healthy slow work
  is never serialized as 500, timeout, failed, or canceled.
- [ ] 1.7 Adapt the legacy scalar route to reuse registered identity, frozen
  descriptor, single-flight artifact, and logical lease when non-ephemeral;
  verify slow followable calls return the canonical 202 and unsupported
  ephemeral calls return the explicit `legacy_non_followable` limitation rather
  than a doomed 202.
- [ ] 1.8 Project #14 restart, retry, stale, expired, unknown, and terminal
  states without a second lifecycle engine; verify persisted generating work
  without a final artifact becomes interrupted, explicit retry creates a new
  token/version, and `/status` does not silently retry actions.
- [ ] 1.9 Add generation start/finish observation fields and bounded status
  metadata without changing identity or cache semantics; retain effective
  x265 `slow`/`veryslow` selection, avoid a short encode timeout, and omit a
  `-progress` pipe and fake progress fields in v1.

## 2. Frontend transport and Compatible Preview UX

- [ ] 2.1 Add API types and methods for stable `ownerId`, start/renew, status,
  cancel, 202 envelopes, lease expiry, and #14 recovery errors; verify every
  controllable API/status/start/cancel/polling fetch explicitly uses
  `credentials: omit`.
- [ ] 2.2 Update Profile Lab and other Compatible Preview surfaces to create
  without encoding, start/renew one logical owner, poll at about 1s then 2s
  capped at 5s, stop on all terminal/recovery conditions, and release on
  ready/cancel/unmount/expiry; verify a generating-to-ready flow performs one
  encode and does not let status polling own it.
- [ ] 2.3 Add bounded transport backoff capped at 15 seconds and presentation
  of generic generating/slow status after 30 seconds; verify temporary network
  failures do not show Preview failed, issue aggressive polling, or cancel
  backend generation.
- [ ] 2.4 Replace whole-video Compatible Preview Blob fetching with ready-gated
  native video URLs and native frame URLs; preserve Range/seek behavior and
  `crossOrigin="anonymous"` where applicable, while explicitly documenting
  that native browser media is not guaranteed `credentials: omit`.
- [ ] 2.5 Preserve #13 defenses and documentation: verify backend
  `stripInboundCookies()` remains in the media path, removes Cookie while
  preserving Range/unrelated headers, and logging does not expose Cookie or
  Authorization; do not change media authorization or CORS policy.
- [ ] 2.6 Handle native media readiness races by rereading canonical status,
  retrying a ready attachment only within a small fixed bound, resuming polling
  for generating, and using normal recovery for terminal states; verify media
  element errors are not all reported as encoder failure and cannot create an
  infinite retry loop.

## 3. Deterministic verification coverage

- [ ] 3.1 Add controlled-encoder/barrier tests for slow x265-like generation,
  resource 202 ownership handoff, existing-lease reuse, unrenewed expiry,
  eventual ready, genuine failure, explicit cancel, repeated cancel, shared
  consumers, zero consumers, status observers, disconnected waiters, and no
  arbitrary encode timeout; use channels/barriers or injected signals rather
  than multi-second lifecycle sleeps.
- [ ] 3.2 Add route tests for the exact `/status` matrix and separate resource/
  start/cancel matrix, including idle, generating, ready, failed, canceled,
  interrupted, stale, expired, and unknown; verify bounded diagnostic and
  retry/recreate fields remain distinct.
- [ ] 3.3 Add backend tests for lost initial `start(ownerId)` response,
  idempotent acquire/renew/cancel/expiry, terminal lease cleanup, shared
  artifact ownership, restart/interrupted recovery, explicit retry/new token,
  cache hits, operational metadata exclusion from identity, and legacy
  followable/non-followable behavior. Explicitly assert cancel A releases once,
  cancel A again returns HTTP 200 with `leaseReleased=false`, random or
  wrong-bound token returns HTTP 409 `invalid_lease`, known expired lease
  returns HTTP 409 `lease_expired`, and expired request tombstone returns HTTP
  410 `expired`.
- [ ] 3.4 Add a shared-owner regression asserting A and B reference one
  artifact, A cancels twice, B still owns exactly one reference, and generation
  remains active. Verify no repeated cancel decrements B or triggers
  zero-consumer cancellation.
- [ ] 3.5 Add frontend/API tests for 202 transport, lease renewal/cancel,
  polling/backoff stop conditions, slow wording, no Blob download,
  ready-gated native media, native readiness-race recovery, and terminal UI;
  verify controllable fetch credentials, native cookie limitation wording,
  `crossOrigin`, and no false credentialless claim.
- [ ] 3.6 Add focused middleware/media tests that preserve Cookie stripping,
  Range forwarding, unrelated-header behavior, and Cookie/Authorization log
  redaction; retain explicit x265/NAS metadata coverage without promising fake
  progress.
- [ ] 3.7 Run focused changed-area tests, then
  `openspec validate slow-compatible-preview --type change --strict` and
  `./scripts/verify.sh --auto`; verify `ALL CHECKS PASSED` before any future
  implementation task is presented as complete.
