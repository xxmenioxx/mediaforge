# Issue #57 Redistribution and Compliance Audit

Audit date: 2026-09-28 (America/Mexico_City)
Repository: `xxmenioxx/mediaforge`  
Issue: [#57](https://github.com/xxmenioxx/mediaforge/issues/57), OPEN  
Feature branch: `chore/57-compliance-baseline`
Initial audit base: `67deb47ae9d6859474b739a4062ec23b8bd7ce17` (main after #41)
Final branch base: `6827bed5f7aa20d63565efd51609d22ddd72771c` (current `origin/main`, including #41 and #195)

This is a technical inventory of repository files, local builds, and read-only
GitHub API data. It distinguishes historical observed state from current
post-cleanup state. It is not a legal determination or legal advice. The local
images and corresponding-source bundle were not published and do not prove the
contents of images that were historically hosted on GHCR.

## Issue-to-repository gap analysis

| Requirement | Status | Evidence and remaining gap |
| --- | --- | --- |
| Temporarily keep the repository private | **complete** | The fresh read-only API snapshot reports `private: true` and `visibility: private`; no visibility setting was changed. |
| Pause new public releases and image publication | **complete; publication remains paused** | The release workflow defaults `MVFORGE_PUBLIC_RELEASES_ENABLED` to false and also requires `MVFORGE_EXISTING_DISTRIBUTIONS_REVIEWED=true`. The current Actions Variables endpoint reports zero variables, so neither gate is enabled. |
| Document intended use, user responsibility, and content boundaries | **complete** | `LEGAL.md` and README describe a self-hosted processing tool, user responsibility, no supplied media/keys/acquisition service, and third-party licensing. README positions it alongside Jellyfin/Plex and Radarr/Sonarr. |
| Explicitly exclude DRM circumvention | **complete** | `LEGAL.md` and README exclude AACS/BD+, CSS, Widevine or similar bypass, key retrieval/distribution, DRM stripping, and TPM bypass. |
| Capture and review the actual FFmpeg build | **complete** | Raw `-version`, `-buildconf`, linkage, enabled external libraries, VAAPI/QSV runtime dependencies, dpkg package versions, license metadata, and notice paths are in [`backend-image-2026-09-28.txt`](evidence/backend-image-2026-09-28.txt). This is evidence for the local build, not a claim about historical image contents. |
| Determine redistribution status of Docker images | **complete by removal of currently hosted packages** | Fresh API lookups return 404 for both GHCR packages. No backend or web package remains hosted on GitHub. Historical image contents and access history are unknown; removal is the chosen mitigation, not a claim that past copies or downloads were retracted. |
| Provide corresponding source/source-location and required notices | **complete for the future release process** | The final Dockerfile and scripts build a deterministic, checksummed source bundle containing FFmpeg/SVT-AV1, exact Debian x264/x265, libbluray, FriBidi, LAME, SoXR, and zimg sources, the build recipe/scripts, and runtime notices. Release-side evidence verification passed. No local image or source bundle was published. |
| Inventory backend Go and shipped frontend npm dependencies | **complete for the checked-in dependency graph** | Go: 63 resolved modules, 25 imported-package/license report rows including the local module, and 25 corresponding license files. npm: 364 lock entries with declared metadata; 90 production/shared packages with zero unresolved license texts. The release checker regenerates/compares both. |
| Include third-party notices in conveyed runtime artifacts | **complete for the future release process** | Actual local image inspection confirms Debian package copyright files, FFmpeg/SVT-AV1, SeConv, SkiaSharp, HarfBuzzSharp, Intel/oneVPL notices, plus frontend npm inventory/notices. Both historic GHCR packages are now absent; their old image contents were not established. Web evidence is in [`frontend-image-2026-09-28.txt`](evidence/frontend-image-2026-09-28.txt). |
| Generate/verify SBOM and provenance for releases | **complete as a future release gate** | The release workflow requests BuildKit SBOM and max-mode provenance and verifies SPDX/SLSA predicates before release creation. No current GHCR package remains; this does not establish whether historical images had those attestations. |
| Review existing Releases, tags, packages, and image distribution history | **complete by removal mitigation** | Fresh API evidence reports zero current Releases and tag refs and 404 for both historical GHCR package lookups. Historical downloads, clones, forks, cached images, previous copies, and prior license grants cannot be determined from current state. |
| Keep publication closed until deliberate reopening | **complete as a current hold** | The repository is private, current Releases/tag refs and GHCR packages are absent, and both release gates remain unset. This task does not enable either publication variable; reopening remains a separate deliberate action. |
| Keep the compliance CI check active | **complete** | The compliance job installs the Go and frontend dependency environments before running `scripts/verify-compliance.sh`; the check validates Go/npm inventory completeness and reproducibility. |

**Verdict: PR-ready and issue-closeable.** Repository-local source, notices,
dependency inventories, CI, and future release gates are implemented and
validated. Both historically observed GHCR packages were subsequently deleted,
and current read-only lookups confirm neither package exists. Removal resolves
the retained GitHub-hosted distribution blocker. It does not retract or prove
the absence of past downloads, clones, forks, cached images, previous copies,
or prior license grants. Public publication remains disabled.

## Current GitHub state

The post-cleanup read-only API capture window was
`2026-09-29T05:02:59Z`–`2026-09-29T05:03:16Z`. Its current-state snapshot is
[`github-post-cleanup-2026-09-29.json`](evidence/github-post-cleanup-2026-09-29.json).
The original package inventory below remains preserved at
[`github-packages-2026-09-28.json`](evidence/github-packages-2026-09-28.json)
as historical observed state.

| Resource | Current state |
| --- | --- |
| Repository visibility | Private |
| GitHub Releases | 0 |
| Git tag refs | 0 |
| `mvforge-backend` GHCR package | Not found (HTTP 404, `Package not found.`) |
| `mvforge-web` GHCR package | Not found (HTTP 404, `Package not found.`) |
| Actions variables | 0; public release and distribution review variables are unset |

The previous audit recorded 145 GitHub Releases, from `v0.2.1` through
`v1.1.45`, with only the three installer assets. Their current Release metadata
and Git tag refs are empty. This reports current GitHub state only; it does not
establish that historical downloads, clones, forks, or grants were retracted.

### Historical GHCR inventory before deletion

The preserved snapshot captured these package records before the package
deletions. These figures are historical and are not current retained counts.

| Package | Observed type / visibility | Version records | Tagged versions | Untagged versions | Unique tags |
| --- | --- | ---: | ---: | ---: | ---: |
| `mvforge-backend` | container / private | 452 | 149 | 303 | 299 |
| `mvforge-web` | container / private | 455 | 150 | 305 | 301 |

The historical total was **907 version records**. Every historical version ID,
digest (API `name` field), tag, creation time, and update time remains in
[`github-packages-2026-09-28.json`](evidence/github-packages-2026-09-28.json).
The latest tagged records at the time were:

- Backend: ID `1226427294`, digest
  `sha256:6c0385bc58c1827a893597d1bc55acfab012489eb777c43cc2cac8b213f7c53b`,
  tags `sha-7845128`, `1.1.45`, `1.1`, and `latest`.
- Web: ID `1226421239`, digest
  `sha256:52e92abececb07ff6a83965083a59bde246719a14d8cac3797f1c8534ae08605`,
  tags `sha-7845128`, `1.1.45`, `1.1`, and `latest`.

After that historical capture, both GHCR package entities were deleted. Fresh
package lookups now return HTTP 404 for both names, so no current backend/web
package versions or tags remain available through these package endpoints. The
historical API did not expose platform/runtime manifests or embedded
SBOM/provenance fields. Its attestation lookups returned 404 for the latest
tagged digests; those responses did not establish whether attestations existed.

Deletion mitigates continued GitHub-hosted distribution. It cannot retract or
prove the absence of historical downloads, clones, forks, cached images,
previous copies, or prior license grants. That uncertainty is recorded as a
historical limitation, not as a current retained-distribution blocker.

## FFmpeg and backend image evidence

The final local backend image is Linux/amd64, identified by manifest-list
digest `sha256:3574de2dd7d97232532114ace8e367b4fcea20add6af0c9b91ae57c30d4f6fab`,
and is identified in the linked evidence file. Build command (local only):

```sh
tail -n +2 backend/Dockerfile > /tmp/mvforge-57-Dockerfile-compliance
docker buildx build --platform linux/amd64 --file /tmp/mvforge-57-Dockerfile-compliance \
  --tag mvforge-compliance-audit:local --load backend
```

The local Docker Hub lookup for `docker/dockerfile:1.7` timed out. The
validation therefore used BuildKit's bundled Dockerfile frontend; the
Dockerfile body and build context were the checked-in versions. The release
workflow continues to use the checked-in syntax directive and has not been
run for a release.

Observed `ffmpeg -version` is FFmpeg 8.1.2. The actual `-buildconf` includes
`--enable-gpl`, `--enable-version3`, `--enable-libx264`, `--enable-libx265`,
`--enable-libaom`, `--enable-libdav1d`, `--enable-libsvtav1`, `--enable-libvpx`,
`--enable-libass`, `--enable-libfreetype`, `--enable-libfontconfig`,
`--enable-libfribidi`, `--enable-libmp3lame`, `--enable-libopus`,
`--enable-libvorbis`, `--enable-libsoxr`, `--enable-libzimg`,
`--enable-libwebp`, `--enable-libopenjpeg`, `--enable-libbluray`,
`--enable-libvpl`, `--disable-libmfx`, `--enable-vaapi`, `--enable-libdrm`,
and `--enable-opencl`. `--enable-nonfree` is absent from the binary's
configuration output.

`ldd` of the built `libavcodec.so` confirms dynamic linkage to
`libx264.so.164` and `libx265.so.215`, as well as libaom, dav1d, SVT-AV1,
libvpx, libva, libvpl, libva-drm, libdrm, and OpenCL. FFmpeg reports `vaapi`,
`qsv`, `drm`, and `opencl` methods. The Intel `iHD_drv_video.so` file is in the
image and the relevant `libva`/`libvpl`/DRM libraries resolve; no GPU device
was attached, so hardware execution was not tested.

The inspected package copyright files identify x264/x265 as GPL-2+;
libbluray, FriBidi, and SoXR as LGPL-2.1+; and LAME as LGPL-2+ (with
additional file-specific notices). The zimg package copyright metadata
contains GPL-3+ with an AutoConf exception and WTFPL-2 entries. Other directly
linked Debian components include BSD-2-Clause libaom/dav1d, BSD-3-Clause
libvpx/Opus, and mixed package notices for OpenJPEG. All available Debian
`/usr/share/doc/*/copyright` files are copied into
`/usr/share/licenses/mvforge/debian/`; the image build and evidence capture fail
if identified GPL/LGPL package notices are absent. Exact package versions and
license metadata are in the linked runtime evidence file.

The FFmpeg build flags select the GPL version-3 build path and the binary is
dynamically linked to GPL-2+ x264/x265. This identifies source and notice
obligations to review; it is not a conclusion about the legal status of the
complete MVForge image. FFmpeg documents the effect of GPL-enabled options and
external libraries in its [license guidance](https://ffmpeg.org/legal.html).
GPLv3 defines corresponding source to include what is needed to generate,
install, run, and modify the object code, including build scripts; see
[GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.en.html).

### Source, notices, and dependency inventory

- The corresponding-source build was run locally and packaged as a
  deterministic gzip/tar archive. The final local archive SHA256 is
  `43fd5dc514045bb01f2137ae4b7eccc737d505f98e356cfda771ba73c018c9ee`; its
  release-evidence checker passed. It includes FFmpeg 8.1.2 and SVT-AV1 4.1.0
  archives pinned by SHA256; exact x264/x265 Debian sources; and exact Debian
  libbluray, FriBidi, LAME, SoXR, and zimg sources matching pinned
  runtime/development package versions. It also contains the exact release
  Dockerfile, source/archive scripts, Intel source commit references, component
  license texts/notices, backend Go inventory and license texts, and a per-file
  `SHA256SUMS` plus outer archive checksum. This was a local validation
  artifact; no source archive was published.
- The backend runtime contains SeConv's checksum-verified archive `LICENSE`,
  SkiaSharp/HarfBuzzSharp NuGet license and third-party notice files, oneVPL
  Intel GPU runtime `LICENSE`, Intel component licenses, FFmpeg/SVT-AV1
  notices, and Debian copyright files.
- `backend/licenses/GO_MODULES.txt`, `GO_LICENSES.csv`, and `go-modules/`
  provide the resolved Go graph, package/license scan, and license texts. The
  scanner warns that it cannot inspect C/assembly files and that the local
  module's empty version makes its generated license URL potentially
  imprecise; the local module itself is Apache-2.0 and the CSV has no Unknown
  licenses.
- `frontend/licenses/npm/inventory.json` and
  `frontend/licenses/npm/THIRD_PARTY_NOTICES.txt` cover 90 production/shared
  packages with zero unresolved entries. The built nginx image was inspected
  and contains both files under `/licenses/npm/`; the notice file is 107,697
  bytes. Image evidence is in [`frontend-image-2026-09-28.txt`](evidence/frontend-image-2026-09-28.txt).
- The release workflow requests BuildKit SBOM and max-mode provenance for
  exact image digests. `scripts/verify-image-attestations.sh` fails without
  SPDX SBOM and SLSA provenance predicates; the published image is not
  available in this local audit, and no release was created.

Base images are pinned by digest in backend and frontend Dockerfiles. Exact
x264/x265 and identified LGPL package versions are pinned in the backend
builder/runtime, and corresponding Debian sources are packaged. Other Debian
APT dependencies still resolve from the live configured repositories; the
per-image BuildKit SBOM and copied package copyright metadata are required for
each future digest. This is a remaining reproducibility limit, not a claim
that every APT package is byte-for-byte pinned.

## Historical limitation and future publication controls

Both historical GitHub container packages have been deleted, and the current
API lookups report neither package exists. This is the selected mitigation for
the unverifiable historical container artifacts: removal rather than continued
GitHub-hosted redistribution. The current snapshot does not retract or prove
the absence of historical downloads, clones, forks, cached images, previous
copies, or prior license grants. This uncertainty is documented but is not a
retained-distribution blocker because the packages are no longer hosted.

Public publication remains disabled. The Actions Variables endpoint currently
reports zero variables, and the release workflow defaults both gates to
closed. This reconciliation does not set or modify either variable. Reopening
public distribution remains a separate deliberate action, even though the
historical distribution review/mitigation requirement is now satisfied by
removal.

Every future image publication must still pass the existing release process,
including:

- corresponding-source archive and checksum;
- required project and third-party notices;
- reproducible Go and npm inventories;
- exact image digest evidence;
- SBOM and provenance generation and verification; and
- release compliance verification before Release creation.
