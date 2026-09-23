## Why

Compatible Preview request creation is already decoupled from encoding, but the
first video, frame, inspection, or metrics resource request can still wait on
the complete shared artifact through its HTTP context. A legitimate software
x265 `slow` or `veryslow` encode can therefore be reported as a 408, canceled,
or generic failure when only the browser, request, or proxy stopped waiting.

Issue #15 makes long-running generation observable and non-misleading without
changing the artifact identity, single-flight, cancellation, or restart
ownership established by #1 and merged #14.

## What Changes

- Add one canonical Compatible Preview lifecycle/status contract for registered
  request IDs, including `generating`, `ready`, `failed`, `canceled`,
  `interrupted`, `stale_artifact`, `expired`, and `unknown_request` outcomes.
- Separate starting/joining generation from waiting for an HTTP media response;
  an active generation that exceeds the bounded resource wait budget returns a
  machine-readable `202 Accepted` `generating` response rather than a failure.
  Before the HTTP waiter is released, the request promotes or creates one
  bounded logical lease, so every followable `202` keeps that same generation
  alive. An existing valid lease is reused rather than duplicated.
- Add a bounded logical preview lease for an actively waiting UI, with
  idempotent initial ownership, explicit release/cancel behavior, renewal, and
  expiry. Status observation does not itself add an artifact consumer or keep
  abandoned work alive.
- Route non-ephemeral legacy scalar preview calls through the same registered
  request and lease contract. A legacy call that cannot expose a followable
  identity must return an explicit compatibility limitation, never a doomed
  `202 generating` response.
- Define separate status-projection and resource/action HTTP contracts:
  `/status` reports known lifecycle states with `200`, while resource/start
  actions preserve #14 terminal `409`/`410`/`404` behavior.
- Give the frontend status-aware polling and slow-work presentation, including
  backend-provided encoder/preset and elapsed time, while preserving explicit
  terminal recovery and cancellation messaging.
- Serve ready compatible video and frame resources through native media
  elements to preserve browser Range/streaming behavior. Controllable API,
  status, start, cancel, and polling fetches remain `credentials: omit`;
  native browser media requests are explicitly documented as browser-controlled
  and not guaranteed credentialless. #13 cookie stripping and dedicated-host
  deployment guidance remain authoritative.
- Re-read canonical status after a native media readiness race and use bounded
  recovery: retry ready attachment, resume polling for generating, and show
  normal recovery for terminal states without treating every media error as an
  encode failure.
- Keep generation lifetime independent from request wait budgets; do not add a
  short fixed FFmpeg timeout, change the selected x265 preset, or add an
  x265-specific lifecycle.
- Define focused backend, frontend, restart, cancellation, shared-consumer,
  cache-hit, network interruption, and deterministic slow-encoder coverage.

Out of scope: redesigning #1 artifact identity or single-flight ownership,
replacing #14 persistence/restart reconciliation, resuming an FFmpeg process,
adding a general scheduler/job engine, changing preview quality or preset
selection, implementing #19 preview-first Fidelity orchestration, or adding
live FFmpeg percentage/speed reporting unless a later change proves the
existing subprocess path can expose it safely.

## Capabilities

### New Capabilities

- `slow-compatible-preview`: Non-misleading long-running Compatible Preview
  status, bounded HTTP waiting, logical ownership, cancellation, and frontend
  UX for slow software encoders.

### Modified Capabilities

- No archived main capability is modified on this base: the Compatible Preview
  lifecycle from #1 and #14 remains change-owned by those in-flight changes.
  This capability is nevertheless an intentional compatibility delta for that
  existing behavior. It refines long resource waiting and `202 generating`,
  adds explicit logical ownership, and evolves the previous credentialless
  preview-fetch contract for ready native media as documented below.

## Impact

- Backend Compatible Preview request/artifact handlers, lifecycle projection,
  bounded wait behavior, logical lease bookkeeping, and API routes.
- Frontend Compatible Preview API types/client, Profile Lab orchestration,
  status polling, cancellation, and native media presentation.
- The credentialless contract is split deliberately: controllable fetches use
  `credentials: omit`; ready native media uses browser-controlled credentials,
  with backend `Cookie` stripping and #13 dedicated-host guidance required for
  complete browser isolation. No media authorization or CORS policy changes.
- Existing Nginx/browser timeout behavior is documented as an outer transport
  layer, not reused as an encode-kill policy; no proxy timeout increase is
  required for the non-blocking path.
- Focused backend/frontend tests and OpenSpec validation only. No production
  implementation is part of this design-only change.
