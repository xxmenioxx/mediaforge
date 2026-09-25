## Context

See `proposal.md` for the motivation and scope. The current backend image is a
multi-stage `debian:trixie-slim` image with separate Go, Intel media, FFmpeg,
SeConv, and final runtime stages (`backend/Dockerfile`). The final stage
copies the application binaries, FFmpeg/ffprobe, Intel media prefixes, SeConv
and native libraries, then installs Debian runtime packages for media, OCR,
SQLite, SSH, and healthchecks.

This design audits and optimizes the Linux/Docker image. Its hardware device,
driver, and diagnostic dependencies are Linux image requirements only, not
global MVForge runtime requirements. #150 owns this image scope; #161 owns
native macOS/VideoToolbox runtime capability support; #162 owns cross-platform
Server/Worker role composition and packaging.

The current runtime authority is in the application: FFmpeg encoder listing
and smoke tests, QSV/VAAPI initialization, metric filter/model probes, and
OCR readiness/execution determine actual availability. `RuntimeState` is live
state; `RuntimeSnapshot` is immutable audit evidence. The image must support
these contracts but must not replace them.

The production NAS Compose file maps `/dev/dri` and defines the backend curl
healthcheck; the local development Compose file uses a development frontend
target and does not represent the production hardware deployment. This
distinction is part of validation evidence, not a reason to add image-specific
capability flags.

## Goals / Non-Goals

**Goals:**

- Produce an evidence-backed dependency map before any removal or relocation.
- Keep build toolchains and source trees out of the final runtime image.
- Identify safe opportunities to reduce copied prefixes, duplicate artifacts,
  package footprint, invalidation, and build time, subject to runtime proof.
- Preserve actual FFmpeg/ffprobe, software filters/codecs, and Linux/Docker
  QSV/VAAPI, libva/oneVPL/media-driver loading, plus metric probing,
  OCR/subtitle behavior, healthchecks, backups, remote execution, and
  diagnostics.
- Compare current-origin/main and optimized image size/layers with the same
  reproducible method, or report measurement unavailable.
- Make each implementation task independently verifiable and reversible.

**Non-Goals:**

- Redesigning capability detection, `MetricCapabilities`, runtime policies,
  `RuntimeState`, `RuntimeSnapshot`, retention, or cache-reset semantics.
- Adding a second Docker/image capability authority or hardcoded capability
  flags.
- Redesigning FFmpeg pipelines, QSV policy, OCR, subtitle, translation,
  scheduling, or media-quality behavior.
- Changing persisted models, migrating or cleaning application data, or
  changing production/NAS databases.
- Proving Docker size savings without measurements or changing to a new base
  image solely for theoretical savings.

## Decisions

### 1. Runtime image authority boundary

The image supplies executables, shared libraries, drivers, language data,
device access, and operational utilities. It does not declare them usable.
Existing application probes remain the only authority for actual capabilities:

```text
runtime probe -> capability available/unavailable -> RuntimeState
             -> immutable RuntimeSnapshot at existing audit boundaries
```

`MetricCapabilities` remains the #69 contract: FFmpeg version, filter/provider
evidence, model identity, availability, reason, and runtime identity are
preserved. #149 semantics remain unchanged: live detection upserts
`RuntimeState`; only explicit audit boundaries create `RuntimeSnapshot`; stable
fingerprints, ExecutionPlan/Test Encode freezes, retention, refresh, and both
cache resets remain application-owned.

An image change may cause a different probe result. That is an expected runtime
observation, not permission to alter probe logic or infer a result from an apt
package list.

### 2. Dependency map and evidence gate

The following is the initial map from the read-only audit. It is a planning
baseline, not proof that any candidate is removable. Implementation must extend
it with command output, ELF closure, and container-runtime evidence before
marking a dependency removable.

