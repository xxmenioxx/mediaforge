# MVForge — Codex Context: Asset-Derived GOP Recommendation by Time

## Goal

Update the frame-structure recommendation logic so GOP is calculated primarily from **time in seconds**, not from a fixed frame count.

The reason is simple:

```text
-g 120
```

means very different things depending on FPS.

Examples:

```text
23.976 fps → ~5.0 seconds
29.97 fps  → ~4.0 seconds
59.94 fps  → ~2.0 seconds
```

MVForge should therefore reason in:

```text
GOP seconds
```

and only translate to:

```text
GOP frames
```

when building the encoder configuration.

Do NOT hardcode one GOP frame value for all assets.

---

# 1. Core calculation

Use:

```text
targetGOPFrames =
round(FPS × targetGOPSeconds)
```

Example:

```text
23.976 fps × 3.0 s ≈ 72 frames
29.97 fps × 3.0 s ≈ 90 frames
59.94 fps × 3.0 s ≈ 180 frames
```

The recommendation engine should internally keep both:

```text
targetGOPSeconds
targetGOPFrames
```

Example:

```json
{
  "targetGopSeconds": 3.75,
  "targetGopFrames": 90
}
```

---

# 2. Source GOP should be converted to seconds

The source analyzer already produces/should produce:

```text
average GOP in frames
FPS
```

Derive:

```text
sourceGOPSeconds =
sourceAverageGOP / FPS
```

This is a much more useful normalization metric than raw GOP frames.

Example Rayearth:

```text
Source average GOP = 71.7
FPS ≈ 23.976

71.7 / 23.976 ≈ 2.99 seconds
```

So Rayearth's source is effectively using a ~3 second GOP.

Example Arbegas:

```text
Source average GOP = 75.4
FPS ≈ 29.97

75.4 / 29.97 ≈ 2.52 seconds
```

So Arbegas is around ~2.5 seconds per GOP.

---

# 3. GOP ranges for automatic recommendation

Use GOP time ranges rather than fixed frame ranges.

Suggested initial policy:

```text
~1–2 seconds
→ very conservative
→ frequent keyframes
→ strong seek/compatibility bias

~2–4 seconds
→ normal/recommended VOD range

~4–6 seconds
→ more compression-oriented

~6–8 seconds
→ long GOP
→ only recommend automatically with strong confidence

>8 seconds
→ do not normally auto-recommend

>10 seconds
→ Custom/manual territory
```

These are policy guardrails, not codec hard limits.

Do not claim that HEVC cannot support larger GOPs.

The point is:

> MVForge should avoid automatically recommending unnecessarily extreme GOPs.

---

# 4. Automatic hard/soft limits

Suggested initial limits:

```text
minimum automatic GOP:
~2.0 seconds

normal automatic range:
~2.0–5.0 seconds

soft automatic maximum:
~6.0 seconds

hard automatic recommendation cap:
~8.0 seconds
```

The user may still use:

```text
Custom
```

to configure a larger GOP manually.

Do not silently clamp Custom unless an encoder capability/technical limit requires it.

---

# 5. Source-derived baseline

Start with:

```text
sourceGOPSeconds
```

when source data is valid.

Then normalize it into a sensible baseline.

Conceptually:

```text
baselineSeconds =
clamp(sourceGOPSeconds, 2.0, 4.0)
```

Example:

```text
source = 2.99 sec
→ baseline ≈ 2.99 sec

source = 0.7 sec
→ baseline ≈ 2.0 sec

source = 12 sec
→ baseline ≈ 4.0 sec
```

Do NOT blindly preserve extreme source GOP values.

The source is a signal, not an absolute target.

---

# 6. Interaction with Frame Structure Mode dropdown

The new dropdown already defined is:

```text
Compatible
Balanced
Maximum Compression
Custom
```

The mode should influence the GOP recommendation in **seconds**.

Do not use fixed GOP frame values per mode.

Conceptually:

```text
source-derived baseline
        ↓
Frame Structure Mode
        ↓
target GOP seconds
        ↓
FPS conversion
        ↓
target GOP frames
```

---

# 7. Compatible mode

Goal:

```text
prefer predictability, seek behavior and compatibility
```

Suggested behavior:

```text
targetSeconds ≈ baselineSeconds - 0.0 to 0.5 seconds
```

Clamp to:

```text
~2–3 seconds
```

for most assets.

Do not automatically shorten a source GOP that is already sensible unless policy requires it.

