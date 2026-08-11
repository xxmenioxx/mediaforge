# MVForge — Context for Codex: Frame Structure Recommendation Engine

## Purpose

Add a recommendation system in MVForge that analyzes a source asset and suggests a sensible output frame structure for encoding.

The recommendation should cover:

```text
GOP size
maximum B-frames / B-run target
Adaptive I policy
Adaptive B policy
IDR / keyframe interval where applicable
```

The recommendation must NOT attempt to reproduce the exact source I/P/B counts.

The goal is:

> Derive a stable and encoder-appropriate GOP/frame-structure recommendation from the source characteristics and the target encoder, then validate the actual output against that recommendation.

This recommendation is advisory and should integrate with Profile Lab first.

---

# 1. Important distinction

Do NOT attempt to calculate:

```text
exact number of I frames
exact number of P frames
exact number of B frames
```

for the target encode.

Those are consequences of encoder decisions.

Instead calculate encoder constraints and targets such as:

```text
target GOP
maximum B-frame depth
adaptive I enabled/disabled
adaptive B enabled/disabled
keyframe interval
```

Then after encoding, inspect what the encoder actually produced.

The architecture should be:

```text
Source Analysis
      ↓
Frame Structure Recommendation
      ↓
Encoder Configuration
      ↓
Encode
      ↓
Actual Frame Analysis
      ↓
Requested vs Actual Validation
```

---

# 2. Source metrics available / desired

Use source analysis metrics where available:

```text
FPS
I-frame count
P-frame count
B-frame count
B-frame ratio
maximum consecutive B-frame run
average GOP
minimum GOP
maximum GOP
scene-change density if available
duration
resolution
interlaced/progressive state
```

Useful derived metrics:

```text
framesPerSecond
secondsPerSourceGOP
sourceBRun
sourceBRatio
IFrameDensity
```

Example:

```text
Source:
FPS: 29.97
I: 7
P: 203
B: 290
B ratio: 58%
B-run max: 3
Average GOP: 82.7
```

Derived:

```text
82.7 / 29.97
≈ 2.76 seconds per GOP
```

This can be used as one signal for recommendation.

---

# 3. GOP recommendation

The core calculation should be based on time rather than blindly copying source GOP frames.

Conceptually:

```text
targetGOPFrames =
round(FPS × targetKeyframeSeconds)
```

Examples:

```text
23.976 fps × 4 seconds ≈ 96
29.97 fps × 3 seconds ≈ 90
29.97 fps × 4 seconds ≈ 120
59.94 fps × 2 seconds ≈ 120
```

Prefer GOP recommendations expressed internally as both:

```text
seconds
frames
```

Example model:

```go
type FrameStructureRecommendation struct {
	TargetGOPFrames   int
	TargetGOPSeconds  float64
	MaxBFrames        int
	AdaptiveI         bool
	AdaptiveB         bool
	Reasoning         []string
	Warnings          []string
}
```

---

# 4. GOP recommendation should use multiple signals

Do NOT use only FPS.

Suggested inputs:

```text
FPS
source average GOP
content type
motion level
source frame structure
target encoder
target playback policy
worker capabilities
```

A first-pass policy can use:

```text
source GOP as baseline
+
encoder-safe boundaries
+
content-type adjustments
```

Example:

```text
source GOP ≈ 83 frames
29.97 fps
≈ 2.8 seconds
```

A sensible recommendation might round this into:

```text
90 frames
```

or:

```text
120 frames
```

depending on target policy.

Do not blindly use:

```text
-g 83
```

just because the source average is 82.7.

Prefer stable encoder-friendly values.

---

# 5. Suggested initial GOP ranges

These are policy starting points, not universal truths.

### Conservative / compatibility-oriented

```text
~2 seconds
```

Examples:

```text
23.976 → 48
25 → 50
29.97 → 60
30 → 60
59.94 → 120
```

### Balanced

