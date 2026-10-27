# NAS observer core — fixtures only (Issue #427)

This package is a **library**, not a deployed NAS daemon and not an alternate admission authority.
Its input can be synthetic Linux procfs or separately captured bytes. It contains no CLI,
network dialer, key generator, cron unit, private-key loader, Docker access, production
mount, or authorized real-Ollama execution mode.

## Implemented inputs

- `ParseLinuxHost`: bounded `/proc/meminfo` evidence, with explicit
  `MemTotal`, `MemAvailable`, `SwapTotal`, `SwapFree`; CPU usage from two
  advancing aggregate `/proc/stat` snapshots. No load-average substitution.
- `CollectLinuxHost`: optional read-only local procfs source with an explicit
  100 ms–5 s sampling interval. Do not invoke it in CI against the real host:
  synthetic parser fixtures are the verification boundary.
- `ParseOllamaPS`: decodes caller-supplied bounded `/api/ps` JSON only; it
  neither queries Ollama nor verifies the endpoint or model digest.
  `providerConfigurationId` is a *declared mapping*, not an attested endpoint.
- `GuardFromSignals`: unknown activity => `unavailable`; confirmed busy
  playback/FFmpeg/worker or unhealthy storage => `pressure`. It returns
  `allow` only when **all four independently supplied** signals are known idle
  and healthy. No real Jellyfin/FFmpeg/worker signal producers exist yet.
- `SignSnapshot`: accepts an **externally held** Ed25519 private key and
  serializes the same signed envelope/payload keys consumed by the existing
  `airesource.SignedObservationFileSource`. It never opens a key file or
  publishes a snapshot and does not establish that the measured values are
  authentic.

## Trust boundary

Signing fixture-derived values makes them *signed fixtures*, not NAS evidence.
An operator must not provision the signer with the same key as future host-owned
real observations. This PR does not provide a production key, secure key
storage, key rotation, host identity attestation, real Ollama `/api/ps` access,
verified service activity, atomic signed-snapshot publication, or live admission
authorization.

An eventual observer needs a separate design and security review for:
rootless host OS sensors with confirmed host namespace, read-only authenticated
Ollama connection bound to the exact endpoint, trusted service health/activity
sources for Jellyfin/MVForge/FFmpeg/storage, protected signing key custody,
atomic output and replay defense. It must not expose a Docker socket or read
production media/SQLite. The observer must fail closed on missing or conflicting
signals; a signed snapshot never overrides `airesource` or separate egress and
moment-of-run permission.

## Validation

The Go package unit tests use only synthetic data. Required repository
validation: `go test ./internal/nasobserver` (from `backend/`), strict
OpenSpec, `./scripts/verify.sh --auto`, then CI / Pipeline Smoke.

**Real preflight remains BLOCKED / NO-GO.** No `run-real` command is introduced.


## Stage 6: attributed critical signals and atomic handoff (fixture-only)

`GuardFromAttributedEvidence` requires a distinct exact expected source identity
and a recent sample for each of Jellyfin playback, FFmpeg activity, MVForge worker
activity and storage health. Unknown, stale or mismatched signals result in
`unavailable`, not `allow`. Observed busy activity produces `pressure`
only when all four source attributions pass validation. The source labels
are NOT cryptographic attestations, and consumers MUST NOT populate them from
untrusted caller input.

`ParseJellyfinSessions` is pure parsing of a supplied bounded `/Sessions`
response. It treats any `NowPlayingItem`, including paused, as busy. An empty
array alone is NOT sufficient evidence of idle on a shared NAS: the eventual
adapter must prove authentication, complete authorized session scope, fresh
response, exact peer identity, and no unobserved sessions. No such live adapter
or privilege is implemented here.

`PublishSignedSnapshotAtomically` writes an already signed envelope to an
existing directory, with a mode-0600 temp file, fsync, rename and parent sync.
It rejects a symlink destination and malformed envelope shapes. It does NOT
verify signature authenticity, secure parent-directory ownership, prevent
replay or provide private-key custody. An independently reviewed host service
must enforce those properties before this is allowed near production.

No producer currently establishes real host/namespace identity, endpoint
trust, Jellyfin session completeness, FFmpeg/worker authority, actual storage
health, key custody, or snapshot delivery. The real Ollama preflight stays
BLOCKED / NO-GO. No daemon, scheduled publisher, key file handling or network
probes were added.
