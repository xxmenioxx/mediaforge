## 1. Corresponding Source and Runtime Notices

- [x] 1.1 Pin the backend base images and the GPL/LGPL codec and media package/source versions; verify the pinned references build successfully.
- [x] 1.2 Add a separate build target and script that exports exact FFmpeg, SVT-AV1, x264/x265, and linked GPL/LGPL Debian sources, the build recipe, and checksums; verify the archive contains each source input.
- [x] 1.3 Preserve and verify the exact SeConv, SkiaSharp, HarfBuzzSharp, Debian package copyright, and oneVPL Intel GPU runtime notices; inspect the built image's `/usr/share/licenses/mvforge` tree.

## 2. Resolved Dependency Inventories

- [x] 2.1 Generate the resolved Go module graph, imported-package license report, and matching license-text bundle; verify the report is reproducible against `go.mod`/`go.sum`.
- [x] 2.2 Generate the npm inventory and text notices from the production lockfile closure; verify the web image serves the notice bundle and unresolved entries fail the release check.

## 3. Release Evidence and External State

- [x] 3.1 Require source, notice, Go/npm inventory, SBOM/provenance, and digest evidence before the release job creates a Release; verify the default publication gate remains closed.
- [x] 3.2 Capture read-only Releases, tag-ref, GHCR package/version, and attestation metadata; update the dated audit without claiming historical downloads or clones were retracted.
- [x] 3.3 Rebuild and inspect the actual backend image; record FFmpeg configuration, linkage, package versions, and evidence limitations.

## 4. Verification and Delivery

- [x] 4.1 Run `./scripts/verify-compliance.sh` and any focused manifest/license/source-bundle checks; record unresolved external evidence.
- [x] 4.2 Run `./scripts/verify.sh --full` and confirm the verifier result and check count.
- [ ] 4.3 Commit and push only the scoped #57 changes to `chore/57-compliance-baseline`; verify its relationship to `origin/main`.