Example Rayearth:

```text
source ≈ 2.99 sec

Compatible:
~3.0 sec

23.976 × 3.0 ≈ 72
```

So:

```text
-g 72
```

is a very sensible Compatible recommendation.

---

# 8. Balanced mode

Balanced should be the preferred/default VOD mode.

Suggested behavior:

```text
targetSeconds ≈ baselineSeconds + 0.0 to 0.75 sec
```

Typical target:

```text
~3–4 seconds
```

Example Rayearth:

```text
baseline ≈ 3.0 sec

Balanced ≈ 3.75 sec

23.976 × 3.75 ≈ 90
```

This matches the recent successful test:

```text
Requested GOP: 90
Measured GOP: 90.0
```

This is a good example of Balanced intentionally using a GOP longer than the source while still remaining reasonable.

---

# 9. Maximum Compression mode

Suggested behavior:

```text
targetSeconds ≈ baselineSeconds + 1–2 sec
```

Typical target:

```text
~4–5 seconds
```

Potentially up to:

```text
~6 seconds
```

if:

```text
source analysis is consistent
worker capability is healthy
preview validation passes
```

Do NOT automatically jump to 8–10 seconds simply because the user selected Maximum Compression.

The mode means:

```text
more compression-oriented
```

not:

```text
maximum possible GOP
```

---

# 10. Example: Rayearth

Observed source:

```text
Frames sampled: 2399

I: 39
P: 813
B: 1547

Average GOP: 71.7
```

FPS approximately:

```text
23.976
```

Derived:

```text
source GOP seconds ≈ 2.99
```

Suggested modes:

```text
Compatible:
~3.0 sec
→ ~72 frames

Balanced:
~3.75 sec
→ ~90 frames

Maximum Compression:
~4.5–5.0 sec
→ ~108–120 frames
```

Recent Balanced-like test:

```text
-g 90
-bf 3
-adaptive_i 1
-adaptive_b 1
```

produced:

```text
Source GOP: 71.7
Requested GOP: 90
Measured GOP: 90.0
```

This is healthy.

Do NOT warn simply because:

```text
90 > 71.7
```

The user/request explicitly selected a longer GOP.

Preferred interpretation:

```text
Source GOP:      71.7
Requested GOP:   90
Measured GOP:    90.0

Verdict:
Matches requested GOP
```

---

# 11. Example: Arbegas

Observed source:

```text
Average GOP: 75.4
FPS ≈ 29.97
```

Derived:

```text
75.4 / 29.97 ≈ 2.52 seconds
```

Possible recommendations:

```text
Compatible:
~2.5 sec
→ ~75 frames

Balanced:
~3.0–3.5 sec
→ ~90–105 frames

Maximum Compression:
~4–5 sec
→ ~120–150 frames
```

The exact choice should still depend on asset analysis/confidence.

Do not hardcode these frame values.

Calculate them from seconds.

---

# 12. Do not evaluate output GOP only against source

This is a critical change.

Current interpretation may effectively do:

```text
source GOP
vs
output GOP
```

That is insufficient once MVForge intentionally recommends a different GOP.

The Lab should compare:

```text
Source GOP
Requested GOP
Effective GOP
Measured GOP
```

Priority:

```text
Requested vs Effective
Requested vs Measured
```

The source remains contextual information.

Example:

```text
Source:     71.7
Requested:  90
Effective:  90
Measured:   90.0
```

This should be:

```text
Healthy
Matches requested GOP
```

not:

```text
output differs from source
```

---

# 13. Suggested tolerance

Do not require exact equality between requested and measured average GOP.

Adaptive I / scene cuts may legitimately reduce average GOP.

Suggested initial interpretation:

```text
absolute/relative match very close
→ Matches requested GOP

small/moderate deviation
→ Within expected variation

large deviation
→ Review

extreme deviation
→ Warning
```

Potential first-pass relative thresholds:

```text
≤10%
→ strong match

10–25%
→ normal/review tolerance depending on adaptive I / scene changes

25–50%
→ review

>50%
→ strong review/warning
```

Do not apply these blindly if the asset contains many scene cuts.

Use effective Adaptive I state/context where available.

---

# 14. GOP can legitimately be shorter than requested

If:

```text
Adaptive I = ON
```

or scene-cut handling inserts additional I frames, the measured average GOP may be shorter than the maximum `-g`.

Remember:

```text
-g
```

