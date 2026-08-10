# MVForge — Codex Context Summary
## Updated: 2026-08-08

# Project identity

The project previously known as **MediaForge** is now **MVForge — Media Vault Forge**.

MVForge is a local/self-hosted media processing platform for analyzing, previewing, converting, restoring, validating, publishing, and auditing media assets. It complements Radarr/Sonarr/Jellyfin/Plex rather than replacing them.

Core stack:
- Backend: Go + Gin + GORM + SQLite
- Frontend: React + TypeScript + MUI + Vite
- Media: FFmpeg / FFprobe
- Subtitle OCR: Subtitle Edit CLI / SeConv + Tesseract
- Hardware acceleration: Intel Quick Sync (QSV) and Apple VideoToolbox
- Runtime: Docker Compose on NAS plus local macOS development/testing

Current high-level pipeline:

```text
Assets
  ↓
Discovery / Analysis
  ↓
Profiles / Asset Overrides
  ↓
Queue
  ↓
Worker execution
  ↓
Validation
  ↓
Publisher
  ↓
History / Reports / Logs
```

Long-running UI-triggered work is beginning to use a separate concept:

```text
Background Operations
```

Do not merge Background Operations and Pipeline Jobs into the same abstraction yet.

---

# Hosting / hardware direction

Recommended architecture:

```text
NAS
├─ MVForge controller/backend
├─ frontend
├─ database
├─ scheduler
├─ background operations
├─ primary media storage
└─ local QSV worker

MacBook
└─ optional remote executor / future worker
   ├─ VideoToolbox
   ├─ FFmpeg / FFprobe
   └─ x265 / special workloads
```

The NAS should remain the stable control plane and primary storage location. The MacBook should provide optional compute capacity, not become infrastructure critical to MVForge.

## NAS context

UGREEN DXP4800 Plus with Intel Pentium Gold 8505 / Alder Lake-P iGPU, 24 GB RAM, NVMe for Docker, and QSV available through `/dev/dri/renderD128`.

## MacBook context

VideoToolbox support confirmed:

```text
h264_videotoolbox
hevc_videotoolbox
prores_videotoolbox
```

A 60-second HEVC VideoToolbox test reading from NAS storage completed successfully at about:

```text
5.76x
```

The Mac can read and write directly against NAS-backed MVForge storage.

Current macOS mount used in testing:

```text
/Volumes/docker/nas-media-stack/work/mediaforge
```

NAS → SSH → MacBook has also been manually validated. Remote execution of `ffmpeg`, `ffprobe`, and VideoToolbox capability commands works.

---

# QSV capability work

MVForge is moving to a runtime capability-driven encoder model. Do not assume a feature is supported merely because FFmpeg exposes a command-line option.

Confirmed on the current NAS runtime:

Working:
- ICQ Main
- ICQ Main10
- CQP Main
- CQP Main10
- VBR Main
- VBR Main10
- CBR Main
- CBR Main10
- VBR + Extended BRC Main10
- CBR + Extended BRC Main10
- Adaptive I Main10
- Adaptive B Main10

Not effectively available on the current runtime:
- LA-ICQ Main10 (requested LA_ICQ falls back/reports ICQ)
- VBR + LookAhead Main10
- CBR + LookAhead Main10
- full VBR advanced combination
- QSV Low Power

Important rule:

```text
FFmpeg option exists ≠ runtime supports it
```

Capabilities must be probed per runtime / worker.

A shared frontend helper was introduced:

```text
frontend/src/utils/qsvCapabilities.ts
```

Conceptual usage:

```ts
resolveQSVFeatures(capability, {
  main10,
  rateControl,
})
```

It independently determines Adaptive I, Adaptive B, Extended BRC, and LookAhead availability instead of depending on legacy `qsvFullCombination` behavior.

This helper is being used in AssetsPage, ProfilesPage, and ProfileLabPage.

---

# VideoToolbox direction

VideoToolbox is handled separately from QSV.

Desired mappings:

```text
Main   → yuv420p
Main10 → p010le
```

The UI should expose Main / Main10 only when runtime capability supports them.

A separate backend test exposed a VideoToolbox mapping issue where a Main10 request could still produce:

```text
-profile:v main
-pix_fmt yuv420p
```

That issue remains separate from OCR/background operation work.

---

# Frame Fidelity direction

Current validation sometimes compares source directly with output without deriving the expected output from the requested/effective profile.

Desired model:

```text
Source
→ Requested profile
→ Expected output
→ Effective config
→ Actual output
→ Verdict
```

Example:

```text
source:    8-bit yuv420p
requested: VideoToolbox Main10 / p010le
actual:    8-bit yuv420p
```

This should be considered unexpected output, not simply preserved output.

Pixel format and bit depth are good first fields to migrate to this model.

---

# Blu-ray PGS OCR bug and fix

Real test asset: Django Unchained Blu-ray MKV.

FFprobe showed multiple PGS subtitle streams. Stream 10 was confirmed Spanish.

Original behavior:
- MVForge generated a file named as Spanish stream 10
- actual OCR content was Portuguese

Root cause:
- SeConv `--track-number` against the MKV did not reliably correspond to FFmpeg absolute stream index

Fix for PGS codecs (`hdmv_pgs_subtitle`, `pgssub`):

1. Extract the exact subtitle stream using FFmpeg absolute mapping:

```text
-map 0:<stream index>
-c:s copy
```

2. Write a temporary `.sup` file.
3. Run SeConv directly against the `.sup`.
4. Do not pass `--track-number` for extracted PGS.

DVD/VobSub keeps the previous Matroska track-number flow.

Manual validation confirmed:

```text
stream 10
→ extracted SUP
→ SeConv/Tesseract
→ correct Spanish SRT
```

This proved FFmpeg stream mapping and OCR were correct; the previous bug was track selection.

---

# OCR async/background operation work

Bitmap subtitle OCR launched from Assets Override is now asynchronous.

Backend operation structure currently resembles:

```go
type subtitleExtractionOperation struct {
    ID          string
    Status      string
    Phase       string
    Progress    float64
    Processed   int
    Total       int
    StreamIndex int
    Format      string
    AssetPath   string
    Message     string
    Result      *SubtitleExtractionResult
    Error       string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

Operations are stored in memory using a map protected with `sync.RWMutex`.

API:

```text
POST /api/assets/extract-subtitles
GET  /api/assets/extract-subtitles
GET  /api/assets/extract-subtitles/:id
```

Bitmap OCR POST now returns:

```text
HTTP 202 Accepted
```

with an `operationId`.

Example:

```json
{
  "operationId": "ocr-10-...",
  "status": "running",
  "phase": "preparing",
  "progress": 1,
  "streamIndex": 10,
  "format": "srt"
}
```

Status can be polled via:

```text
GET /api/assets/extract-subtitles/:id
```

---

# OCR process lifetime

Critical implementation detail:

Do not use request context for background OCR:

```go
c.Request.Context()
```

because it is cancelled when the HTTP request completes.

Current OCR uses:

```go
context.WithTimeout(
    context.Background(),
    2*time.Hour,
)
```

This allows OCR to continue after closing the Snapshot dialog or navigating away from Assets.

---

# OCR progress discovery

SeConv live output includes a line such as:

```text
Running tesseract OCR on 1945 Blu-Ray sup image(s)...
```

MVForge successfully parses:

```text
total = 1945
```

However SeConv does not currently emit useful per-image progress through the stdout/stderr stream being consumed by MVForge.

Therefore:

```text
processed = 0
```

for the OCR phase.

Do not fake progress.

During OCR the frontend should show truthful indeterminate progress:

```text
Running OCR on 1945 images…
[animated indeterminate bar]
```

instead of freezing at 15%.

---

# OCR state survives navigation

A major UX improvement has been implemented and manually verified.

Each OCR operation now stores:

```text
AssetPath
```

The collection endpoint supports:

```text
GET /api/assets/extract-subtitles?path=<asset path>
```

AssetsPage behavior:

```text
Start OCR
→ receive operationId
→ poll operation

Close Snapshot
→ OCR continues

Navigate elsewhere
→ OCR continues