| Component | Package/binary/library | Consumer or probe | Build-time or runtime | Status | Evidence | Proposed treatment |
| --- | --- | --- | --- | --- | --- | --- |
| Go API | Go/CGo toolchain, `gcc`, `libsqlite3-dev` | `go build` in `build` stage | Build-time | Required build-only | `backend/Dockerfile:1-14` | Keep in build stage; never install in final image. |
| Intel media build | CMake, Meson, Ninja, Git/LFS, headers, `libdrm-dev`, X/Wayland development packages, Python | libva, libva-utils, gmmlib, media-driver, libvpl, libvpl-tools, oneVPL GPU builds | Build-time | Required build-only | `backend/Dockerfile:16-121` | Keep isolated; remove only from intermediate layers where builder design permits. |
| FFmpeg build | FFmpeg/SVT-AV1 compilers, headers, `*-dev`, NASM/YASM, CMake, pkg-config | FFmpeg and SVT-AV1 build/configure | Build-time | Required build-only | `backend/Dockerfile:123-202` | Keep isolated; consider separate SVT-AV1/cache stages for build reuse. |
| SeConv build/download | `curl`, `unzip`, archive and NuGet extraction tools | SeConv and native Skia/HarfBuzz extraction | Build-time | Required build-only | `backend/Dockerfile:204-233` | Keep out of final image except copied `seconv` and native libraries. |
| FFmpeg tools | `ffmpeg`, `ffprobe` and `/opt/ffmpeg/lib` | Scanning, previews, conversion, validation, workers, subtitle/OCR extraction, runtime and metric probes | Runtime | Required | `backend/Dockerfile:256-257`; numerous `exec.Command*` consumers | Retain binary discovery, shared-library closure, filters/codecs, licenses; prove any pruning with `ldd`, loader, and functional tests. |
| Software FFmpeg codecs/filters | libx264, libx265, SVT-AV1, AOM, dav1d, VPx, AVIF/OpenJPEG, ASS/font, audio, zimg, OpenCL/DRM libraries | Media plans, previews, audio, subtitle and quality validation | Runtime | Required by feature/use | `backend/Dockerfile:240-248`; FFmpeg configure flags `:161-190` | Retain all evidenced providers; do not infer unused from one test path. Validate exact filters/codecs used by current plans. |
| Linux VAAPI/Intel stack | libva, libdrm, `iHD_drv_video.so`, gmmlib/media-driver, oneVPL/libmfx-gen, driver metadata | Linux FFmpeg QSV/VAAPI initialization, `mvforge-hw-probe`, `/dev/dri` execution | Linux/Docker runtime, dynamically loaded | Required only when the Linux hardware path is configured; optional on CPU-only Linux hosts; not a global MVForge requirement | `backend/Dockerfile:256-260`; `capabilities/ffmpeg.go`; `cmd/hwprobe` | Retain in the Linux/Docker image and validate with device-enabled probes; package presence is insufficient. Native macOS/VideoToolbox support is owned by #161. |
| Linux Intel diagnostics | `vpl-inspect`, `vainfo` and their dependencies | Linux `mvforge-hw-probe` diagnostics; direct in-repo caller of installed probe binary not confirmed | Linux/Docker runtime diagnostic/operational | Optional Linux diagnostics; not global MVForge requirements | `backend/Dockerfile` builds/copies VPL tooling; `cmd/hwprobe/main.go` invokes tools | Retain in the Linux image unless explicit diagnostic compatibility evidence permits pruning. Cross-platform Server/Worker packaging is owned by #162. |
| Metric providers | FFmpeg `ssim`, `psnr`, `xpsnr`, `libvmaf`/MS-SSIM provider and model identity | `ProbeMetricCapabilitiesWithRunner`, quality evidence | Runtime/dynamically loaded | Required where configured; optional provider/model | `backend/internal/capabilities/metrics.go`; #69 tests | Preserve existing probe contract. Verify actual filter/model availability; do not add or remove a provider solely from Docker assumptions. |
| OCR engine | `tesseract` and `tesseract-ocr` runtime support | Bitmap OCR readiness and execution | Runtime | Required for bitmap OCR | `ocr_lifecycle.go`, `subtitle_ocr.go`; Docker `:247-252` | Retain and run list-languages/readiness checks. Missing engine must remain an explicit existing failure. |
| OCR language data | `tesseract-ocr-eng`, `-spa`, `-jpn`, `-jpn-vert` and traineddata | Requested language validation and Japanese fallback | Runtime data | Required per configured workflow; packs optional by language | `subtitle_ocr.go:407-546`; Docker `:247-252` | Retain current supported base packs unless usage evidence and product decision explicitly narrow support; validate each configured pack. |
| Bitmap OCR converter | `seconv` plus `libSkiaSharp.so`, `libHarfBuzzSharp.so` | PGS/image subtitle OCR, canonical `both` artifacts | Runtime/dynamically loaded | Required for bitmap OCR | `subtitle_ocr.go:585-776`; Docker `:257-260` | Retain binary/native closure and version compatibility. No parallel OCR path. |
| OCR dictionaries | `hunspell-en-us`, `hunspell-es` `.dic/.aff` data | Optional OCR scoring/correction lookup | Runtime optional data | Optional | `subtitle_ocr.go:778-788, 881-957`; Docker `:247-248` | Retain current dictionaries until scoring behavior is tested; absence must use existing optional degradation. |
| Subtitle text formats | ASS/SSA/SRT handling plus FFmpeg conversion support | Subtitle transform, validation, publication, translation | Runtime | Required by enabled workflows | `subtitle_transform.go`, `subtitle_operation.go`, `translation_*` | Preserve native extensions, canonical SRT/ASS behavior, bitmap paths, and sidecar validation. |
| Local translation boundary | Ollama endpoint is configured externally; OCR/FFmpeg/sidecar tools are local | Text/bitmap translation workflows | Runtime external/optional | Optional when enabled | `translation_provider_factory.go`, `translation_ollama.go` | Do not package a new translation service or redesign architecture; validate local prerequisites and external-provider degradation separately. |
| SQLite library | `libsqlite3-0` | Go SQLite driver/application | Runtime library | Required | `backend/Dockerfile:241-247`; backend database package | Retain and validate API startup/migrations in a disposable DB. |
| SQLite CLI | `sqlite3` | `deploy/nas/backup.sh` `.backup` command | Runtime operational | Required for backup path until external workflow disproves it | `deploy/nas/backup.sh:9` | Retain; no removal based on absent Go import. |
| Backend healthcheck | `curl` | Dockerfile and NAS Compose `/health` | Runtime operational | Required | `backend/Dockerfile:288-289`, `deploy/nas/compose.yml:20-26` | Retain or replace only with an equivalent validated command in the same scope; do not broaden liveness semantics. |
| Frontend production healthcheck | `wget` | Nginx production image `/health` | Runtime operational | Required in frontend production target | `frontend/Dockerfile:29-37` | Retain in frontend image; unrelated to backend package pruning. |
| Remote executor | `openssh-client`/`ssh` | Configured remote command execution | Runtime optional | Required when configured | `remote_executor.go:369`; Docker `:241` | Retain until remote executor validation proves an alternative; absence must fail the existing readiness path. |
| Shell/base utilities | `sh`, `date`, `mkdir`, `test`, `grep`, `printf`, loader tools | Entrypoint, backup, healthcheck, release/deploy scripts | Runtime/operational | Required | `deploy/nas/backup.sh`, Docker healthcheck, deployment scripts | Preserve Debian base utilities; do not use Go import search as removal evidence. |
| Timezone data | `tzdata` | Runtime timestamps and configured `TZ` | Runtime | Required for configured timezone behavior | Docker `:248`; NAS Compose `TZ` | Retain unless a measured and behaviorally equivalent alternative is proven. |
| macOS sleep inhibitor | `caffeinate` | Optional native macOS worker path | Native macOS runtime | Outside #150 Linux/Docker image scope | `worker_processes.go:59-72` | Do not classify as a Linux image dependency; native macOS runtime support is owned by #161 and cross-platform role packaging by #162. |

