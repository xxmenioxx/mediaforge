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
- Add a bounded logical preview lease for an actively waiting UI, with explicit
  release/cancel behavior and expiry. Status observation does not itself add an
  artifact consumer or keep abandoned work alive.
- Give the frontend status-aware polling and slow-work presentation, including
  backend-provided encoder/preset and elapsed time, while preserving explicit
  terminal recovery and cancellation messaging.
- Serve compatible video and frame resources through native media elements only
  after the artifact is ready, preserving browser Range/streaming behavior.
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

None.

## Impact

- Backend Compatible Preview request/artifact handlers, lifecycle projection,
  bounded wait behavior, logical lease bookkeeping, and API routes.
- Frontend Compatible Preview API types/client, Profile Lab orchestration,
  status polling, cancellation, and native media presentation.
- Existing Nginx/browser timeout behavior is documented as an outer transport
  layer, not reused as an encode-kill policy; no proxy timeout increase is
  required for the non-blocking path.
- Focused backend/frontend tests and OpenSpec validation only. No production
  implementation is part of this design-only change.