Return to Assets
→ open same asset
→ query operations by AssetPath
→ restore local subtitle generation state
→ resume polling
```

This flow was manually tested and confirmed working.

Frontend uses a `useRef<Set<string>>` to avoid duplicate pollers for the same `operationId`.

The same `generateExternalSubtitle()` flow supports:
- synchronous text subtitle extraction
- asynchronous bitmap OCR

---

# Background Operations — next phase

Current OCR operations should become the first implementation of a reusable global Background Operations architecture.

Potential future operation types:
- subtitle OCR
- preview generation
- frame generation
- fidelity analysis
- long validations
- cleanup/publisher actions
- maintenance tasks

Important distinction:

```text
BackgroundOperation
= UI-visible lifecycle of long-running action

Pipeline Job
= scheduler/conversion workflow lifecycle
```

Do not rewrite the pipeline solely to support a global Operations indicator.

Desired global UX:

```text
Operations (1)
```

with entries such as:

```text
Django Unchained
Subtitle OCR · Spanish · Stream 10
Running OCR on 1945 images…
```

Suggested future generic endpoints:

```text
GET /api/operations
GET /api/operations/:id
```

Existing subtitle operation endpoints should remain compatible during migration.

---

# OCR and pipeline interaction

Manual Assets OCR currently follows roughly:

```text
Assets Override
→ async operation
→ generateBitmapSubtitleSidecar(...)
→ runBitmapOCR(...)
```

Pipeline subtitle conversion can continue its existing synchronous path, conceptually:

```text
Pipeline
→ generateBitmapSubtitleAtPath(...)
→ runBitmapOCR(...)
```

The background-operation UX does not require converting the pipeline subtitle stage to async.

---

# Duplicate OCR risk

Now that manual OCR can run independently, this case is possible:

```text
Manual OCR
+
Pipeline OCR for the same subtitle
```

Potential problems:
- duplicated Tesseract CPU work
- unnecessary disk I/O
- duplicate output generation
- confusing UI state

Recommended operation identity should eventually include:

```text
operation type
+ asset path
+ stream index
+ output format
+ OCR language
+ OCR mode
```

Example:

```text
subtitle_ocr:<assetPath>:10:srt:spa:accurate
```

If an equivalent OCR is already running, a new request should reuse the existing operation rather than launch another SeConv process.

---

# Operation persistence limitation

Current operation registry is in memory only.

Therefore:

```text
closing modal        survives
navigation           survives
frontend refresh     survives
browser restart      survives while backend stays alive
backend restart      operation state is lost
```

This is acceptable for the current phase.

Future persistence may use SQLite, but persisted metadata does not imply an external FFmpeg/Tesseract process can actually resume after backend restart.

---

# MacBook RemoteExecutor foundation — validated

The current plan is to keep MVForge on the NAS and use the MacBook as optional compute.

Validated manually:

```text
NAS → SSH → MacBook
```

works.

From the NAS, remote commands can execute on the Mac.

The Mac has:
- FFmpeg
- FFprobe
- VideoToolbox
- access to mounted NAS storage

Therefore the following architecture is viable:

```text
MVForge backend on NAS
        │
        │ SSH
        ▼
MacBook
├─ FFmpeg
├─ FFprobe
├─ VideoToolbox
└─ mounted NAS storage
```

---

# Planned RemoteExecutor phase

Before implementing the full worker protocol, create a minimal `RemoteExecutor` abstraction.

Initial goals:

1. configure the Mac executor
2. validate SSH connectivity
3. probe FFmpeg remotely
4. probe FFprobe remotely
5. probe VideoToolbox remotely
6. validate storage path mapping
7. execute controlled FFmpeg commands through SSH
8. stream FFmpeg progress back to the NAS-hosted MVForge job

Do not rewrite the scheduler first.

Suggested temporary model:

```go
type RemoteExecutor struct {
    Name        string
    Host        string
    User        string
    StorageRoot string
}
```

Possible configuration:

```yaml
remoteExecutors:
  - name: macbook
    host: 192.168.x.x
    user: anuelvs
    storageRoot: /Volumes/docker/nas-media-stack/work/mediaforge
