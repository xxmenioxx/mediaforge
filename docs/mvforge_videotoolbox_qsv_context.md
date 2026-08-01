# MVForge vNext – VideoToolbox and Quick Sync Quality Presets

## Scope

Implement encoder-specific quality presets only for:

- Apple VideoToolbox
- Intel Quick Sync Video (QSV)

User-facing presets:

- Compact
- Medium
- Recommended
- Best
- High Quality
- Custom

The same label represents the same user intention, but each encoder must translate it differently.

---

## 1. Shared analysis requirements

Before calculating a preset, inspect:

- Duration
- Source video bitrate
- Total container bitrate
- Resolution
- Frame rate
- Bit depth
- Pixel format
- HDR/SDR
- Grain/noise
- Motion complexity
- Content type
- Audio streams and bitrates
- Subtitle streams
- Container overhead

Do not calculate from total container bitrate when the video stream bitrate is available.

Use:

```text
Source video bitrate =
total bitrate
- audio bitrate
- subtitle bitrate
- estimated container overhead
```

---

# 2. Apple VideoToolbox

Supported encoders:

```text
h264_videotoolbox
hevc_videotoolbox
```

Primary target:

```text
hevc_videotoolbox
```

## Rate-control model

VideoToolbox presets should be bitrate-based.

```text
Target bitrate =
source video bitrate
× preset multiplier
× content adjustment
```

## Base multipliers

```text
Compact       0.40
Medium        0.52
Recommended   0.65
Best          0.80
High Quality  0.95
```

## Content adjustments

```text
Heavy grain           +15%
Moderate grain        +10%
Anime/detail-rich     +10%
Concert               +5%
Sports/high motion    +10%
DVD/noisy source      +5% to +10%
Clean digital source  -5%
Low-motion content    -10%
Simple animation      -10%
```

Clamp total adjustment to:

```text
Minimum: -15%
Maximum: +20%
```

## Suggested HEVC bitrate floors

### 480p

```text
Compact       1.5 Mbps
Medium        2.0 Mbps
Recommended   2.5 Mbps
Best          3.2 Mbps
High Quality  4.0 Mbps
```

### 720p

```text
Compact       2.2 Mbps
Medium        3.0 Mbps
Recommended   4.0 Mbps
Best          5.0 Mbps
High Quality  6.5 Mbps
```

### 1080p

```text
Compact       3.0 Mbps
Medium        4.0 Mbps
Recommended   5.0 Mbps
Best          6.0 Mbps
High Quality  7.0 Mbps
```

The adaptive source-based calculation may produce higher values.

## Dynamic constraints

```text
maxrate = target bitrate × 1.5
bufsize = target bitrate × 2.5
```

Example:

```text
Target: 5 Mbps
Maxrate: 7.5 Mbps
Buffer: 12.5 Mbps
```

## Recommended defaults

Offline conversion:

```text
-realtime 0
-power_efficient 1
-g 120
```

Use B-frames only when confirmed by the active encoder probe:

```text
-bf 3
```

### HEVC 8-bit

```text
-profile:v main
-pix_fmt yuv420p
```

### HEVC 10-bit

```text
-profile:v main10
-pix_fmt p010le
```

Never generate incompatible pairs such as:

```text
main10 + yuv420p
main + p010le
```

unless a real probe confirms support.

## Main UI controls

```text
Quality preset
Target bitrate
Estimated output size
Profile
Bit depth
Realtime
Power efficient
GOP
Color policy
```

## Advanced controls

```text
Maxrate
Buffer
B-frames
Frame reordering
Pixel format
Codec tag
Custom bitrate
```

## Example: Recommended

```bash
-c:v hevc_videotoolbox -b:v 5M -maxrate 7500k -bufsize 12500k -g 120 -realtime 0 -power_efficient 1 -profile:v main10 -pix_fmt p010le
```

---

# 3. Intel Quick Sync

Supported encoders:

```text
h264_qsv
hevc_qsv
av1_qsv
```

Primary target:

```text
hevc_qsv
```

## Preferred rate-control order

```text
1. LA-ICQ, when Look Ahead is confirmed functional
2. ICQ
3. CQP
4. VBR
5. CBR
```

The active worker probe is the source of truth.

## Default ICQ mappings

```text
Compact       ICQ 30
Medium        ICQ 27
Recommended   ICQ 25
Best          ICQ 22
High Quality  ICQ 20
```

