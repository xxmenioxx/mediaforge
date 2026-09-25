## Purpose

This capability defines the evidence, runtime contracts, and measurable safety
gates for optimizing MediaForge Docker images without turning image contents
into a second authority for runtime capabilities.

## ADDED Requirements

### Requirement: Dependency changes are evidence-gated

Every dependency considered for movement, pruning, or replacement MUST be
recorded in a dependency map containing the component, package/binary/library,
consumer or probe, build-time/runtime classification, required/optional/
unknown status, evidence, and proposed treatment. A dependency classified as
unknown MUST NOT be removed until its application, script, healthcheck,
diagnostic, dynamic-loader, and transitive-library use has been resolved.

#### Scenario: A package has no direct Go import

- **WHEN** a package is absent from Go source references but is used by a
  backup script, healthcheck, shell command, dynamic loader, or diagnostic
  binary
- **THEN** the package remains classified as a runtime or operational
  dependency and is not removed based on the missing Go import alone

#### Scenario: A copied library has no obvious source consumer

- **WHEN** a library is copied from a build stage but its ELF or plugin
  consumers have not been resolved
- **THEN** it remains unknown and the implementation records the required
  closure/probe evidence before proposing its removal

### Requirement: Image composition does not assert capability availability

Docker stages, installed packages, copied libraries, device mappings, and
environment variables MUST NOT be treated as proof that a capability is
usable. Existing runtime probes remain authoritative for encoder, hardware,
metric, OCR, and related capability availability. The #69 `MetricCapabilities`
contract and the #149 `RuntimeState` / `RuntimeSnapshot` separation MUST remain
unchanged.

#### Scenario: Installed acceleration libraries cannot initialize hardware

- **WHEN** the image contains libva, oneVPL, and an Intel media driver but the
  runtime probe cannot initialize the available device
- **THEN** the capability is reported through the existing unavailable or
  degraded result, and the image does not claim QSV or VAAPI availability

#### Scenario: An image optimization changes detected capabilities

- **WHEN** a later image exposes a different result from an existing runtime
  probe
- **THEN** refresh/cache-reset behavior and existing state/snapshot semantics
  remain unchanged, with the detected result flowing through `RuntimeState`
  and an immutable `RuntimeSnapshot` only at its existing audit boundaries

### Requirement: The media runtime contract is executable

The runtime image MUST provide usable `ffmpeg` and `ffprobe` binaries through
the existing discovery path, the FFmpeg filters and codecs used by MediaForge,
and the dynamically loaded libraries required by those binaries. QSV and VAAPI
support MUST retain the libva, libdrm, oneVPL, Intel media-driver, driver-file,
device-access, and loader requirements needed by existing probes and execution.
Metric providers MUST continue to be evaluated by the existing
`MetricCapabilities` probes, including provider/filter evidence and model
identity where applicable. A successful `ffmpeg` startup alone MUST NOT count
as proof of every media capability.

#### Scenario: CPU media processing remains usable

- **WHEN** a runtime container has no usable GPU device
- **THEN** `ffmpeg`, `ffprobe`, required software filters/codecs, and metric
  probes that are available on the image continue to work, while QSV/VAAPI
  capabilities degrade through the existing probe result

#### Scenario: Intel acceleration is tested on a device-enabled runtime

- **WHEN** the production-style container exposes `/dev/dri` and the required
  permissions
- **THEN** the existing encoder and hardware smoke probes exercise QSV/VAAPI
  initialization and report actual usable modes rather than package presence

#### Scenario: Metric provider is absent or unusable

- **WHEN** an FFmpeg metric filter, provider, or required model identity cannot
  be verified by the existing metric probe
- **THEN** the corresponding `MetricCapabilities` entry is unavailable with
  its existing reason/identity semantics, without adding image-specific
  capability flags or changing the probe contract

### Requirement: OCR and subtitle workflows retain their runtime contract

The runtime image MUST preserve the currently supported OCR and subtitle
behaviors: Tesseract execution, configured OCR language data, SeConv and its
native Skia/HarfBuzz libraries, FFmpeg subtitle extraction/conversion, native
ASS/SRT/SSA handling, bitmap subtitle workflows, and existing translation
tooling boundaries. Hunspell language dictionaries MUST remain available when
used for the existing optional OCR scoring/correction path. The change MUST
NOT introduce a parallel OCR, subtitle, or translation architecture.

#### Scenario: Bitmap OCR language is configured

- **WHEN** a bitmap subtitle workflow requests a supported OCR language
- **THEN** the existing readiness and execution path can find Tesseract,
  SeConv, and the required trained data, or reports the existing actionable
  unavailable/error result without silently producing a false success

#### Scenario: Hunspell is absent