is effectively a maximum/target GOP size, not a guarantee that every GOP must have exactly that length.

Therefore:

```text
requested = 90
measured = 75
```

may still be healthy if scene structure explains it.

Do not report this automatically as encoder failure.

---

# 15. A measured GOP larger than requested is more suspicious

If:

```text
requested GOP = 90
```

but:

```text
measured average GOP = 180+
```

that deserves stronger review because the output is exceeding the intended keyframe interval substantially.

Suggested validation direction:

```text
measured much shorter than requested
→ investigate scene cuts / Adaptive I first

measured much longer than requested
→ investigate encoder effective configuration
```

---

# 16. Five-window sampling

Continue using time-based distributed sampling.

Preferred:

```text
5 windows × 20 seconds
```

distributed around:

```text
10%
30%
50%
70%
90%
```

or equivalent non-edge positions.

Do NOT return to fixed:

```text
500 frames
```

because 500 frames represents different durations at different FPS.

---

# 17. GOP must be calculated independently per sample

Each window should calculate its own GOP statistics.

Do not connect:

```text
last keyframe of window A
```

to:

```text
first keyframe of window B
```

because those samples are separated in time.

Aggregate:

```text
per-window GOP observations
```

afterward.

---

# 18. Use variability across windows

Do not only use global average GOP.

Example consistent asset:

```text
Window GOP:
72
74
71
73
72
```

This is stable.

Recommendation confidence:

```text
High
```

Example inconsistent asset:

```text
72
75
180
68
240
```

This should lower recommendation confidence.

The average alone may hide important variation.

---

# 19. Suggested confidence calculation

A simple deterministic v1 is enough.

Possible inputs:

```text
number of valid windows
variance / coefficient of variation of GOP seconds
FPS consistency
cadence consistency
scene-change density
```

Suggested result:

```text
High
Medium
Low
```

No ML is needed.

---

# 20. GOP recommendation should be conservative when confidence is low

Example:

```text
source windows vary strongly
```

Then do NOT choose:

```text
Maximum Compression → very long GOP
```

Instead:

```text
use shorter/more conservative target
lower confidence
show Review recommendation
```

Potential rule:

```text
Low confidence
→ cap automatic GOP around 3–4 seconds
```

even if Maximum Compression was selected.

The UI can explain the downgrade.

---

# 21. Motion can influence GOP recommendation

If motion classification already exists or is added later:

```text
low motion
→ slightly longer GOP may be acceptable

medium motion
→ baseline

high motion
→ favor shorter GOP
```

Keep the adjustment small.

Example:

```text
low motion:
+0.25–0.5 sec

high motion:
-0.25–0.75 sec
```

Always clamp to safety range.

Do NOT make motion the only decision factor.

---

# 22. Scene cuts

If scene-change density is high:

```text
Adaptive I becomes more useful
actual average GOP may naturally shorten
```

Do not force every scene change to align with the nominal GOP.

GOP recommendation is a maximum/target structure, not an exact repeated pattern.

---

# 23. Content type

Content type may make small adjustments.

Examples:

```text
Anime / animation:
moderate GOP can work well
often stable/static regions

Film:
~2–4 sec baseline reasonable

High-motion sports:
favor shorter GOP

Compatibility-first:
favor shorter GOP
```

Do not create wildly different algorithms per content type in v1.

Use content type as a modifier.

---

# 24. Encoder awareness

The recommendation is generic in seconds but the final configuration must remain encoder-aware.

Conceptually:

```text
Recommended:
3.75 sec GOP

QSV @ 23.976
→ -g 90

QSV @ 29.97
→ -g 112/round normalized

x265
→ encoder-specific translation if needed

VideoToolbox
→ encoder-specific translation
```

Do not assume every encoder behaves identically.

---

# 25. Normalization of frame GOP values

After:

```text
FPS × targetSeconds
```

the result may be awkward.

Example:

```text
29.97 × 3.5 ≈ 104.895
```

Round sensibly.

Do not over-normalize.

Potential rule:

```text
round to nearest integer
```

is acceptable initially.

Optional later:

```text
normalize near common values such as
48 / 60 / 72 / 75 / 90 / 96 / 120
```

but only if this does not distort the intended duration significantly.

Keep v1 deterministic and simple.

---

# 26. Frame Structure Mode should display both seconds and frames

Suggested Advanced UI:

```text
GOP
90 frames · 3.75 sec
```

or:

```text
GOP Frames
90

GOP Duration
~3.75 s
```

