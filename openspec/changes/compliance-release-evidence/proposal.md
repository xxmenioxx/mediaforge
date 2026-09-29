## Why

The current release workflow requests SBOM and provenance generation but does not preserve a reproducible corresponding-source bundle or complete notices for each distributed image. Issue #57 requires image-specific evidence before V1 publication, so the release gate must remain closed until that evidence can be generated, checked, and attached consistently.

## What Changes

- Define a per-image compliance evidence contract covering the resolved dependency inventory, license notices, build inputs, and GPL corresponding sources/source locations.
- Make the backend image retain notices for the exact SeConv, SkiaSharp, HarfBuzzSharp, and Intel GPU runtime artifacts it conveys.
- Produce resolved backend Go and shipped frontend npm license inventories and include the relevant notices with their runtime/release artifacts.
- Add a read-only GitHub Packages inventory capture and reconcile the audit with current Releases, tag refs, GHCR versions, and available attestations.
- Require release-side evidence before a release can pass the public publication gate; keep the gate closed by default.

Out of scope: changing repository visibility, deleting or republishing package versions, creating Releases, assessing past downloads/clones, legal advice, and changes to Profile semantics.

## Capabilities

### New Capabilities

- `compliance-release-evidence`: reproducible source, inventory, notice, and attestation evidence required for container release publication.

### Modified Capabilities

None.

## Impact

Affected systems: `backend/Dockerfile`, `frontend/Dockerfile`, license inventories, compliance scripts, `.github/workflows/release-images.yml`, and `docs/compliance/redistribution-audit.md`. No application API or media-processing behavior changes are intended. The release workflow will continue to reject publication unless the closed-by-default compliance gate is explicitly enabled after evidence review.
