## 1. Baseline and dependency inventory

- [x] 1.1 Capture the current `origin/main` backend/frontend image baseline before any #150 implementation, recording Git SHA, target platform, Docker/buildx version, build arguments, cache/pull policy, final image size, `docker history`, layer IDs, package inventory, and relevant prefix/file sizes; verify the record is reproducible and contains no estimated values. Docker server/image measurements are explicitly recorded unavailable in `baseline.md` because the configured daemon socket did not exist.
- [x] 1.2 Complete the dependency map for every Docker stage, installed package, copied artifact, shared library, driver/plugin, language/model file, healthcheck command, backup/restore utility, shell utility, diagnostic binary, and Go external-command consumer; verify every entry has consumer, stage, required/optional/unknown status, evidence, and treatment. Static source/consumer evidence is recorded; container/ELF runtime evidence remains an explicit unresolved limitation.
- [ ] 1.3 Resolve all `unknown` candidates before implementation by tracing dynamic loading and operational use, including `sqlite3`, `vpl-inspect`, `vainfo`, SeConv native libraries, FFmpeg/Intel ELF closures, metric-provider packaging, and the Dockerfile continuation-whitespace risk; verify no unknown is marked removable without evidence.
- [x] 1.4 Record the two deployment contexts and their validation implications: local development Compose versus production NAS Compose `/dev/dri`, healthchecks, service names, and backup assumptions; verify the design does not treat either Compose file as a capability authority.

## 2. Safe build and image optimization

- [ ] 2.1 Preserve the existing multi-stage boundary and prove that Go, CGo, Intel, FFmpeg/SVT-AV1, SeConv compilers, headers, source trees, package managers, and archive tools do not enter the final runtime image; verify with final-image package/file inspection.
- [ ] 2.2 Apply only measured build-cache/context improvements that do not alter runtime contents, such as Go/npm cache mounts, independent cacheable build stages, or narrowly justified `.dockerignore` entries; verify clean and incremental builds use the intended inputs and the final capability matrix is unchanged.
- [ ] 2.3 Evaluate copied `/opt/ffmpeg`, `/opt/intel`, and `/usr/local/lib` contents for duplicate development artifacts; implement pruning only after ELF/plugin closure and runtime probe evidence identifies files as unnecessary; verify no missing loader, FFmpeg, Intel driver, VPL, or SeConv dependency.
- [ ] 2.4 Evaluate runtime Debian package cleanup one candidate at a time, retaining healthcheck, backup, SSH, OCR, subtitle, SQLite, timezone, and diagnostic consumers; verify package removal with focused operational and feature checks before accepting it.
- [ ] 2.5 Keep the existing base image unless a measured alternative passes the complete contract with no unacceptable NAS/runtime tradeoff; verify the decision is supported by captured before/after measurements rather than theoretical savings.

## 3. Runtime capability and media validation

- [ ] 3.1 Verify final-image `ffmpeg` and `ffprobe` discovery, version output, loader closure, required filters, codecs, subtitle/font support, audio paths, and representative software conversion smoke tests; verify binary existence is reported separately from functional capability success.
- [ ] 3.2 Validate QSV/VAAPI in a production-style `/dev/dri`-enabled disposable runtime, including libva, oneVPL, Intel media-driver, driver plugin, permissions, and existing encoder smoke probes; verify CPU-only execution reports hardware unavailability through the existing model.
- [ ] 3.3 Validate the shipped `mvforge-hw-probe` diagnostics and any `vainfo`/`vpl-inspect` paths that are proven operational; verify missing optional diagnostics degrade explicitly and do not create a second capability authority.
- [ ] 3.4 Exercise the existing encoder capability cache and reset path after image changes; verify `CheckEncoder`/`ResetEncoderCache` behavior and capability payloads remain unchanged.
- [ ] 3.5 Exercise existing #69 MetricCapabilities probing for FFmpeg version, SSIM/PSNR/XPSNR, libvmaf/model identity, MS-SSIM provider evidence, unavailable reasons, cached results, and reset behavior; verify no Docker-specific flags or package-presence assumptions are introduced.

## 4. OCR, subtitle, and operational regression validation

- [ ] 4.1 Validate Tesseract execution, `--list-langs`, each supported configured language pack (`eng`, `spa`, `jpn`, and `jpn_vert` where applicable), SeConv launch/version, and Skia/HarfBuzz native loading; verify bitmap OCR succeeds for valid fixtures and reports the existing explicit failure when required tooling is absent.
- [ ] 4.2 Validate optional Hunspell dictionaries and the existing no-dictionary degradation path; verify OCR scoring/correction behavior does not become a hidden image assumption.
- [ ] 4.3 Validate ASS, SSA, SRT, bitmap subtitle extraction/conversion, canonical `both` reuse, sidecar validation/publication, and configured text/bitmap translation readiness using disposable fixtures; verify no OCR/subtitle/translation resolver or architecture is duplicated.
- [ ] 4.4 Run the final backend curl healthcheck and production frontend wget healthcheck; verify liveness remains independent from FFmpeg, OCR, hardware, and metric capability status.
- [ ] 4.5 Run the NAS backup command against a disposable database and verify SSH remote-execution readiness when configured; verify shell utilities, `sqlite3`, `openssh-client`, and runtime libraries remain available without touching production data.
- [ ] 4.6 Exercise startup, explicit runtime refresh, RuntimeState upsert, immutable RuntimeSnapshot audit capture, ExecutionPlan/Test Encode snapshot freeze, retention references, and both encoder/metric cache resets after the image change; verify #149 semantics and historical snapshot validity are unchanged.

## 5. Measurements and handoff

- [ ] 5.1 Build the optimized image using the exact baseline target, platform, arguments, and cache/pull policy; capture final size, layer history/IDs, package footprint, copied-prefix sizes, and capability/operational results with the same commands; verify an AFTER record exists or records each unavailable check and its concrete cause.
- [ ] 5.2 Compute and report measured before size, after size, absolute delta, percentage delta, and relevant layer/package changes; verify no estimate is presented as a saving and build-time cache improvements are reported separately from final-image size.
- [ ] 5.3 Review the complete diff against `origin/main` for capability redesign, removed transitive libraries, duplicated probes, hardcoded assumptions, unrelated refactors, and undocumented behavior changes; verify only #150 image/build/runtime validation surfaces are changed.
- [ ] 5.4 Run focused Docker/runtime/backend checks and record their results; then run `openspec validate runtime-image-optimization --type change --strict` and verify strict validation passes.
- [ ] 5.5 Run `./scripts/verify.sh --auto` exactly once after implementation and verify the output is `ALL CHECKS PASSED`; update only proven-complete #150 task checkboxes and prepare the scoped commit without opening or merging a PR.