```text
~3–4 seconds
```

Examples:

```text
23.976 → 72–96
25 → 75–100
29.97 → 90–120
30 → 90–120
```

### Compression-oriented

Potentially longer, but only if:

```text
playback target allows it
encoder handles it well
validation confirms sane output structure
```

Do NOT automatically use very long GOP values simply to reduce bitrate.

---

# 6. Source-relative GOP recommendation

Source GOP should influence the recommendation.

Example heuristic:

```text
sourceGOPSeconds =
sourceAverageGOP / FPS
```

If source GOP is already in a reasonable range:

```text
~2–5 seconds
```

prefer a recommendation near the source structure.

If source GOP is extremely short:

```text
< 1 second
```

do not necessarily preserve it.

If source GOP is extremely long:

```text
> 8–10 seconds
```

do not necessarily preserve it either.

Normalize toward MVForge's safe range.

Conceptually:

```text
targetSeconds =
clamp(sourceGOPSeconds, minimumSafe, maximumSafe)
```

Then round into an encoder-friendly frame count.

---

# 7. B-frame recommendation

Do NOT recommend a target percentage of B-frames.

Do NOT calculate:

```text
B ratio should be exactly 60%
```

Instead recommend:

```text
maximum B-frame depth
```

or equivalent encoder parameter.

For example:

```text
maxBFrames = 3
```

means the encoder may use B-frames but should not normally create pathological runs.

The actual encoder may produce fewer.

---

# 8. Source B-run as input

Source maximum B-run is useful.

Example:

```text
Source B-run max: 3
```

This suggests the original stream uses a conventional B-frame depth.

A reasonable recommendation can preserve approximately:

```text
2–4 B-frames
```

rather than allowing hundreds.

Do not blindly copy extreme source values.

Suggested heuristic:

```text
if sourceBRun <= 0:
    recommend 0–2 depending on compatibility policy

if sourceBRun between 1 and 4:
    preserve approximately sourceBRun

if sourceBRun > 4:
    cap to encoder/profile safe maximum
```

For QSV/HEVC, initial recommendation can generally remain conservative.

---

# 9. Important example from actual MVForge analysis

Observed source:

```text
I: 7
P: 203
B: 290
B ratio: 58.0%
B-run max: 3
Average GOP: 82.7
```

Observed QSV output:

```text
I: 2
P: 0
B: 477
B ratio: 99.6%
B-run max: 247
Average GOP: 248
```

This should NOT be considered a normal consequence of a recommendation such as:

```text
max B-frames = 3
```

If MVForge requested a bounded B-frame structure and actual output is:

```text
B-run = 247
```

that is a major requested-vs-effective deviation and should be flagged.

---

# 10. Adaptive I recommendation

Adaptive I should be treated separately from GOP.

`Adaptive I` allows the encoder to make adaptive I-frame decisions.

Suggested policy:

```text
enable only if:
- preset/policy allows advanced QSV features
- active worker capability probe confirms support
- content benefits from scene-adaptive decisions
```

Do NOT interpret:

```text
Adaptive I OFF
```

as:

```text
no I-frames
```

I-frames remain necessary for GOP/keyframe structure.

Adaptive I only changes how dynamically the encoder decides them.

---

# 11. Adaptive B recommendation

Adaptive B also must be treated separately from B-frame existence.

Suggested policy:

```text
enable only if:
- preset/policy allows it
- worker capability confirms it
- encoder/rate-control combination supports it
```

Do NOT interpret:

```text
Adaptive B OFF
```

as:

```text
B-frames disabled
```

Normal B-frame generation may continue.

Adaptive B controls adaptive B/P decisions, not whether B-frames exist at all.

---

# 12. QSV capability requirements

All QSV recommendations must respect active worker capability.

The recommendation engine may suggest:

```text
Adaptive I = desired
```

but effective configuration must still check:

```text
worker supports Adaptive I for this exact combination?
```