```

The NAS remains authoritative for:
- DB
- jobs
- scheduler
- job state
- history
- background operations

The Mac only performs media execution.

---

# Path mapping requirement

Absolute paths differ between the NAS and macOS.

Example:

```text
NAS:
/volume1/.../mediaforge/raw/movie.mkv

Mac:
/Volumes/docker/nas-media-stack/work/mediaforge/raw/movie.mkv
```

Long-term preferred model:

```text
storage = main-media
relativePath = raw/movie.mkv
```

with executor-specific mount mappings:

```text
NAS:
main-media → /volume1/.../mediaforge

Mac:
main-media → /Volumes/docker/nas-media-stack/work/mediaforge
```

Do not assume host-specific absolute paths are portable between workers.

---

# Remote FFmpeg progress

SSH does not need to hide FFmpeg progress.

Use FFmpeg machine-readable progress where appropriate:

```text
-progress pipe:1
```

MVForge can consume fields such as:

```text
frame=
fps=
out_time_ms=
speed=
progress=continue
```

The frontend should keep showing normal job progress regardless of where FFmpeg executes.

---

# Remote executor failure behavior

The MacBook is not always-on infrastructure.

Expected failure cases:
- sleep
- network loss
- SSH disconnect
- NAS mount unavailable
- missing FFmpeg

MVForge should detect executor failure and mark the job failed/errored appropriately rather than leave it stuck in `running`.

A future always-on Mac mini could become a more reliable permanent worker.

---

# Recommended immediate Codex priorities

## Priority 1 — stabilize current OCR changes

- async OCR continues working
- navigation rehydration continues working
- indeterminate OCR progress remains truthful
- duplicate polling remains prevented
- pipeline subtitle behavior remains unchanged

## Priority 2 — generic Background Operations read model

Introduce a global read-only operations view backed first by existing OCR operations.

Do not migrate all jobs.

## Priority 3 — OCR deduplication

Prevent equivalent OCR from running twice.

## Priority 4 — RemoteExecutor skeleton

Implement only:

```text
configuration
SSH health probe
FFmpeg probe
FFprobe probe
VideoToolbox probe
storage path mapping validation
```

Do not run production conversion jobs remotely yet.

## Priority 5 — controlled remote execution

Use the RemoteExecutor first for:
- explicit manual test
- preview
- short controlled conversion

Then integrate full job execution.

---

# Non-goals right now

Do NOT:
- move the MVForge controller to the MacBook
- create two independent MVForge databases
- rewrite the scheduler entirely
- merge BackgroundOperation and Pipeline Job into one model
- fake OCR progress
- replace SeConv only to obtain progress
- make pipeline OCR async without a concrete need
- implement fake process resume after backend restart
- build full worker leasing before validating the RemoteExecutor abstraction
- assume paths are identical across hosts
- expose QSV/VideoToolbox features without runtime capability probes

---

# Core design principles

1. NAS remains the stable control plane.
2. Storage remains centralized on NAS.
3. Executors/workers are compute capacity, not the source of truth.
4. Capabilities must be runtime-probed.
5. Requested / effective / actual media configuration should remain distinguishable.
6. Long-running UI actions should survive navigation.
7. Progress must be truthful.
8. Duplicate expensive work should be prevented.
9. Pipeline jobs and background operations remain separate concepts for now.
10. Host-specific absolute paths should evolve toward logical storage mappings.
11. Prefer small validated changes over broad refactors.
12. MacBook should initially be optional compute, not critical infrastructure.

---

# Current validated state

```text
✓ QSV runtime probing is more accurate
✓ contextual QSV UI feature gating exists
✓ PGS OCR selects the correct absolute subtitle stream
✓ Spanish Blu-ray OCR was manually validated
✓ Assets bitmap OCR is asynchronous
✓ OCR survives leaving Assets
✓ OCR state is restored when reopening the asset
✓ duplicate frontend pollers are prevented
✓ SeConv total image count is detected
✓ OCR UI uses honest indeterminate progress
✓ NAS → SSH → MacBook works
✓ MacBook VideoToolbox works
✓ MacBook reads/writes directly against NAS media storage
✓ remote FFmpeg / FFprobe execution is viable
```

Next major implementation milestones:

```text
Global Background Operations
+
RemoteExecutor skeleton for MacBook
```