This map intentionally leaves `sqlite3` operational status, full ELF closures,
VPL/VAAPI diagnostic use, and actual libvmaf packaging as evidence tasks rather
than making unsafe assumptions. The audit also found a trailing space after a
Dockerfile continuation at the runtime package install; it is a build-syntax
risk to verify, not a license to broaden the change.

### 3. Build/runtime separation and optimization order

The existing multi-stage shape is preferred because it already excludes Go,
Intel, FFmpeg, and SeConv toolchains from the final stage. The implementation
will proceed in this order:

1. Capture the baseline and complete the dependency/ELF/loader inventory.
2. Make only low-risk build improvements first: cache mounts, narrower build
   context, independent cacheable build stages, apt-list cleanup, and removal
   of duplicate build artifacts that are proven not to be copied.
3. Evaluate final-prefix pruning. Preserve executables, shared objects,
   VAAPI driver plugins, VPL/FFmpeg runtime metadata, fonts/models/data, and
   licenses required by dynamic loading. `ldd` output alone is not sufficient
   for plugin-loaded drivers; run the existing probes and feature tests too.
4. Consider package cleanup only after all Go, shell, Docker, diagnostic,
   backup, and remote-command consumers are mapped.
5. Reject a base-image change unless the same capability matrix and measured
   image/layer comparison demonstrate no unacceptable tradeoff.

