## Context

See `proposal.md` and `specs/compliance-release-evidence/spec.md`. The current backend image builds FFmpeg with GPL/version-3 flags and links Debian x264/x265 packages; it pins upstream FFmpeg/SVT archives and several Intel commits but does not pin the Debian package universe or preserve GPL source with the release. The release workflow asks BuildKit for SBOM/provenance but does not verify or attach a source bundle. GitHub Packages currently retains private, tagged image versions even though repository Releases and Git tags are empty.

## Goals / Non-Goals

**Goals:** Provide exact, image-associated technical evidence and fail closed when repository-produced evidence is absent. Keep source/build inputs and notices together with the image release without adding application behavior or runtime dependencies.

**Non-Goals:** Change repository or package visibility; remove image versions; create or publish a release in this task; establish a legal conclusion; claim that prior downloads or clones were retracted.

## Decisions

- Produce a separate corresponding-source archive from the same locked build inputs, rather than putting compiler trees into the runtime image. Include the FFmpeg and SVT source archives, matching Debian x264/x265 source packages, pinned Intel source references, the backend Dockerfile, checksums, and an image-build manifest. Attach the archive to the versioned GitHub Release alongside required notices.
- Pin Debian base image digests and use a dated Debian snapshot plus explicit x264/x265 package/source versions. Retain the package-version inventory for each built image so transitive runtime package changes are visible and source package versions can be matched.
- Copy the SeConv archive's own license file, all license/notice files present in the exact SkiaSharp and HarfBuzzSharp NuGet packages, and the oneVPL Intel GPU runtime's upstream license from the same pinned source checkout into the runtime license tree. Fail the build if a required upstream notice is unavailable.
- Generate a resolved Go module inventory and license-text bundle from the module graph used to build the backend. Generate the npm inventory from the lockfile production closure; include that bundle in the web runtime and release assets.
- Keep the existing explicit `MVFORGE_PUBLIC_RELEASES_ENABLED=true` switch closed by default. The verification job must construct and validate evidence before the gate; image publication must expose BuildKit SBOM/provenance for the resulting digest; the GitHub Release job must require and attach the source, notices, inventories, and digest evidence.
- Treat GitHub API metadata as a dated, read-only snapshot. Record package version IDs, API-provided digests/tags/timestamps, and visibility, while labeling absent SBOM/provenance fields or inaccessible registry details as unverified.

## Risks / Trade-offs

- [Source archive increases release download size] → Keep source separate from the runtime image and build it only in release verification.
- [Debian snapshot or source package versions are unavailable] → Make source generation and compliance verification fail before publication; do not substitute a nearby source version.
- [NuGet packages omit third-party license texts] → Preserve their package metadata and upstream notices only after exact-version inspection; report unresolved components and keep the gate closed.
- [BuildKit emits attestations that are not visible to the package API] → Bind the release evidence to the pushed image digest and record API visibility limits; require a human registry-side check before changing the gate setting.
- [Evidence becomes stale after an image rebuild] → Generate it for every release from that release's locked manifests and attach it to the matching digest.

## Migration Plan

No existing runtime data or APIs change. Future release tags first run evidence generation and validation; the existing gate still stops before publication unless explicitly enabled. Rollback consists of reverting the release-evidence workflow/build changes while leaving the gate unset; already-retained GHCR versions are not mutated by this change.

## Open Questions

The GitHub Packages REST endpoints return private package version metadata but do not expose a complete registry-layer inspection or an SBOM/provenance view for every retained digest. An administrator must decide the external disposition of the retained image tags after reviewing digest-bound evidence; this change does not perform package deletion or visibility changes.
