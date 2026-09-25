## Why

MediaForge’s backend image combines several large build toolchains with a
runtime that must preserve FFmpeg/ffprobe, Intel media acceleration, metric
probes, OCR, subtitle conversion, and operational tooling. The image has not
yet had a consumer-by-consumer dependency audit, so removing packages or
changing stages without evidence could silently turn a capability into a
startup-only or package-presence assumption. Issue #150 establishes a bounded,
measurable audit and safe optimization plan on the runtime architecture
introduced by #69 and #149.

## What Changes

- Inventory Docker stages, package installs, copied artifacts, runtime-loaded
  libraries, shell scripts, healthchecks, and Go external-command consumers.
- Classify dependencies as build-only, required runtime, optional runtime,
  dynamically loaded runtime, diagnostic/operational, or unresolved; an
  unresolved dependency is not removable.
- Define the backend runtime contract for FFmpeg/ffprobe, QSV/VAAPI/libva,
  oneVPL and the Intel media driver, metric providers, Tesseract/language
  data, Hunspell, SeConv, subtitle conversion, healthchecks, and backups.
- Identify bounded image improvements such as build/runtime separation, cache
  and layer cleanup, duplicate artifact removal, reduced build context, and
  proven package cleanup without changing application policy.
- Preserve runtime probing as the authority for actual capability availability;
  installed packages and image composition must not create a second authority.
- Add reproducible pre/post image-size, layer, package-footprint, and runtime
  capability validation requirements. Measurements are reported only when
  actually captured with the same method before and after implementation.
- Define optional-capability degradation, rollback, #149 snapshot interaction,
  and follow-up handling for findings outside image/dependency scope.

No production code, Dockerfile, Compose file, runtime dependency, persisted
data, or capability-detection behavior is changed by this SDD itself.

## Capabilities

### New Capabilities

- `runtime-image-optimization`: Defines the observable runtime image
  dependency contract, build/runtime separation, optional degradation,
  measurement methodology, and regression evidence required for safe image
  optimization.

### Modified Capabilities

None. The existing #69 metric capability contract and #149 RuntimeState /
RuntimeSnapshot contract remain authoritative and unchanged; this change only
specifies how an image optimization must preserve them.

## Impact

- `backend/Dockerfile`, backend runtime/build stages, and the NAS Compose
  healthcheck/backup environment are the likely implementation surfaces.
- Go consumers under `backend/internal/capabilities`,
  `backend/internal/runtimeinfo`, handlers, scheduler/runtime probes, and
  operational scripts provide the evidence and regression-test surfaces.
- Final image size, layer composition, build cache behavior, and optional
  capability availability may change after implementation; persisted database
  rows, existing RuntimeSnapshots, media provenance, and execution policy must
  remain compatible.
- No database migration or application API redesign is expected merely for
  image optimization.

## Scope Boundaries

This change owns Docker image composition, dependency audit, build/runtime
separation, image/layer optimization, runtime dependency validation, and
reproducible size measurement. It does not own capability architecture,
RuntimeState or RuntimeSnapshot semantics, FFmpeg pipeline or QSV policy,
OCR/subtitle/translation architecture, worker scheduling, or media-quality
policy. Desirable findings outside those boundaries become follow-up work.