Likewise for:

```text
Adaptive B
Look Ahead
Extended BRC
LA-ICQ
```

The recommendation engine should never override capability validation.

Architecture:

```text
Asset recommendation
       ↓
QSV feature resolver
       ↓
Worker capability matrix
       ↓
Effective config
```

---

# 13. Extended BRC is unrelated to frame-structure calculation

Do NOT use Extended BRC as part of B-frame/GOP recommendation logic.

Extended BRC belongs to bitrate control.

MVForge policy remains:

```text
ICQ    → ExtBRC OFF
CQP    → ExtBRC OFF
LA-ICQ → ExtBRC OFF

VBR/CBR
→ ExtBRC may be used only when validated by active worker capability
```

Frame-structure recommendation must not say:

```text
bad B-frame pattern → enable ExtBRC
```

That is incorrect.

---

# 14. Look Ahead is also not a generic GOP fix

Look Ahead may improve encoder decisions when supported, but it must not be used as a workaround for abnormal frame structure.

On the current QSV worker, Look Ahead / LA-ICQ were not validated.

Therefore:

```text
frame recommendation
```

must work correctly without Look Ahead.

---

# 15. Content-type adjustments

MVForge may optionally adjust recommendations using asset classification.

Examples:

### Anime / animation

Potential characteristics:

```text
long static areas
limited movement between frames
scene cuts
repeated backgrounds
```

A reasonable starting recommendation:

```text
GOP: ~3–4 seconds
Max B-frames: 3–4
Adaptive I: useful if supported
Adaptive B: optional if validated
```

### Film

```text
GOP: ~2–4 seconds
Max B-frames: 3–4
Adaptive I: useful
Adaptive B: optional
```

### High-motion / sports

Prefer somewhat shorter GOP:

```text
~2 seconds
```

and conservative B-depth.

### Compatibility-first

Prefer:

```text
shorter GOP
lower B-depth
```

Do not over-optimize compression.

These should be policy hints, not rigid rules.

---

# 16. Motion analysis

If MVForge already has or later adds motion metrics, use them as an input.

Conceptually:

```text
low motion
→ longer GOP may be acceptable

high motion
→ shorter GOP may be preferable
```

Do NOT create an overly complex model initially.

A simple enum is sufficient:

```go
type MotionClass string

const (
	MotionLow    MotionClass = "low"
	MotionMedium MotionClass = "medium"
	MotionHigh   MotionClass = "high"
)
```

Potential adjustment:

```text
Low:
target GOP seconds +0.5 to +1

Medium:
baseline

High:
target GOP seconds -0.5 to -1
```

Always clamp to safe limits.

---

# 17. Scene-change density

If scene-change data is available:

```text
many scene cuts
→ Adaptive I becomes more valuable
→ shorter effective GOP may occur naturally
```

Do not force every scene change into a keyframe unless encoder policy specifically requires it.

Use scene-change density as a recommendation signal, not a hard count.

---

# 18. First implementation should remain deterministic

Do NOT build an ML model initially.

Use deterministic calculations so Profile Lab can explain its recommendation.

Example output:

```text
Recommended GOP: 90 frames (~3.0 s)

Reasoning:
- Source average GOP is 82.7 frames (~2.76 s)
- Source uses max B-run of 3
- Content is animation
- QSV worker supports Adaptive I
- Balanced compatibility policy selected

Recommended max B-frames: 3

Adaptive I: ON
Adaptive B: OFF
```

The user should be able to understand why MVForge recommended each value.

---

# 19. Suggested recommendation algorithm — v1

Pseudo logic:

```text
INPUT:
fps
sourceAverageGOP
sourceMaxBRun
contentType
motionClass
encoder
qualityPreset
workerCapabilities
```

Step 1:

```text
sourceGOPSeconds =
sourceAverageGOP > 0
? sourceAverageGOP / fps
: defaultGOPSeconds
```

