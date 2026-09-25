# #150 baseline and runtime evidence

Captured on 2026-09-25. The recorded BEFORE source is the unmodified
`origin/main` commit `d793cc467831f189fb71afcf68aa1c49f7db10dc`. The AFTER
image was built from the #150 implementation commit
`ca73aa2b750d42a35801026757804f6eb45c8b8f`; the later evidence-only changes
to this OpenSpec record do not change Docker inputs.

## Historical daemon limitation

The first Phase 1 capture was made while the configured Docker socket was
absent. That historical record correctly had no image measurements. Docker is
now available, so the measurements and runtime evidence below supersede the
unavailable portions of that record.

## Reproducible measurement method

- Docker client/server: `29.6.1` / `29.6.1`.
- Buildx: `v0.35.0-desktop.2` (`b554ce1decd8b509893b1e7c6227eabfb923d094`).
- Docker context: `desktop-linux`; target platform: `linux/amd64`.
- Backend target: default final stage; frontend target: `production`.
- Build arguments: none supplied; Dockerfile defaults were used.
- Pull policy: `--pull=false`.
- Cache policy: the local Docker BuildKit cache only; no external or GHA
  cache was used for these measurements.
- Images were loaded into the local daemon with `--load`.
- Backend command, run from `backend/`:

  ```text
  docker build --progress=plain --platform linux/amd64 --pull=false --load \
    -t <before-or-after-backend-tag> .
  ```

- Frontend command, run from `frontend/`:

  ```text
  docker build --progress=plain --platform linux/amd64 --pull=false --load \
    --target production -t <before-or-after-web-tag> .
  ```

The BEFORE images were built from an isolated archive of the exact baseline
commit, not from the feature worktree. `docker image inspect`,
`docker history --no-trunc`, disposable `docker run`, package inventory, and
prefix-size commands were run against both image pairs.

## Measured image results

| image | source | image ID | size (bytes) | size (MiB) | layers |
| --- | --- | --- | ---: | ---: | ---: |
| backend BEFORE | `d793cc4` | `sha256:4a259805d46ed41cd59274ac2562c0e22ee0d981364f49afe4e7c81823c98eea` | 444,810,619 | 424.20 | 16 |
| backend AFTER | `ca73aa2` | `sha256:ab125398061d5338a25cabc4deb47011def074987288a989b29a3ec1282fa9e7` | 440,814,792 | 420.39 | 16 |
| frontend BEFORE | `d793cc4` | `sha256:d29924b9ccc969e0b3cab05b2b23f272d89734f6834f813df357cd00cb032850` | 26,394,520 | 25.17 | 10 |
| frontend AFTER | `ca73aa2` | `sha256:05dd2653b7336897df47df9d1c38bc7774274da35a9c0c012f41b72654189550` | 26,394,506 | 25.17 | 10 |

| comparison | BEFORE | AFTER | absolute delta | percentage delta |
| --- | ---: | ---: | ---: | ---: |
| backend | 444,810,619 | 440,814,792 | -3,995,827 bytes | -0.898321% |
| frontend | 26,394,520 | 26,394,506 | -14 bytes | -0.000053% |
| combined | 471,205,139 | 467,209,298 | -3,995,841 bytes | -0.848005% |

The backend AFTER layer history retains the same 16-layer structure and the
same copied-prefix layer sizes: API 43.1 MB, hardware probe 3.21 MB, FFmpeg
binary 668 kB, FFmpeg libraries 39 MB, Intel prefix 562 MB, SeConv 80.7 MB,
native libraries 14 MB, and the license layers. The reported runtime package
layer is 177 MB BEFORE and 165 MB AFTER; the Debian base layer is 87.4 MB
BEFORE and 87.6 MB AFTER. The frontend retains the same Nginx base/config/
distribution layer sizes and 10-layer shape.

The Debian `trixie-slim` tag resolved to different base snapshots during the
two daemon builds (`@1783900800` BEFORE and `@1789689600` AFTER). The runtime
Dockerfile package list and copied runtime artifacts were not changed by
#150, so the backend byte delta cannot be attributed solely to the cache and
context optimizations; it includes the observed base/package snapshot drift.
The measured deltas are reported as required, without treating them as an
estimated optimization saving.

## Package and prefix evidence

| observation | BEFORE | AFTER |
| --- | ---: | ---: |
| installed Debian packages | 190 | 190 |
| `dpkg` installed size | 282,653 kB / 276.03 MiB | 282,859 kB / 276.23 MiB |
| `/opt/ffmpeg` | 38M | 38M |
| `/opt/intel` | 536M | 536M |
| `/usr/local/lib` | 14M | 14M |

The relevant package identities and installed sizes remained present AFTER:

- `curl`, `ca-certificates`, and `openssh-client` for health and remote
  execution;