- **WHEN** the optional Hunspell dictionary used by OCR scoring/correction is
  unavailable
- **THEN** OCR continues through the existing optional-degradation path, with
  no image assumption claiming dictionary-backed quality

#### Scenario: Text and bitmap subtitle conversions are exercised

- **WHEN** runtime validation processes representative ASS, SSA, SRT, and
  bitmap subtitle inputs, including the logical `both` output and translation
  paths where configured
- **THEN** the existing conversion, validation, sidecar, and artifact-reuse
  behavior remains intact

### Requirement: Operational binaries remain available to their consumers

The runtime image MUST retain the binaries and base utilities required by the
Docker healthchecks, backup/restore scripts, startup behavior, remote
execution, and diagnostics. This includes validating the backend curl
healthcheck, frontend wget healthcheck where the production target is used,
the SQLite CLI and shell utilities used by the NAS backup command, and SSH
used by configured remote execution. Health status MUST NOT be expanded to
claim media or hardware capability success.

#### Scenario: Backend healthcheck runs in the final image

- **WHEN** the backend container is running
- **THEN** its existing curl-based `/health` check succeeds using the final
  image and service port, independent of optional media capability state

#### Scenario: NAS backup runs from the deployed stack

- **WHEN** the backup script executes against the configured backend service
- **THEN** the final image still provides shell, date, mkdir, and `sqlite3`
  behavior needed to create a backup without modifying application data
  semantics

#### Scenario: Remote execution is configured

- **WHEN** a configured remote executor uses the existing SSH command path
- **THEN** the final image retains the SSH client and its required runtime
  libraries, or the feature is explicitly classified unavailable before use

### Requirement: Optional capabilities degrade through existing mechanisms

Missing optional packages, language packs, GPU devices, metric providers, or
diagnostic tools MUST be represented through the existing readiness,
capability, warning, or error mechanisms applicable to that component. The
implementation MUST NOT add Docker-image-specific flags or infer availability
from package installation alone. Mandatory base functionality MUST remain a
hard validation failure when its consumer is enabled.

#### Scenario: Optional GPU or metric tooling is missing

- **WHEN** a runtime image lacks an optional GPU device or metric provider
- **THEN** the existing runtime/capability model reports it unavailable and
  CPU or other available paths remain eligible where current policy permits

#### Scenario: Required OCR tooling is missing

- **WHEN** a bitmap OCR operation is requested but Tesseract or SeConv is
  unavailable
- **THEN** the existing operation fails or reports not-ready explicitly and
  does not emit an apparently valid OCR artifact

### Requirement: Optimization and measurement are reproducible

Image optimization MUST be limited to build-only dependency removal,
multi-stage/cache improvements, package or artifact cleanup, layer/context
improvements, or safe base-image changes whose behavior is demonstrated by
the dependency map and regression validation. Before and after measurements
MUST use the same source revision basis, target platform, build arguments,
cache policy, image inspection commands, and package/layer reporting method.
The report MUST include final image size, relevant layer changes, and where
practical package/dependency footprint, plus absolute and percentage deltas.
Savings MUST NOT be estimated; unavailable Docker measurements MUST be
reported as unavailable.

#### Scenario: Docker measurement is available

- **WHEN** the current origin/main image is built or obtained before the
  implementation and the optimized image is built with the same method
- **THEN** the report contains before, after, absolute delta, percentage
  delta, and relevant layer/package observations from captured measurements

#### Scenario: Docker measurement is unavailable

- **WHEN** Docker build, inspect, or runtime execution is blocked by the
  environment
- **THEN** the report states measurement or validation unavailable with the
  concrete reason and does not substitute an estimate

### Requirement: Runtime snapshot and rollback compatibility is preserved

Image optimization MUST NOT change RuntimeState authority, RuntimeSnapshot
immutability or audit semantics, stable fingerprint rules, ExecutionPlan or
Test Encode snapshot freeze, retention, runtime refresh, encoder-cache reset,
or MetricCapability-cache reset. A Docker/runtime rollback MUST be possible
independently of persisted application data and MUST require no database
migration solely for image optimization. Existing RuntimeSnapshots MUST remain
valid historical evidence even if a later image detects a different
capability set.

#### Scenario: The image is rolled back after a capability change

- **WHEN** a runtime dependency change is reverted after deployment
- **THEN** the previous image can be redeployed without destructive database
  operations, and existing runtime snapshot rows and references remain valid

#### Scenario: Runtime refresh follows an image change

- **WHEN** explicit runtime refresh and encoder/metric cache reset are invoked
  after deployment
- **THEN** the existing probes repopulate live state/cache results while
  immutable snapshot audit and retention behavior remain unchanged