Step 2:

```text
baseline = clamp(sourceGOPSeconds, 2.0, 4.0)
```

Step 3: content adjustment.

Example:

```text
anime:
  +0.25 to +0.5 sec

high motion:
  -0.5 sec

compatibility policy:
  cap at 2 sec
```

Step 4:

```text
targetGOPFrames =
round(fps × adjustedSeconds)
```

Step 5: normalize to reasonable frame values.

Examples:

```text
58 → 60
89 → 90
118 → 120
```

Do not over-normalize fractional broadcast rates incorrectly.

Step 6: B-frame recommendation.

```text
if sourceMaxBRun between 1 and 4:
    maxBFrames = sourceMaxBRun
else:
    maxBFrames = safe encoder default
```

Then clamp:

```text
0..4
```

for the first version unless the encoder-specific policy explicitly supports a larger safe value.

Step 7:

```text
Adaptive I =
preset allows advanced features
AND worker validates Adaptive I

Adaptive B =
preset allows advanced features
AND worker validates Adaptive B
```

Do not automatically enable Adaptive B just because source uses many B-frames.

---

# 20. Recommended result model

Potential Go model:

```go
type FrameStructureRecommendation struct {
	TargetGOPFrames  int      `json:"targetGopFrames"`
	TargetGOPSeconds float64  `json:"targetGopSeconds"`
	MaxBFrames       int      `json:"maxBFrames"`

	AdaptiveI bool `json:"adaptiveI"`
	AdaptiveB bool `json:"adaptiveB"`

	SourceAverageGOP float64 `json:"sourceAverageGop"`
	SourceMaxBRun    int     `json:"sourceMaxBRun"`
	SourceBRatio     float64 `json:"sourceBRatio"`

	Confidence string   `json:"confidence"`
	Reasons    []string `json:"reasons"`
	Warnings   []string `json:"warnings"`
}
```

Possible confidence:

```text
high
medium
low
```

High confidence could require:

```text
FPS known
source GOP measured
source B-run measured
worker capabilities available
```

---

# 21. Encoder-specific policy

Do not assume one recommendation works identically for:

```text
libx265
hevc_qsv
hevc_videotoolbox
```

The generic analyzer may produce:

```text
GOP target
B-depth target
```

Then an encoder-specific layer translates that into supported options.

Example:

```text
Generic:
GOP = 90
Max B = 3
```

QSV builder:

```text
-g 90
-bf 3
```

x265 may map equivalent concepts through x265 options.

VideoToolbox may have different effective behavior or constraints.

The recommendation engine should remain encoder-aware.

---

# 22. Requested vs effective frame structure

MVForge already follows a requested/effective philosophy.

Apply it here too.

Record:

```text
Recommended:
GOP 90
Max B 3
Adaptive I ON
Adaptive B OFF

Requested:
GOP 90
Max B 3

Effective command:
-g 90
-bf 3
-adaptive_i 1
```

Then inspect actual output:

```text
Actual:
Average GOP 92
B-run max 3
B ratio 65%
```

This would be healthy.

But:

```text
Actual:
Average GOP 248
B-run max 247
P frames 0
```

should trigger a major deviation warning.

---

# 23. Validation thresholds

Do NOT require exact equality.

For GOP:

```text
actual average GOP
may differ from requested because of scene cuts / adaptive keyframes
```

Use tolerance.

Example:

```text
Target GOP = 90

Normal:
60–120 depending on scene behavior

Warning:
average GOP > ~1.5–2× requested target

Strong warning:
extreme value plus P-frame disappearance/B-run explosion
```

For B-run:

```text
requested maxBFrames = 3
```

If output analyzer reports:

```text
B-run max = 3 or 4
```

likely acceptable.

If:

```text
B-run max = 247
```

this is not a small encoder deviation.

Flag it.

---

# 24. Output health rules

Do not rely on one number.

Use combinations.

