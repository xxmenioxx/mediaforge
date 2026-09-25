# #150 Phase 1 baseline and inventory record

Captured on 2026-09-25 before any #150 Dockerfile or runtime dependency
changes.

## Source and build context

- Authoritative baseline: `origin/main` at
  `d793cc467831f189fb71afcf68aa1c49f7db10dc`.
- Working branch before implementation: `feat/150-runtime-image-optimization`;
  it was exactly one OpenSpec commit ahead of that baseline.
- `backend/Dockerfile` SHA-256 matched `origin/main`:
  `a190403cc82d612f7c5df66c61fac6a22ff181051c8c07ff91812feb2ffa1bd3`.
- Host: Darwin x86_64; intended release platform is `linux/amd64` from
  `.github/workflows/release-images.yml`.
- Docker client: `29.6.1`.
- Docker buildx: `v0.35.0-desktop.2`.
- Docker context: `desktop-linux`.
- Docker server: unavailable. The configured socket
  `unix:///Users/anuelvs/.docker/run/docker.sock` did not exist, so no image
  could be built, inspected, or executed.
- Build target: backend default final stage; frontend production target was
  also identified for the healthcheck contract.
- Baseline pull/cache policy: no local daemon was available to capture a
  runtime build. CI/release configuration uses BuildKit GHA cache scopes
  (`backend`, `web`) and release `linux/amd64`; no local override was
  substituted.

## Dockerfile defaults

The baseline uses five backend stages: `build`, `intel-media`, `ffmpeg`,
`seconv`, and the final `debian:trixie-slim` runtime. The pinned defaults are:

- Intel build arguments: `LIBVA_COMMIT=217da1c28336d6a7e9c0c4cb8f1c303968a675f`,
  `LIBVA_UTILS_COMMIT=1a13ae13382b7b548f3a7e8035e1d7df66662c0a`,
  `GMMLIB_COMMIT=acee26b150043058f722920c8090187426e4abba`,
  `MEDIA_DRIVER_COMMIT=c6ab0c941edfedf3ce79fa4dd7756f3369902e8d`,
  `LIBVPL_COMMIT=c45b5d786bf7cdabbe49ff1bab78693ad78feb78`,
  `LIBVPL_TOOLS_COMMIT=ad9deecaf0ee76f689373b55fe620c843e3c8a4d`, and
  `VPL_GPU_COMMIT=e61f5633b0afee74639285e8d201fb803aeacb2f`.
- FFmpeg `8.1.2`; SVT-AV1 `4.1.0`.
- SeConv `5.1.0-rc15`; SkiaSharp native assets `3.119.4`;
  HarfBuzzSharp native assets `8.3.1.5`.
- No build arguments or target overrides were applied during this baseline
  record.

## Measurements unavailable

The following BEFORE measurements are unavailable because the Docker daemon
socket was absent:

- final backend/frontend image size;
- `docker history --no-trunc`, layer IDs, and layer sizes;
- installed Debian package inventory and installed sizes;
- runtime `/opt/ffmpeg`, `/opt/intel`, and `/usr/local/lib` sizes/content;
- `ldd`/loader closure for FFmpeg, Intel/VAAPI/VPL, SeConv, and native OCR
  libraries;
- container execution, software capability probes, QSV/VAAPI probes,
  Tesseract/SeConv checks, healthchecks, backup, and SSH runtime checks.

No size or percentage estimate is made.

## Static dependency evidence

- Build-only stages contain Go/CGo, Intel, FFmpeg/SVT-AV1, and SeConv
  compilers, headers, source trees, package managers, and archive tools;
  none are copied into the final stage.
- The final stage copies `mvforge-api`, `mvforge-hw-probe`, FFmpeg/ffprobe,
  `/opt/ffmpeg/lib`, the complete `/opt/intel` prefix, SeConv, and native
  Skia/HarfBuzz libraries.
- Runtime packages include media shared libraries, `libsqlite3-0`, `sqlite3`,
  `curl`, `openssh-client`, Tesseract and `eng`/`spa`/`jpn`/`jpn_vert` data,
  Hunspell `en`/`es`, OpenCL/DRM support, and `tzdata`.
- `curl` is consumed by the backend healthcheck; `wget` is consumed by the
  production frontend healthcheck; `sqlite3` is consumed by the NAS backup;
  SSH is consumed by remote execution; `vainfo` and `vpl-inspect` are invoked
  by `mvforge-hw-probe`.
- FFmpeg is configured with libVPL, VAAPI, libdrm, and software codec/filter
  providers; `MetricCapabilities` probes actual FFmpeg filters/provider/model
  evidence and remains authoritative.
- Production NAS Compose maps `/dev/dri`; local development Compose does not.

## Phase 1 treatment decision

No runtime dependency, copied prefix, driver, plugin, language pack, native
library, or operational binary is removed, relocated, or pruned in this
implementation without Docker/loader evidence. The implementation is limited
to BuildKit cache mounts and conservative build-context exclusions that do not
change final runtime contents. Runtime image measurements and hardware/loader
validation remain explicit follow-up requirements until a Docker daemon and
appropriate hardware runtime are available.