Lower values mean higher quality.

## Complexity adjustments

For difficult content, reduce ICQ:

```text
Heavy grain          -2
Moderate grain       -1
Anime/detail-rich    -1
Concert              -1
Sports/high motion   -1
DVD/noisy source     -1
```

For simple content, increase ICQ:

```text
Low motion           +1
Clean digital video  +1
Simple animation     +1
```

Suggested limits:

```text
Minimum ICQ: 18
Maximum ICQ: 32
```

## LA-ICQ

When supported:

```text
-look_ahead 1
-look_ahead_depth 40
```

Suggested UI values:

```text
20  Fast
40  Recommended
60  High analysis
```

Default:

```text
40
```

When unsupported, fall back to ICQ and show:

```text
LA-ICQ requested, but Look Ahead is not supported by this worker.
Using ICQ instead.
```

## Extended BRC

Use only after a real encode probe confirms support:

```text
-extbrc 1
```

Recommended default:

```text
ICQ: ExtBRC off
LA-ICQ: optional when supported
```

## Adaptive I / B

Optional:

```text
-adaptive_i 1
-adaptive_b 1
```

Enable only when the full combination is confirmed by the runtime.

## Low power

Quality-focused presets should default to:

```text
-low_power 0
```

Low-power mode may reject:

- Look Ahead
- Extended BRC
- Certain profiles
- Certain rate-control modes
- B-frames

Use it only for explicit low-power or realtime profiles.

## Suggested QSV presets

```text
Compact       fast
Medium        medium
Recommended   medium
Best          slow
High Quality  slow
```

Validate accepted values with:

```bash
ffmpeg -hide_banner -h encoder=hevc_qsv
```

## QSV 8-bit mapping

```text
Pixel format: nv12
Profile: main
Bit depth: 8
```

Example:

```bash
-c:v hevc_qsv -global_quality 25 -profile:v main -pix_fmt nv12 -preset medium -low_power 0
```

## QSV 10-bit mapping

```text
Pixel format: p010le
Profile: main10
Bit depth: 10
```

Example:

```bash
-c:v hevc_qsv -global_quality 25 -profile:v main10 -pix_fmt p010le -preset medium -low_power 0
```

Never generate:

```text
NV12 + Main10
P010 + Main
```

## Example: Recommended ICQ

```bash
-c:v hevc_qsv -global_quality 25 -profile:v main10 -pix_fmt p010le -preset medium -low_power 0
```

## Example: Recommended LA-ICQ

```bash
-c:v hevc_qsv -global_quality 25 -profile:v main10 -pix_fmt p010le -preset medium -low_power 0 -look_ahead 1 -look_ahead_depth 40 -extbrc 1 -adaptive_i 1 -adaptive_b 1
```

Emit the full combination only after the active worker probe confirms it.

## Main UI controls

```text
Quality preset
Rate-control mode
ICQ quality
Preset
Profile
Bit depth
Look Ahead
Low power
Estimated output size
Color policy
```

## Advanced controls

```text
Look Ahead depth
Extended BRC
Adaptive I
Adaptive B
MBBRC
Async depth
GOP
B-frames
QP I/P/B
Target bitrate
Maxrate
Buffer
```

---

# 4. Output-size estimation

## VideoToolbox

```text
Estimated video bytes =
target video bitrate × duration / 8
```

This can produce a relatively precise estimate.

## QSV ICQ / LA-ICQ

ICQ does not guarantee an exact bitrate.

Until enough historical data exists, use approximate ranges relative to source video size:

```text
Compact       40% to 55%
Medium        50% to 65%
Recommended   60% to 75%
Best          70% to 85%
High Quality  80% to 100%
```

Display a range:

```text
Estimated output: 4.8–5.6 GB
```

Do not present a single exact size for ICQ unless based on measured preview samples.

## Preview-based estimate

Optionally encode:

```text
5 distributed samples × 20 seconds
```

Then calculate:

```text
measured sample bitrate × full duration
```

Show:

```text
Estimate source: distributed preview samples
Confidence: high
```

---

# 5. Audio-aware estimation

Always include mapped audio.

For copied audio:

```text
audio size =
original audio bitrate × duration / 8
```

For transcoded audio:

```text
audio size =
target audio bitrate × duration / 8
```

Display:

```text
Estimated video size
Estimated audio size
Subtitle/attachment size
Container overhead
Estimated output size
Estimated savings
```