Example strong warning:

```text
output P frames == 0
AND
output B ratio >= 95%
AND
output max B-run > 20
```

Additional source-relative warning:

```text
output B-run > source B-run × large factor
```

Example:

```text
source = 3
output = 247
```

This should clearly be flagged.

---

# 25. UI recommendation

Profile Lab should display something like:

```text
Frame Structure Recommendation

Source
GOP: 82.7
Max B-run: 3
B ratio: 58%

Recommended
GOP: 90 (~3.0 sec)
Max B-frames: 3
Adaptive I: Yes
Adaptive B: No

Why
• Source GOP is approximately 2.8 seconds
• Source uses short B-frame runs
• Animation profile favors moderate GOP length
• Active QSV worker supports Adaptive I
```

After preview:

```text
Actual Output
GOP: 92
Max B-run: 3
B ratio: 63%

Verdict:
Within expected structure
```

or:

```text
Actual Output
GOP: 248
Max B-run: 247
B ratio: 99.6%
P frames: 0

Verdict:
Unusual frame structure
```

---

# 26. Do not auto-apply initially

For the first implementation:

```text
ANALYZE
→ RECOMMEND
→ USER REVIEWS
→ PROFILE LAB PREVIEW
```

Do NOT silently change production profiles.

Once the recommendation has been validated across representative assets and encoders, MVForge may later support:

```text
Apply recommendation
```

explicitly.

---

# 27. Tests Codex should add

Add unit tests for recommendation logic.

Examples:

### Source with normal 3-second GOP

```text
fps = 30
average GOP = 90
B-run = 3

expected:
GOP around 90
B max around 3
```

### Excessively long source GOP

```text
fps = 30
average GOP = 600

expected:
recommendation capped to safe range
```

### Missing source GOP data

```text
fps = 23.976
average GOP = 0

expected:
fallback deterministic recommendation
```

### Extreme B-run

```text
source B-run = 50

expected:
do not recommend B=50
cap to safe encoder value
```

### Unsupported Adaptive B

```text
worker Adaptive B capability = false

expected:
Adaptive B recommendation/effective = false
```

### Low quality preset

```text
preset < High Quality

expected:
advanced Adaptive I/B defaults remain disabled according to MVForge policy
```

---

# 28. Important constraints

Do NOT:

```text
copy exact source I/P/B counts
force B-frame percentage
infer Adaptive B from B count
infer Adaptive I from I count
enable ExtBRC to fix GOP
enable Look Ahead to fix GOP
ignore active worker capability
assume every encoder interprets GOP/B settings identically
```

Do:

```text
calculate target GOP from time/FPS
use source GOP as a signal
use B-run as a signal
apply encoder-specific safe limits
respect worker capabilities
record reasoning
compare recommendation/request/effective/actual
```

---

# 29. Initial objective

The first useful version should answer:

```text
For this asset and this selected encoder,
what GOP and B-frame depth would be a reasonable starting point?
```

For the representative source:

```text
FPS ≈ 29.97
Average source GOP = 82.7
Source B-run max = 3
B ratio = 58%
```

a reasonable result might be approximately:

```text
Target GOP:
90–120 frames

Preferred starting point:
~90

Max B-frames:
3

Adaptive I:
enable if profile policy and worker capability allow

Adaptive B:
optional; do not infer from source B-frame count
```

This is a recommendation, not a universal fixed answer.

The final value should come from deterministic asset analysis + encoder policy + active worker capabilities.

---

# 30. Core principle

The goal of this feature is NOT:

> Make the output frame structure identical to the source.

The goal is:

> Produce an encoder-appropriate, predictable and explainable frame structure, then verify that the encoder actually respected it within reasonable tolerances.

MVForge should treat frame structure the same way it treats other fidelity decisions:

```text
Source
→ Recommendation
→ Requested
→ Effective
→ Actual
→ Verdict
```

That architecture must be preserved.