Build-cache improvements may reduce build time without changing final image
size. They must be reported separately from final-size changes.

### 4. FFmpeg and hardware validation matrix

The post-change validation uses the same container target and arguments as the
baseline and distinguishes these results:

| Check | Evidence required |
| --- | --- |
| Binary discovery | `ffmpeg -version` and `ffprobe -version` succeed through the existing `PATH`; expected version/build identity is recorded. |
| Software contract | `ffmpeg -filters`, `-encoders`, and representative current filter/codec smoke commands cover field/cadence, scaling/crop/format, subtitle/font, audio, x264/x265/SVT-AV1, and metric-related filters. |
| QSV/VAAPI | In a `/dev/dri`-enabled runtime, existing `CheckEncoder`/hardware smoke probes and `mvforge-hw-probe` (including `vainfo`/`vpl-inspect` when that path is supported) report actual results. In a CPU-only runtime, explicit unavailable degradation is expected. |
| Dynamic loader | `ldd`/loader inspection and actual execution verify FFmpeg, FFprobe, Intel drivers, VPL, and SeConv native libraries; no missing transitive library is hidden by `/health`. |
| Encoder cache | Existing encoder cache reset causes the current probe to re-run and preserves the existing result shape; no new cache authority is added. |
| Metric capabilities | Existing `MetricCapabilities` tests and runtime probe cover version, filters, libvmaf/model identity, unavailable reasons, cache, and reset behavior. |

`ffmpeg` starting successfully is only one row in this matrix. Hardware
device access and per-mode smoke tests remain necessary.

### 5. OCR, subtitle, and translation validation matrix

Use disposable fixtures and existing focused tests/commands. Verify Tesseract
binary and `--list-langs`, each currently shipped language pack required by
the configured fixture, SeConv version/launch/native libraries, and Hunspell
lookup when dictionaries are present. Exercise native ASS/SSA/SRT, canonical
text conversion, bitmap OCR, logical `both` reuse, sidecar validation, and
configured translation readiness. Verify absent optional Hunspell or language
packs produce existing degradation/readiness errors and do not create false
artifacts. Do not add a new resolver or new translation provider.

### 6. Operational validation matrix

