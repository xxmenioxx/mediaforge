# Issue #57 Redistribution and Compliance Audit

Audit date: 2026-09-28  
Repository: `xxmenioxx/mediaforge`  
Issue: [#57](https://github.com/xxmenioxx/mediaforge/issues/57), OPEN  
Branch/base reviewed: `chore/57-compliance-baseline` at `fac11c9c4866d82efc4a0715063460f0d92e4c4b`

This is a technical inventory of repository files, a local build, and
read-only GitHub metadata. It does not determine legal compliance or provide
legal advice. The local image below was built for this audit and was not
published; it is not proof of the contents or redistributability of an
existing GHCR image.

## Issue-to-repository gap analysis

| Issue requirement | Status | Evidence and gap |
| --- | --- | --- |
| Temporarily make the repository private | **complete** | GitHub API reports `PRIVATE`. |
| Pause new public releases | **complete** | Release workflow fails unless `MVFORGE_PUBLIC_RELEASES_ENABLED` is exactly `true`; the repository has no Actions variables, so the workflow default is `false`. |
| Pause container publication | **complete** | `publish` depends on the closed release gate. Existing GHCR package tags could not be inspected; see external items below. |
| Review existing releases | **complete** | All 145 releases were enumerated. Every release has only `mvforge-backup.sh`, `mvforge-compose.yml`, and `mvforge.env.example`; none includes license notices, SBOM, or a source bundle. Latest is `v1.1.45`. |
| Review existing packages/images | **external/manual evidence required** | The available GitHub token lacks `read:packages`; anonymous manifest requests for the two workflow image names returned HTTP 401. This does not establish whether a package is absent, private, or which tags/digests exist. |
| Document intended/legal use boundaries | **complete** | `LEGAL.md` covers user responsibility, no supplied content/keys/acquisition service, third-party licenses, and no legal advice. README now links those boundaries. |
| Explicitly exclude DRM circumvention | **complete** | `LEGAL.md` lists AACS/BD+, CSS, Widevine, key retrieval/distribution, DRM stripping, and TPM bypass as non-goals. README now states the non-goal. |
| Capture and review the actual FFmpeg configuration | **complete for this checkout build; partial for published images** | The backend Dockerfile was built and its resulting local image was inspected. Published GHCR image digests remain unverified. |
| Determine image redistribution status | **partial** | The local build uses GPL and version-3 FFmpeg flags and dynamically links GPL-2+ x264/x265. Corresponding-source and notice delivery are not present as a verified release package. No legal conclusion is made. |
| Inventory Go, npm, FFmpeg, and third-party dependencies | **partial** | `go list -m all` resolves 63 Go modules. `frontend/package-lock.json` has 364 resolved package entries and license metadata on all 364. The summary notice is not a complete per-component license-text bundle, and a Go license report is absent. |
| Generate release/container SBOM and provenance | **partial** | Release workflow requests BuildKit `sbom: true` and `provenance: mode=max` for images. The registry artifacts could not be inspected. SBOM/provenance are not among the 145 existing GitHub release asset sets. |
| Reopen only after the gate passes | **complete as current hold** | The repository remains private and the code-level publication gate defaults closed. No visibility, release, or package settings were changed. |

**Issue #57 is not ready to clear. Keep the release gate closed.** Existing
public release history and package visibility still need review, and this local
build surfaced source/notice work that is not represented by a passing
manifest-presence check.

## FFmpeg build evidence

Reproduction on 2026-09-28 from backend context at base commit
`fac11c9c4866d82efc4a0715063460f0d92e4c4b`:

```sh
cd backend
docker build --progress=plain --tag mvforge-compliance-audit:local .
docker run --rm --entrypoint /opt/ffmpeg/bin/ffmpeg mvforge-compliance-audit:local -hide_banner -version
docker run --rm --entrypoint /opt/ffmpeg/bin/ffmpeg mvforge-compliance-audit:local -hide_banner -buildconf
```

Build result: Linux/amd64, image/index ID
`sha256:f730cf3b8a87f343b61f3591a44dfcd574efbcfa8058f1ae8d5734d0ebe55b6b`.
Docker's build output resolved `golang:1.25-trixie` to
`sha256:2c4c60ef415fbfa5e90300722293bef36c5e63fae17570ce18f580af933dbd73`
and `debian:trixie-slim` to
`sha256:a99cfc517144bc59b1978475ec53b46ecabec7e43635402ee5b77cc54d1b20a`.
The final Dockerfile smoke test completed for x265, SVT-AV1, and audio.

Observed `ffmpeg -version`:

```text
ffmpeg version 8.1.2 Copyright (c) 2000-2026 the FFmpeg developers
built with gcc 14 (Debian 14.2.0-19)
libavutil      60. 26.102
libavcodec     62. 28.102
libavformat    62. 12.102
libavdevice    62.  3.102
libavfilter    11. 14.102
libswscale      9.  5.102
libswresample   6.  5.102
```

Observed `ffmpeg -buildconf` includes:

```text
--enable-gpl --enable-version3 --enable-libx264 --enable-libx265
--enable-libaom --enable-libdav1d --enable-libsvtav1 --enable-libvpx
--enable-libass --enable-libfreetype --enable-libfontconfig --enable-libfribidi
--enable-libmp3lame --enable-libopus --enable-libvorbis --enable-libsoxr
--enable-libzimg --enable-libwebp --enable-libopenjpeg --enable-libbluray
--enable-libvpl --disable-libmfx --enable-vaapi --enable-libdrm --enable-opencl
```

`--enable-nonfree` is absent from the actual `-buildconf` output. `ldd` on
`libavcodec.so` confirmed dynamic links to `libx264.so.164`, `libx265.so.215`,
`libaom.so.3`, `libdav1d.so.7`, `libSvtAv1Enc.so.4`, `libvpx.so.9`,
`libva.so.2`, `libva-drm.so.2`, `libvpl.so.2`, `libdrm.so.2`, and
`libOpenCL.so.1`. FFmpeg reports `vaapi`, `qsv`, `drm`, and `opencl` hardware
acceleration methods. The image build verifies the Intel `iHD` driver file,
but no GPU device was passed to this local container, so hardware encode/decode
was not exercised.

The captured Debian runtime package versions include:

| Package | Version | Package copyright metadata |
| --- | --- | --- |
| `libx264-164` | `2:0.164.3108+git31e19f9-2+b1` | GPL-2+ (with file-specific notices) |
| `libx265-215` | `4.1-2` | GPL-2+ |
| `libaom3` | `3.12.1-1+deb13u1` | BSD-2-Clause |
| `libdav1d7` | `1.5.1-1` | BSD-2-Clause |
| `libvpx9` | `1.15.0-2.1+deb13u1` | BSD-3-Clause |
| `libbluray2` | `1:1.3.4-1+b2` | LGPL-2.1+ |
| `libmp3lame0` | `3.100-6+b3` | LGPL-2+ (with file-specific notices) |
| `libopus0` | `1.5.2-2` | BSD-3-Clause (with file-specific notices) |
| `libsoxr0` | `0.1.3-4+b2` | LGPL-2.1+ (with file-specific notices) |
| `libopenjp2-7` | `2.5.3-2.1~deb13u2` | Mixed package notices |

These license identifiers were read from the Debian copyright files inside
the built image. Full notices remain in the package copyright files where
Debian supplies them. The build also copies FFmpeg's GPLv3/license files,
SVT-AV1's BSD and patent files, and the listed Intel libva, gmmlib, media-driver
and oneVPL component license files into `/usr/share/licenses/mvforge`.

### Redistribution blockers shown by the build

- FFmpeg is configured with `--enable-gpl` and `--enable-version3`; x264 and
  x265 are dynamically linked GPL-2+ Debian libraries. The built image does
  not include an accompanying FFmpeg/x264/x265 corresponding-source bundle,
  source location, or written offer. The Dockerfile pins the FFmpeg and SVT-AV1
  source archives by SHA256 and pins Intel Git commits, but that build context
  alone does not capture source for the dynamically installed Debian packages.
- Debian base tags and APT packages are not pinned to immutable snapshots.
  The package versions above identify this local build only; rebuilding later
  can resolve different Debian package contents.
- The Dockerfile copies the SeConv executable and SkiaSharp/HarfBuzzSharp
  native libraries but does not copy separate license/notice materials for
  them into `/usr/share/licenses/mvforge`. It also did not contain a distinct
  notice there for the oneVPL Intel GPU runtime. Recover and verify the exact
  upstream texts before treating those notices as satisfied.
- The local build did not run the release workflow's registry SBOM step and
  cannot establish the SBOM or source availability for any already-published
  image digest.

FFmpeg's own licensing page distinguishes its GPL-enabled build from an
LGPL-only build and notes that the applicable license/source conditions must
be reviewed for the distributed binary:
[FFmpeg License and Legal Considerations](https://ffmpeg.org/legal.html).
GPLv3 defines corresponding source to include source needed to generate,
install, run, and modify the object code, including build/install scripts:
[GNU GPLv3, section 6 and definitions](https://www.gnu.org/licenses/gpl-3.0.en.html).
These references inform the technical follow-up; they are not a legal opinion.

## Manifests, notices, releases, and gate

- Backend Go: `backend/go.mod` and `backend/go.sum`; `go list -m all` returned
  63 modules including the main module. The repository inventory lists direct
  packages but has no resolved module-to-license report or bundled notice set.
- Frontend npm: `frontend/package-lock.json` is lockfile version 3 with 364
  `node_modules/` records; all 364 currently declare a `license` field. These
  are package metadata values, not verified license texts. The verifier now
  fails if any resolved lock entry loses that field.
- Containers: `.github/workflows/release-images.yml` requests BuildKit SBOM and
  provenance for backend and web images, and the current release job includes
  `LICENSE`, `NOTICE`, `LEGAL.md`, and `THIRD_PARTY_LICENSES.md`. Neither
  condition retroactively changes existing releases.
- GitHub metadata: the repository is private; the Actions variables endpoint
  returned zero variables, so `MVFORGE_PUBLIC_RELEASES_ENABLED` is unset and
  the workflow's `false` default holds. The repository has 145 releases from
  `v0.2.1` through `v1.1.45`; all 145 have the same three install assets and
  none has the above legal files or an SBOM/source asset.
- GitHub Packages: the workflow names `ghcr.io/xxmenioxx/mvforge-backend` and
  `ghcr.io/xxmenioxx/mvforge-web`. The packages API returned 403 because the
  current credential lacks `read:packages`; anonymous manifest access returned
  401. The NAS SSH runtime query timed out. Existing image tags, digests,
  visibilities, and attestations therefore remain unverified.

## Remaining work before clearing #57

1. Have a repository/package administrator inventory both GHCR packages, all
   tags/digests, package visibility, and the SBOM/provenance attached to each
   released image. Compare that evidence to the release list above.
2. Choose and implement an exact corresponding-source and source-offer process
   for the GPL-enabled FFmpeg/x264/x265 build and the rest of the conveyed
   binaries. Include the exact build scripts and source for each artifact.
3. Pin base image digests and Debian APT snapshots, then retain the complete
   package/source inventory and SBOM for each image digest.
4. Add and verify notices for SeConv, SkiaSharp, HarfBuzzSharp, and the oneVPL
   Intel GPU runtime, plus a resolved Go license report and npm license texts
   appropriate to the packages actually included in the web image.
5. Decide how to handle the 145 existing releases and any existing container
   tags. Repository privacy does not retract earlier downloads, clones, forks,
   or grants.

Recommended follow-up: a focused release-source and notice pipeline change.
Keep the current repository private and publication gate closed until those
items are evidenced and reviewed.
