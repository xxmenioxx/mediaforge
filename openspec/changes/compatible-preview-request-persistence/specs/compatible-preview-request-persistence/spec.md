## Purpose

Preserve enough Compatible Preview request and shared-artifact identity across
backend restarts to reuse valid published previews and expose deterministic
recovery states without resuming a dead media process.

## ADDED Requirements

### Requirement: Persistent request identity is recoverable

The system SHALL persist the identity and lifecycle metadata required to
address each non-ephemeral Compatible Preview request after a backend restart.
The persisted relationship SHALL retain the request ID, normalized request
identity, canonical artifact identity, creation/update/access and expiration
times, lifecycle state, and bounded failure or recovery reason when present.
Runtime execution state such as contexts, cancel functions, waiters, timers,
mutexes, goroutines, and process ownership SHALL NOT be required for recovery.

#### Scenario: Persistent request is restored after restart

- **WHEN** a non-ephemeral request has been created and the backend restarts
- **THEN** the same request ID is addressable with its canonical artifact
  relationship and expiration metadata restored

#### Scenario: Pre-feature requests are absent

- **WHEN** an installation is upgraded without durable Compatible Preview
  request records
- **THEN** request IDs that existed only before the upgrade remain unknown after
  restart, while requests created after the upgrade use the new persistence

### Requirement: Ready artifacts remain reusable after restart

The system SHALL keep a restored request usable under its existing request ID
when its persisted artifact is the canonical final artifact, is within the
controlled Compatible Preview cache root, and passes the existing lightweight
published-file validity checks. A valid restored ready artifact SHALL be
served or inspected through the existing #1 artifact path without starting a
new encode.

#### Scenario: Ready request and final artifact survive restart

- **WHEN** a request is ready, its final artifact is present and non-empty, and
  the backend restarts
- **THEN** the existing request ID continues to serve video, inspect, metrics,
  and supported output-frame consumers without a second encode

#### Scenario: Ready metadata points to a missing artifact

- **WHEN** persisted metadata says an artifact is ready but the final file is
  missing or empty after restart
- **THEN** the request is not advertised as ready and returns an explicit
  stale/recovery response that does not serve the missing path

### Requirement: Restart reconciliation never revives dead generation work

The system SHALL reconcile persisted in-flight generation conservatively. A
persisted `generating` state SHALL never be restored as an active joinable
generation because its owner process and cancellation context no longer
exist. If the canonical final artifact is complete and valid, reconciliation
MAY promote the artifact to ready; otherwise it SHALL expose an explicit
interrupted or recoverable state.

#### Scenario: Generating metadata has a complete final artifact

- **WHEN** a request was persisted as generating at restart and its canonical
  final artifact is present and valid
- **THEN** the request is promoted to ready and later consumers reuse that
  artifact without joining the dead generation

#### Scenario: Generating metadata has no complete final artifact

- **WHEN** a request was persisted as generating at restart and its final
  artifact is missing, empty, or only a staging/partial file exists
- **THEN** the old generation becomes interrupted/recoverable, no consumer
  joins it, and a retry can start a fresh generation through the existing #1
  coordination

### Requirement: Terminal failure and cancellation remain deterministic

The system SHALL preserve persisted failed and canceled request/artifact
states as terminal and non-joinable until normal expiration or cleanup. A
restart SHALL NOT silently change a canceled or failed request to generating or
ready. A retry SHALL create or reactivate a fresh generation through the #1
  non-joinable terminal replacement rules.

#### Scenario: Failed request survives restart

- **WHEN** a failed request has not expired and the backend restarts
- **THEN** the same request ID returns a deterministic failed lifecycle result
  with its bounded reason and does not join the failed generation

#### Scenario: Canceled request survives restart

- **WHEN** a canceled request has not expired and the backend restarts
- **THEN** the same request ID returns a deterministic canceled lifecycle result
  and does not resume or silently restart FFmpeg

### Requirement: Shared artifact identity remains canonical

The persisted request layer SHALL reference the canonical artifact identity
owned by #1 rather than deriving a new cache key or creating a second
single-flight lifecycle. Multiple request records that reference one artifact
SHALL be restored to that one artifact relationship and SHALL not cause
duplicate encodes solely because the backend restarted.

#### Scenario: Multiple requests share one artifact across restart

- **WHEN** two or more persistent request IDs reference the same canonical
  artifact and that artifact remains valid across restart
