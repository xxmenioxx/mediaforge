Implement a new encoder-independent quality recommendation engine for MVForge.

## Objective

The current VideoToolbox implementation already calculates adaptive bitrate using the analyzed asset.

Extend the architecture so that VideoToolbox and Quick Sync both use the same asset analysis, but each encoder translates that analysis differently.

Do NOT implement NVENC, x265 or AV1 yet.

This task only affects:

- VideoToolbox
- Intel Quick Sync (QSV)

---

# Design Goal

The user chooses:

- Compact
- Medium
- Recommended
- Best
- High Quality
- Custom

Those are NOT encoder presets.

They are quality intentions.

The backend converts that intention into encoder-specific settings.

Architecture:

Asset Analysis
        ↓
Quality Intent Engine
        ↓
Encoder Translator
        ↓
FFmpeg Command Builder

---

# Step 1

Reuse the existing asset analysis.

The recommendation engine should consume:

- source video bitrate
- duration
- output resolution (after crop/scale)
- HDR/SDR
- grain score
- motion score
- content type
- audio bitrate
- subtitle size
- user preset

Do not duplicate this analysis.

---

# Step 2

Create a new encoder-independent model.

Example:

```go
type QualityIntent struct {
    Preset             QualityPreset

    ResolutionClass    ResolutionClass

    SourceVideoBitrate int64

    GrainScore         float64

    MotionScore        float64

    ComplexityScore    float64

    ContentType        ContentType

    AudioBitrate       int64

    Duration           time.Duration
}
```

This object becomes the input for every encoder translator.

---

# Step 3

Implement VideoToolbox translator

Reuse the adaptive bitrate model already implemented.

Output should contain:

TargetBitrate

Maxrate

Buffer

Realtime

Profile

PixelFormat

ColorPolicy

No architectural changes beyond moving logic into the translator.

---

# Step 4

Implement Quick Sync translator

DO NOT calculate bitrate.

Instead calculate target quality.

Preferred order:

1. LA-ICQ
2. ICQ
3. CQP
4. VBR
5. CBR

The translator must receive worker capabilities.

If LA-ICQ is unavailable:

fallback to ICQ.

If ICQ unavailable:

fallback to CQP.

Store the effective configuration.

---

# Initial ICQ mapping

Base presets

Compact

30

Medium

27

Recommended

25

Best

22

High Quality

20

These are starting defaults.

Now adjust according to the asset.

---

# Complexity adjustments

Heavy grain

ICQ -2

Moderate grain

ICQ -1

Anime

ICQ -1

Sports

ICQ -1

Concert

ICQ -1

DVD

ICQ -1

Low motion

ICQ +1

Clean digital

ICQ +1

Simple animation

ICQ +1

Clamp between

18

and

32

---

Example

Recommended

↓

25

↓

Anime

24

↓

Moderate grain

23

↓

High motion

22

↓

Final ICQ

22

---

# Step 5

Create translator output.

Example

```go
type EncoderRecommendation struct {

    Encoder string

    RateControl string

    TargetBitrate *int64

    GlobalQuality *int

    Maxrate *int64

    Buffer *int64

    Profile string

    PixelFormat string

    LookAhead bool

    LookAheadDepth int

    LowPower bool

    ExtendedBRC bool

    AdaptiveI bool

    AdaptiveB bool

    EstimatedVideoBitrate *int64

    EstimatedOutputSize *int64

    Warnings []string
}
```

---

# Step 6

Estimated size

VideoToolbox

Use bitrate calculation.

Quick Sync

DO NOT estimate from ICQ directly.

Instead

If preview samples exist

↓

estimate from measured bitrate.

Else

↓

estimate using historical ratios.

Return a confidence level.

High

Measured samples.

Medium

Historical encoder data.

Low

Preset only.

---

# Step 7

Worker capability awareness

Quick Sync translator must use the active worker probe.

Do not emit:

Look Ahead

ExtBRC

Adaptive I

Adaptive B

unless confirmed.

Store

requested configuration

effective configuration

fallback reason

---

# Step 8

UI

Frontend should NOT know encoder logic.

It only displays

Recommended

↓

Backend recommendation

↓

Generated FFmpeg command

Also display

Estimated output

Estimated savings

Effective encoder settings

Confidence

Warnings

---

# Acceptance criteria

VideoToolbox

Uses adaptive bitrate.

Quick Sync

Uses adaptive ICQ.

Both start from the same QualityIntent.

Switching encoder changes only the translator.

No duplicated recommendation logic.

No duplicated preset tables.

Worker capabilities determine final FFmpeg parameters.

Show effective configuration when fallback occurs.

---

Before modifying code

1. Explain the current recommendation flow.
2. Identify duplicated preset logic.
3. Identify where encoder-specific decisions are currently made.

After implementation

1. Show new architecture.
2. Show new translator interfaces.
3. Show generated recommendation for:

Akira

720x460

Anime

Medium grain

Recommended

VideoToolbox

↓

Expected bitrate

Quick Sync

↓

Expected ICQ

4. List modified files.

Do not refactor unrelated components.