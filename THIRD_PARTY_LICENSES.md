# Third-Party Software and License Notices

This is a technical inventory of software MVForge builds, bundles, or directly
depends on. It records observed versions and the location of collected notice
material; it does not determine legal compliance or provide legal advice.
Package license fields and this summary are not substitutes for component
license texts.

## Backend media stack

| Component | Version / origin | Observed license metadata | Conveyed use and evidence |
| --- | --- | --- | --- |
| FFmpeg | 8.1.2, upstream source archive pinned by SHA256 | GPL configuration enabled, version 3 enabled | Separate FFmpeg executable; build configuration and source archive are retained in the release source bundle. |
| x264 | Debian `libx264-164` `2:0.164.3108+git31e19f9-2+b1` | Debian copyright metadata: GPL-2+ | Dynamically linked; matching Debian source package version is pinned and included in the corresponding-source bundle. |
| x265 | Debian `libx265-215` `4.1-2` | Debian copyright metadata: GPL-2+ | Dynamically linked; matching Debian source package is pinned and included in the corresponding-source bundle. |
| SVT-AV1 | 4.1.0, upstream archive pinned by SHA256 | BSD-2-Clause and patent notice files | Shared library; source archive and its license/patent files are retained with the corresponding source. |
| libbluray2 | Debian `1:1.3.4-1+b2` | Debian copyright metadata: LGPL-2.1+ | Matching Debian source is pinned and included in the corresponding-source bundle. |
| libmp3lame0 | Debian `3.100-6+b3` | Debian copyright metadata: LGPL-2+ | Matching Debian source is pinned and included in the corresponding-source bundle. |
| libsoxr0 | Debian `0.1.3-4+b2` | Debian copyright metadata: LGPL-2.1+ | Matching Debian source is pinned and included in the corresponding-source bundle. |
| libfribidi0 | Debian `1.0.16-1` | Debian copyright metadata: LGPL-2.1+ | Matching Debian source is pinned and included in the corresponding-source bundle. |
| libzimg2 | Debian `3.0.5+ds1-1+b2` | Debian copyright metadata includes GPL-3+ with AutoConf exception and WTFPL-2 | Matching Debian source is pinned and included in the corresponding-source bundle. |
| libaom, dav1d, libvpx, libass, Opus, Vorbis, zimg, WebP, OpenJPEG | Debian packages resolved during image build | Per-package Debian copyright files; some packages contain multiple notices | Runtime package metadata is copied to `/usr/share/licenses/mvforge/debian/` in the backend image. Their exact versions are recorded per image. |

The observed FFmpeg build enables `--enable-gpl`, `--enable-version3`,
`--enable-libx264`, and `--enable-libx265`. It omits `--enable-nonfree`.
`libx264` and `libx265` are dynamically linked into `libavcodec` in the
inspected backend image. The source bundle also includes the matching Debian
sources for libbluray, LAME, SoXR, and FriBidi (LGPL runtime components), plus
zimg (whose package metadata includes GPL-3+ files). `backend/licenses/SOURCE-OFFER.md` describes the release
source-bundle process; it is not itself an offer or evidence for older
distributed images.

## Intel acceleration and OCR/subtitle components

| Component | Version / origin | Notice evidence in the backend image and source bundle |
| --- | --- | --- |
| libva, libva-utils, gmmlib, Intel media-driver, libvpl, libvpl-tools | Git commits pinned in `backend/Dockerfile` | License files are copied from the same checked-out revisions under `/usr/share/licenses/mvforge/` and into the corresponding-source bundle. |
| oneVPL Intel GPU runtime | Pinned `oneapi-src/oneVPL-intel-gpu` Git commit | `LICENSE` is copied from that exact checkout into the runtime image and source bundle. |
| Subtitle Edit SeConv | 5.1.0-rc15 archive pinned by SHA256 | `LICENSE` is extracted from the checksum-verified release archive and included in the runtime image and source bundle. |
| SkiaSharp native assets | NuGet 3.119.4 package pinned by SHA256 | License, notice, copying, and third-party notice files present in the exact package are copied into the runtime image and source bundle. |
| HarfBuzzSharp native assets | NuGet 8.3.1.5 package pinned by SHA256 | License, notice, copying, and third-party notice files present in the exact package are copied into the runtime image and source bundle. |
| Tesseract, Hunspell, language dictionaries | Debian runtime packages | Package copyright material is retained under `/usr/share/licenses/mvforge/debian/` when supplied by Debian. |

The component-specific texts are the evidence. This table does not infer an
entire component's licensing from package metadata alone.

## Application dependencies

- Go: `backend/go.mod` and `backend/go.sum` define the resolved module graph;
  `backend/licenses/GO_MODULES.txt` records `go list -m all` output;
  `GO_LICENSES.csv` records the resolved imported-package license report, and
  `backend/licenses/go-modules/` contains the collected license texts.
- npm: `frontend/package-lock.json` is the resolved package manifest;
  `frontend/licenses/npm/inventory.json` records the production/shared
  lockfile closure, versions, package-declared license fields, and notice file
  paths. `THIRD_PARTY_NOTICES.txt` bundles the discovered license and notice
  texts for those packages. The generator fails release validation when any
  entry lacks license metadata or bundled license text. The root
  [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) records the direct frontend
  dependency audit and UI/template provenance policy.
- Release images: the workflow requests BuildKit SBOM and provenance
  attestations and checks those predicates against each exact image digest.
  Current post-cleanup API lookups confirm both formerly hosted GHCR packages
  return 404. The pre-deletion version inventory remains historical evidence;
  repository package metadata did not expose old image manifests or embedded
  SBOM/provenance. See `docs/compliance/redistribution-audit.md` and both dated
  API snapshots under `docs/compliance/evidence/`.

## Debian package evidence and limits

The backend runtime image copies available Debian package copyright files to
`/usr/share/licenses/mvforge/debian/<package>.copyright`. Base images and the
GPL-sensitive x264/x265 binary/source package versions are digest/version
pinned. Other APT dependencies still resolve from the configured Debian
repositories at build time; exact versions must therefore be taken from the
per-image SBOM or package inventory. The current workflow retains image
digests and BuildKit attestations but has not been run against a future public
release in this audit.

Third-party names and trademarks belong to their respective owners.