The technical user can understand the actual FFmpeg value while the non-technical user understands the time interval.

---

# 27. Custom mode

When mode is:

```text
Custom
```

the user may directly edit:

```text
GOP frames
```

If practical, update the displayed seconds automatically:

```text
gopSeconds = gopFrames / fps
```

Example:

```text
Custom GOP: 240
FPS: 23.976

UI:
240 frames · ~10.0 sec
```

This is useful because it makes extreme GOP values obvious.

---

# 28. Warn on extreme Custom GOP

Do not prevent advanced users from entering large GOP values unless technically invalid.

But show warnings.

Suggested UI interpretation:

```text
≤6 sec
normal/advanced

6–8 sec
long GOP

8–10 sec
very long GOP

>10 sec
extreme GOP
```

Potential message:

```text
This GOP is unusually long for general VOD.
Seek granularity and recovery points will be less frequent.
```

Do not say the file will necessarily be broken.

---

# 29. Preset + recommendation example in UI

Rayearth:

```text
Source GOP
71.7 frames · ~3.0 sec

Frame Structure Mode
Balanced

Recommended GOP
90 frames · ~3.75 sec

Preview result
90.0 frames · ~3.75 sec

Verdict
Matches requested GOP
```

This is the desired UX.

---

# 30. Do not modify GPB interpretation

This GOP work is separate from the existing GPB discovery.

Preserve the current QSV-specific interpretation:

```text
hevc_qsv + GPB ON + GopRefDist == 1
→ qsv_gpb

hevc_qsv + GPB ON + GopRefDist > 1
→ qsv_mixed_b_gpb
```

Do NOT restore generic warnings based solely on:

```text
P = 0
B ≈ 100%
```

---

# 31. Do not modify existing Adaptive I/B rules

Existing rules remain:

```text
Adaptive I
→ available if capability supports it

Adaptive B
→ mainly relevant when bf > 0

P Strategy
→ mainly relevant when bf == 0

B reference strategy
→ relevant when bf > 0
```

This task is specifically about GOP recommendation and validation.

---

# 32. Requested / Effective / Actual model

Preserve:

```text
Source
Recommendation
Requested
Effective
Actual
Verdict
```

For GOP specifically:

```text
Source GOP:
71.7

Recommended:
90

Requested:
90

Effective GopPicSize:
90

Measured:
90.0

Verdict:
Matches requested GOP
```

This is much more useful than comparing output directly with source.

---

# 33. Suggested model fields

Use existing models where possible.

Conceptually:

```go
type GOPRecommendation struct {
    SourceFrames      float64 `json:"sourceFrames"`
    SourceSeconds     float64 `json:"sourceSeconds"`

    TargetFrames      int     `json:"targetFrames"`
    TargetSeconds     float64 `json:"targetSeconds"`

    Mode              string  `json:"mode"`
    Confidence        string  `json:"confidence"`

    Reasons           []string `json:"reasons"`
    Warnings          []string `json:"warnings"`
}
```

Do not force this exact struct if the existing recommendation model already has an appropriate place.

Avoid duplicate state.

---

# 34. Suggested algorithm v1

Pseudo logic:

```text
INPUT:
fps
sourceAverageGOP
five-window GOP stats
frameStructureMode
motionClass
contentType
recommendationConfidence
```

Calculate:

```text
sourceSeconds =
sourceAverageGOP / fps
```

Then:

```text
baseline =
clamp(sourceSeconds, 2.0, 4.0)
```

Mode adjustment:

```text
Compatible:
target = baseline - 0.0..0.5

Balanced:
target = baseline + 0.0..0.75

Maximum Compression:
target = baseline + 1.0..2.0
```

Then apply:

```text
motion adjustment
confidence adjustment
```

Clamp:

```text
automatic target:
2.0..8.0 sec
```

Prefer:

```text
2.0..6.0 sec
```

unless strong evidence supports longer.

Finally:

```text
targetFrames =
round(fps × targetSeconds)
```

Return both seconds and frames.

---

# 35. Do not use random ranges

The implementation must be deterministic.

The pseudo values:

```text
+0.0..0.75
```

are policy ranges, not random selection.

Define explicit deterministic rules.

Example possible v1:

```text
Compatible:
target = baseline

Balanced:
target = baseline + 0.75

Maximum Compression:
target = baseline + 2.0
```

Then clamp.

Or use existing asset characteristics to choose the deterministic adjustment.

