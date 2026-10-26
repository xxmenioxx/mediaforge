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