Run the final backend image’s existing `/health` check with `curl`; the
production frontend target’s `/health` check with `wget`; the NAS backup script
against a disposable SQLite database and Compose service; SSH version/command
readiness when remote execution is configured; and the shipped hardware probe
diagnostic path. Check startup and runtime refresh in a disposable application
database. `/health` is liveness-only and must not be changed into a broad
capability test.

### 7. Reproducible measurement

The implementation must capture a BEFORE record from the current
`origin/main` image before any #150 Docker/runtime change. The record should
include:

- exact Git SHA, target platform, Docker/buildx version, Dockerfile target,
  build arguments, and cache/pull policy;
- final image size from `docker image inspect`;
- full relevant layer history from `docker history --no-trunc` and layer IDs;
- package inventory/installed-size observations from a disposable container,
  plus file-size summaries for `/opt/ffmpeg`, `/opt/intel`, `/usr/local/lib`,
  OCR data, and application binaries where practical;
- the capability/operational validation results above.

Build the optimized image with the identical measurement inputs and capture
the AFTER record using the same commands. Report:

```text
before = captured final image size
after = captured final image size
absolute delta = after - before
percentage delta = (after - before) / before * 100
```

Report relevant layer additions/removals and distinguish compressed registry
size from local image size if both are captured. If Docker, image inspection,
or runtime execution is blocked, record exactly which measurement/check is
unavailable and why; do not estimate savings.

### 8. #149 interaction and rollback

No database migration is part of this change. A rollback is a normal image
redeploy to the previous digest/tag with the existing persistent data volume
left untouched. Runtime refresh may be run after redeploy to repopulate live
state and existing capability caches. Existing immutable snapshots and plan/
Test Encode references remain valid historical evidence regardless of which
image later produced a different live capability result.

If an implementation candidate changes runtime capabilities or cannot pass the
matrix, revert that candidate independently. Do not repair a failed image by
deleting snapshots, changing fingerprint rules, altering retention, or
rewriting persisted application data.

## Risks / Trade-offs

- **[Dynamic dependency omission]** → A library or driver can be loaded by
  plugin discovery rather than `ldd`; retain full candidates until actual
  probes and feature tests pass in a device-enabled runtime.
- **[False green healthcheck]** → `/health` is liveness-only; keep it as a
  separate operational check and run capability validation independently.
- **[Hardware unavailable in CI]** → Run CPU/software and mocked/probe-fixture
  checks where hardware is unavailable, and explicitly report real QSV/VAAPI
  execution as unavailable rather than claiming it passed.
- **[Metric provider drift]** → Preserve `MetricCapabilities` probe and cache
  contracts; record filter/model evidence and report provider degradation.
- **[OCR language regression]** → Validate exact language data and SeConv
  native libraries with disposable fixtures before pruning any pack.
- **[Backup/remote regression]** → Validate SQLite CLI and SSH paths from the
  actual operational scripts, not from source-import searches.
- **[Build-cache changes alter reproducibility]** → Keep cache policy explicit
  in the measurement record and compare with identical build inputs.
- **[Base-image change expands scope]** → Do not change base image unless the
  complete contract and measured tradeoff are demonstrated; otherwise retain
  `debian:trixie-slim`.

## Migration Plan

1. Preserve the current image tag/digest as the rollback target and capture the
   current-origin/main BEFORE record.
2. Implement one bounded image/build change at a time, with dependency-map
   evidence and focused validation before proceeding to the next candidate.
3. Build and inspect the optimized image using the same target/platform and
   capture the AFTER record.
4. Deploy only the resulting image artifact; do not migrate or edit the
   persistent database for image optimization.
5. Run explicit runtime refresh/cache-reset validation if the deployment
   exposes a changed probe result, then retain existing RuntimeSnapshots and
   audit references.
6. If any required contract fails, redeploy the previous image independently
   of application data and revert the failing image change.