Warn when audio dominates:

```text
Preserved audio tracks represent 32% of the estimated output size.
```

---

# 6. Color and fidelity policy

Both encoders should use the same color-policy model:

```text
Preserve source metadata
Normalize legacy SD to BT.709
Preserve HDR metadata
Convert HDR to SDR
Custom
```

Store separately:

```text
Source characteristics
Canonical preview characteristics
Encoder input characteristics
Output characteristics
```

Validate after conversion:

```text
Color range
Color matrix
Color primaries
Transfer
Pixel format
Bit depth
Chroma location
SAR
DAR
Field order
```

---

# 7. Capability probes

At worker startup:

```bash
ffmpeg -hide_banner -h encoder=hevc_videotoolbox
ffmpeg -hide_banner -h encoder=hevc_qsv
```

Advertised options are not enough. Run real smoke tests.

## VideoToolbox probes

```text
HEVC Main + yuv420p
HEVC Main10 + p010le
B-frames
Power efficient
Realtime off
```

## QSV probes

```text
HEVC Main + NV12 + ICQ
HEVC Main10 + P010 + ICQ
LA-ICQ
Look Ahead depth
ExtBRC
Adaptive I
Adaptive B
Low power
```

Store evidence, for example:

```json
{
  "encoder": "hevc_qsv",
  "available": true,
  "testedModes": {
    "icqMain8": true,
    "icqMain10": true,
    "laIcqMain10": false,
    "extbrc": false,
    "adaptiveI": true,
    "adaptiveB": true,
    "lowPowerMain10": false
  }
}
```

The UI must hide or disable unsupported combinations.

---

# 8. Suggested data model

```go
type QualityPreset string

const (
    QualityCompact     QualityPreset = "compact"
    QualityMedium      QualityPreset = "medium"
    QualityRecommended QualityPreset = "recommended"
    QualityBest        QualityPreset = "best"
    QualityHigh        QualityPreset = "high_quality"
    QualityCustom      QualityPreset = "custom"
)

type VideoToolboxPreset struct {
    Quality       QualityPreset `json:"quality"`
    TargetBitrate int64         `json:"targetBitrate"`
    Maxrate       int64         `json:"maxrate"`
    BufferSize    int64         `json:"bufferSize"`
    GOP           int           `json:"gop"`
    BFrames       int           `json:"bFrames"`
    Realtime      bool          `json:"realtime"`
    PowerEfficient bool         `json:"powerEfficient"`
    Profile       string        `json:"profile"`
    PixelFormat   string        `json:"pixelFormat"`
    ColorPolicy   string        `json:"colorPolicy"`
}

type QSVPreset struct {
    Quality        QualityPreset `json:"quality"`
    RateControl    string        `json:"rateControl"`
    GlobalQuality  int           `json:"globalQuality"`
    Preset         string        `json:"preset"`
    LookAhead      bool          `json:"lookAhead"`
    LookAheadDepth int           `json:"lookAheadDepth"`
    ExtendedBRC    bool          `json:"extendedBrc"`
    AdaptiveI      bool          `json:"adaptiveI"`
    AdaptiveB      bool          `json:"adaptiveB"`
    LowPower       bool          `json:"lowPower"`
    Profile        string        `json:"profile"`
    PixelFormat    string        `json:"pixelFormat"`
    ColorPolicy    string        `json:"colorPolicy"`
}
```

---

# 9. Implementation rules

1. Support only VideoToolbox and QSV in this phase.
2. VideoToolbox presets must be adaptive bitrate presets.
3. QSV presets should use ICQ or LA-ICQ when supported.
4. Do not treat VideoToolbox bitrate and QSV ICQ as equivalent.
5. Calculate from source video bitrate, not total container bitrate.
6. Include every preserved audio track in output-size estimates.
7. Use distributed previews to improve estimates.
8. Display QSV ICQ size estimates as ranges.
9. Never generate incompatible profile/pixel-format combinations.
10. Do not enable QSV low-power mode by default for quality presets.
11. Do not enable Look Ahead, ExtBRC or Adaptive I/B without a real probe.
12. Preserve or deliberately transform color through an explicit policy.
13. Store generated commands, probe evidence and estimates.
14. Validate advanced overrides before creating a job.
15. The active worker’s FFmpeg build, runtime, driver and hardware are the source of truth.