Document the reasoning.

---

# 36. Better deterministic v1 proposal

For a simple first implementation:

```text
baseline = clamp(sourceGOPSeconds, 2.0, 4.0)
```

Then:

```text
Compatible:
target = baseline

Balanced:
target = min(baseline + 0.75, 4.0)

Maximum Compression:
target = min(baseline + 2.0, 5.5)
```

Then:

```text
if confidence == Low:
    target = min(target, 3.5)

if motion == High:
    target = max(2.0, target - 0.5)

if motion == Low && mode == MaximumCompression:
    target = min(target + 0.5, 6.0)
```

Then convert to frames.

This is only a suggested deterministic v1.

Use existing policy architecture if there is already a better centralized recommendation mechanism.

---

# 37. Rayearth expected regression test

Input approximately:

```text
fps = 23.976
sourceAverageGOP = 71.7
sourceGOPSeconds ≈ 2.99
confidence = High
motion = Medium
```

Expected approximately:

```text
Compatible:
~72 frames

Balanced:
~90 frames

Maximum Compression:
~120 frames
```

Do not require exact frame values if the implemented deterministic policy differs slightly, but verify the ordering:

```text
Compatible <= Balanced <= Maximum Compression
```

and all remain within safe automatic time limits.

---

# 38. Arbegas expected regression test

Input approximately:

```text
fps = 29.97
sourceAverageGOP = 75.4
sourceGOPSeconds ≈ 2.52
```

Expected general behavior:

```text
Compatible:
near source duration

Balanced:
moderately longer

Maximum Compression:
longer but still capped
```

Do not copy Rayearth's raw frame GOP values.

The FPS differs.

---

# 39. 60 fps regression test

This test is important.

Input:

```text
fps = 59.94
targetSeconds = 3.0
```

Expected:

```text
targetFrames ≈ 180
```

Do NOT accidentally recommend:

```text
-g 90
```

for the same 3-second target.

This is the primary reason for changing the algorithm to seconds.

---

# 40. Missing source GOP data

If source GOP cannot be measured reliably:

```text
sourceAverageGOP <= 0
```

use deterministic fallback based on mode.

Suggested:

```text
Compatible:
~2.5 sec

Balanced:
~3.5 sec

Maximum Compression:
~5.0 sec
```

Then adjust by confidence/motion if available.

Mark confidence lower.

---

# 41. Variable frame rate / unreliable FPS

If FPS is unavailable or unreliable:

do NOT produce a falsely precise GOP recommendation.

Use:

```text
confidence = Low
```

and either:

```text
fallback to reliable nominal/average FPS
```

if MVForge already has one, or avoid auto-applying until a usable frame rate is known.

Do not silently assume 30 fps.

---

# 42. Validation test

Given:

```text
requested = 90
effective GopPicSize = 90
measured = 90.0
```

Expected:

```text
Matches requested GOP
```

Given:

```text
requested = 90
effective = 90
measured = 72
AdaptiveI = ON
```

Expected:

```text
Within expected adaptive variation
```

or Review depending on scene-cut data.

Given:

```text
requested = 90
effective = 180
measured = 180
```

Expected:

```text
effective configuration mismatch
warning
```

---

# 43. UI wording

Prefer:

```text
Source GOP
Requested GOP
Measured GOP
```

over only:

```text
Average GOP
```

For example:

```text
Source:
71.7 frames · 3.0 s

Requested:
90 frames · 3.75 s

Measured:
90.0 frames · 3.75 s

Result:
Matches requested GOP
```

This makes intentional GOP changes understandable.

---

# 44. Core principle

MVForge must stop thinking:

```text
GOP = arbitrary frame number
```

and instead think:

```text
GOP = desired time interval
        ↓
converted to frames using asset FPS
```

The normal user should understand:

```text
roughly how many seconds between GOP boundaries/keyframe opportunities
```

while the advanced user can still see and edit:

```text
-g <frames>
```

The final flow is:

```text
5-window source analysis
        ↓
source GOP seconds
        ↓
Frame Structure Mode
        ↓
asset-aware target GOP seconds
        ↓
FPS conversion
        ↓
requested GOP frames
        ↓
effective QSV GopPicSize
        ↓
measured output GOP
        ↓
validation against requested value
```

Do not revert the GPB, Adaptive I/B, P Strategy, B Pyramid, capability-driven, or requested/effective work while implementing this GOP recommendation layer.