- `sqlite3` and `libsqlite3-0` for backup and the Go SQLite driver;
- `libdrm2`, `ocl-icd-libopencl1`, the Intel prefix, and the VAAPI/VPL
  libraries and driver;
- FFmpeg shared-library dependencies including libass, libfreetype,
  libfontconfig, libfribidi, libx264, libx265, libvpx, libopus, libvorbis,
  libaom, libdav1d, libopenjp2, libzimg, and libsoxr;
- `tesseract-ocr`, `libtesseract5`, `tesseract-ocr-eng`, `-spa`, `-jpn`,
  `-jpn-vert`, and the required OSD data;
- `hunspell-en-us` and `hunspell-es`, with dictionary absence remaining an
  optional degradation path.

The final image contains no Go/CGo compiler, CMake, Make, Git, pkg-config,
NASM, or copied source/header tree from the build stages. The `perl` executable
observed in the Debian runtime is a transitive base/runtime package, not a
copied build toolchain. No dependency or copied runtime prefix was removed or
relocated by #150.

## Runtime dependency map evidence

| component | package/binary/library | consumer/probe | phase | status | evidence and treatment |
| --- | --- | --- | --- | --- | --- |
| API build | Go/CGo toolchain and module/build caches | backend build stage | build-time | required for build | not present in final image; cache mounts retained |
| Intel build | libva, gmmlib, media-driver, libvpl, VPL tools, VPL GPU build dependencies | Intel build stages; final `/opt/intel` | build-time + runtime prefix | required runtime | final prefix and `iHD_drv_video.so` retained; loader and probe paths checked |
| FFmpeg build | FFmpeg/SVT-AV1 compilers, headers, source, archives | FFmpeg build stage; final `/opt/ffmpeg/bin` and `lib` | build-time + runtime prefix | required runtime | build-only inputs do not enter final image; FFmpeg and ffprobe execute and load successfully |
| FFmpeg/media | `ffmpeg`, `ffprobe`, libva, libdrm, libvpl, Intel driver, FFmpeg shared libs | Go execution paths, `mvforge-hw-probe`, capability probes | runtime | required/optional by capability | software filters/codecs and loader closure pass; NAS `/dev/dri` validation also passes through the existing probes |
| metrics | SSIM/PSNR/XPSNR FFmpeg filters; optional libvmaf/model/provider evidence | #69 `MetricCapabilities` probe and cache/reset paths | runtime | SSIM/PSNR/XPSNR required; VMAF/provider optional | filters execute; libvmaf filter is absent and remains an explicit unavailable result; no package flag is added |
| OCR | `tesseract`, `libtesseract5`, `eng`/`spa`/`jpn`/`jpn_vert`/OSD data | OCR handlers and bitmap subtitle workflow | runtime | required where OCR configured | version, language list, and data presence pass; no language pack removed |
| subtitle rendering | libass, fontconfig, freetype, fribidi, FFmpeg ASS/SSA/SRT support | subtitle extraction/rendering and sidecar paths | runtime | required | ASS and SRT render smoke checks pass; SSA uses the same FFmpeg subtitle path |
| SeConv | `/usr/local/bin/seconv`, `libSkiaSharp.so`, `libHarfBuzzSharp.so` | subtitle conversion/tooling | runtime | required | SeConv help/version works; native loader closure has no unresolved libraries |
| Hunspell | `hunspell-en-us`, `hunspell-es` dictionaries | OCR scoring/correction | runtime | optional data | dictionaries are present; missing dictionaries continue to degrade through existing behavior |
| backup | `sqlite3`, `libsqlite3-0`, `/bin/sh`, `test`, `mkdir` | `deploy/nas/backup.sh` inner command | runtime/operational | required | disposable `.backup` produced and reopened successfully; production data untouched |
| health | `curl`, `wget` | backend and frontend Docker healthchecks | runtime/operational | required | both production healthchecks report healthy on a disposable network |
| remote execution | `openssh-client` and `ssh` | Go `exec.Command` remote executor | runtime/operational | required when configured | `ssh -V` and binary path pass; no configured disposable remote endpoint was available |
| diagnostics | `mvforge-hw-probe`, `vainfo`, `vpl-inspect` | hardware diagnostic path | runtime/diagnostic | optional hardware | binaries are present; local no-device probing returns structured unavailable results, while NAS device-enabled probes pass |

No relevant dependency remains classified as an unknown removable candidate.
Hardware usability is an unavailable environment capability, not evidence that
its runtime libraries can be deleted.

## Build/cache observations

The backend build context transferred approximately 4.98 MB for the exact
baseline source and 2.87 MB for the optimized source after the `.dockerignore`
exclusions. The frontend context was approximately 2.44 MB in both captures.
The Go build step logged 272.5 seconds in the baseline build and 128.8
seconds in the optimized build with the cache mounts. These are observed step
times from separate builds with different cache state, not a controlled
benchmark and are reported separately from final image size. No runtime
package, binary, driver, model, or copied-prefix optimization was accepted
without evidence.