- **THEN** all restored requests reuse one ready artifact and no duplicate
  encode is started for the shared identity

#### Scenario: Retrying a terminal generation

- **WHEN** a consumer retries a failed, canceled, or interrupted request
- **THEN** the fresh attempt uses the existing canonical artifact identity and
  #1 single-flight coordination, while the old terminal generation is not
  joinable

### Requirement: Expiration and cleanup remain bounded and coordinated

The system SHALL apply the existing Compatible Preview request retention
window to persisted request access/expiration and SHALL not turn preview
metadata into permanent history. Request expiration SHALL remove the request
handle from normal lookup and SHALL return an explicit expired state to a
consumer using its old ID. Artifact cleanup SHALL continue to honor #1's
  active-consumer/generation protections, persistent cache age/item/size
  limits, and shared-request references.

#### Scenario: Request expires while the backend is stopped

- **WHEN** a persisted request's expiration time passes before startup
- **THEN** startup cleanup expires the request, removes it from active lookup,
  and a later use returns an explicit expired response rather than an internal
  error

#### Scenario: Shared artifact outlives one request

- **WHEN** one request expires but another live request still references the
  same valid persistent artifact
- **THEN** request cleanup removes only the expired handle and does not delete
  or invalidate the shared artifact

### Requirement: Recovery states have explicit API and frontend semantics

The existing request resource routes SHALL return normal media responses for a
ready request and machine-readable lifecycle responses for expired, stale,
interrupted, failed, and canceled requests. Unknown IDs SHALL remain distinct
from expired IDs. Recovery responses SHALL not use a generic 500 for normal
lifecycle conditions and SHALL include whether retrying is meaningful.

#### Scenario: Frontend resumes with a ready request ID

- **WHEN** the frontend keeps a request ID while the backend restarts and the
  artifact reconciles ready
- **THEN** existing polling/resource calls continue to work without requiring
  the frontend to know how persistence is implemented

#### Scenario: Frontend receives an interrupted or stale request

- **WHEN** a resource call uses a request whose generation was interrupted or
  whose artifact is stale/missing
- **THEN** the backend returns an explicit retryable lifecycle response and the
  frontend presents a useful retry path instead of hiding the condition as a
  timeout or generic server error

#### Scenario: Frontend receives an expired or unknown request

- **WHEN** a resource call uses an expired request ID or an ID never known to
  the backend
- **THEN** the response distinguishes explicit expiration from unknown lookup
  and the frontend clears or recreates state as appropriate

### Requirement: Restored cache paths are safe and startup work is bounded

The system SHALL trust a persisted artifact path only after validating that it
is a canonical final path under the configured Compatible Preview cache root
and corresponds to the persisted artifact identity. It SHALL reject or ignore
paths outside controlled roots, arbitrary filenames, zero-byte files, staging
files, and partial files. Startup reconciliation SHALL use bounded metadata
loading and direct filesystem existence/stat checks and SHALL NOT probe or
decode every cached preview.

#### Scenario: Persisted path escapes the preview cache root

- **WHEN** durable metadata contains an artifact path outside the canonical
  Compatible Preview cache/staging roots
- **THEN** the path is rejected, the request is not restored as ready, and no
  arbitrary file is served or deleted

#### Scenario: Large cache is restored

- **WHEN** many persisted preview records exist at startup
- **THEN** reconciliation remains bounded by metadata and lightweight final
  file checks, deferring deeper validation until consumption where safe

### Requirement: Filesystem and metadata crash windows reconcile safely

The system SHALL tolerate a crash between generation start, final publication,
durable state updates, cleanup, or cancellation. It SHALL treat the final
published artifact and durable metadata as independently recoverable evidence
and SHALL choose a safe stale, interrupted, ready, expired, failed, or
canceled state on the next reconciliation rather than assuming a cross-system
atomic transaction.

#### Scenario: Final file exists while durable state is still generating

- **WHEN** the process dies after atomically publishing a complete final file
  but before recording ready
- **THEN** startup validates the canonical final file and promotes the
  request/artifact to ready

#### Scenario: Metadata says ready after cleanup removed the file

- **WHEN** cleanup deletes a final artifact before the corresponding durable
  row is fully updated
- **THEN** the next reconciliation detects the missing file and removes the
  ready advertisement, returning stale/recovery semantics instead of serving a
  bad path