## Functional runtime evidence

- `ffmpeg` 8.1.2 and `ffprobe` 8.1.2 are discoverable using the image PATH;
  both loader closures report zero `not found` entries.
- Required filters `fieldmatch`, `bwdif`, `decimate`, `setfield`, `crop`,
  `format`, `subtitles`, `ass`, `ssim`, `psnr`, `xpsnr`, `aresample`, and
  `loudnorm` are listed. `libx265`, `libsvtav1`, AAC, Opus, QSV, and VAAPI
  encoders are listed. Software IVTC/x265, SVT-AV1, audio, SRT, ASS, metric,
  and ffprobe smokes pass.
- `tesseract --list-langs` reports `eng`, `spa`, `jpn`, `jpn_vert`, and `osd`.
  SeConv reports version 5.0.0/help successfully. SkiaSharp and HarfBuzzSharp
  native libraries load with no unresolved dependencies.
- Backend and frontend Docker healthchecks both become `healthy`; `/health`
  returns HTTP 200. A disposable SQLite database is backed up and reopened
  with the same command shape used by the NAS backup script.
- The disposable API returns runtime state and accepts explicit runtime
  refresh with HTTP 200. Focused #69/#149 capability/runtime tests pass, so
  MetricCapabilities, RuntimeState, RuntimeSnapshot, cache-reset, and refresh
  semantics remain application-owned rather than image-owned.

## NAS device-enabled runtime evidence

The local Docker host used for the BEFORE/AFTER image measurements did not
expose `/dev/dri`; that historical limitation remains recorded above. The
production-style NAS validation was subsequently run on `MenioServer` using
the optimized backend image in a disposable container, without production
mounts:

- NAS Docker Engine is `29.6.2` on `linux/amd64`.
- `/dev/dri/renderD128` exists with `root:render` ownership and `0660` mode.
- `vainfo --display drm --device /dev/dri/renderD128` succeeds with VA-API
  1.22, libva 2.22, and Intel `iHD` media driver 25.2.6; H.264 and HEVC
  encode entrypoints are exposed.
- `vpl-inspect` succeeds with the `mfx-gen` hardware implementation, VAAPI
  acceleration, Intel vendor `0x8086`, and API 2.15.
- Direct disposable H.264 smoke encodes return zero for both `h264_vaapi` and
  `h264_qsv`. `mvforge-hw-probe` reports the render device readable/writable,
  VAAPI `basic_usable`, QSV `fully_usable`, and successful H.264/HEVC/ICQ
  probes.
- The disposable API reports `gpuDetected=true`; capability availability and
  unavailability remain produced by the existing runtime probes, not by image
  package presence or Docker flags.

The optimized image reports FFmpeg 8.1.2, SSIM/PSNR/XPSNR available through
the existing `MetricCapabilities` model, VMAF unavailable because no usable
provider/model identity is present, and MS-SSIM unavailable because no verified
provider exists. These are explicit model results, not validation failures.

## NAS OCR, subtitle, and operational evidence

- Tesseract 5.5.0 runs in the optimized image and `--list-langs` reports
  `eng`, `spa`, `jpn`, `jpn_vert`, and `osd`. Disposable PBM invocations exit
  successfully for each configured language. SeConv reports version 5.0.0.
  `libSkiaSharp.so` and `libHarfBuzzSharp.so` have no unresolved loader
  dependencies. The explicit missing-tool failure branch was not separately
  exercised because no repository fixture exists for it and the NAS image was
  not modified.
- The image contains `en_US` and `es_ES` Hunspell dictionary/affix pairs.
  Focused OCR scoring and dictionary-evidence tests pass. Missing-dictionary
  degradation was not simulated because no repository fixture exists and the
  NAS installation was not modified.
- Focused disposable-fixture tests pass for SRT, ASS, SSA, PGS/VobSub bitmap
  OCR, canonical `both` reuse, sidecar validation/publication, and configured
  text/bitmap translation preparation. No repository bitmap media fixture was
  available beyond those existing test fixtures.
- A disposable NAS container created and reopened a SQLite backup containing
  probe value `150`; the backup size was `528384` bytes. `sqlite3`, `ssh`,
  `sh`, `mkdir`, and `test` are present, and the `mvforge-nas` SSH readiness
  command succeeds. No production database or media was mounted.
- The optimized disposable API starts, serves `/api/system/runtime`, and
  accepts `POST /api/system/runtime/refresh`. Refresh changes `detectedAt`
  while preserving the stable metric identity.
- Focused `runtimeinfo`, `handlers`, and `scheduler` tests pass for stable
  fingerprints, RuntimeState upsert, immutable snapshot references/retention,
  ExecutionPlan/Test Encode freeze, and the existing cache reset paths.
