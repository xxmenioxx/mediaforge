# MVForge – Context for Codex: VideoToolbox Realtime, B-Frames, and Bitrate Adjustment

## Objective

Review and implement the VideoToolbox changes discussed for MVForge:

1. Restore the `realtime` control.
2. Add an explicit B-frame policy.
3. Stop converting missing B-frame values into `-bf 0`.
4. Validate B-frame behavior with real probes.
5. Adjust VideoToolbox bitrate estimates according to the effective B-frame strategy.
6. Recalculate `maxrate` and `bufsize` from the final effective bitrate.
7. Preserve requested and effective configuration separately.
8. Keep this work isolated from QSV and other encoders.

This task applies only to:

- `h264_videotoolbox`
- `hevc_videotoolbox`

Do not modify QSV, NVENC, VAAPI, AMF, libx264, libx265, or AV1 unless shared infrastructure is reused without changing their behavior.

---

## 1. Design principle

Separate these concepts:

```text
Quality Intent
Capability Resolution
Encoder Strategy
Effective Rate Control
FFmpeg Command
```

Preferred flow:

```text
Asset Analysis
        ↓
Base VideoToolbox Target
        ↓
Capability Resolver
        ↓
Effective Realtime / B-frame Strategy
        ↓
Bitrate Efficiency Adjustment
        ↓
Effective Target / Maxrate / Buffer
        ↓
FFmpeg Command
```

Do not map presets directly to fixed FFmpeg flags.

---

## 2. Restore `realtime`

The VideoToolbox model and UI must include:

```text
Realtime
```

Suggested model:

```go
type VideoToolboxSettings struct {
    Realtime bool `json:"realtime"`
}
```

Translation:

```text
realtime=false -> -realtime 0
realtime=true  -> -realtime 1
```

Default policy:

```text
Normal offline conversion -> realtime=false
Preview, capture, or explicit low-latency profile -> realtime=true
```

Do not infer `realtime=1` merely because VideoToolbox is used.

Persist requested and effective values separately.

---

## 3. Explicit B-frame policy

Add three distinct states:

```text
Auto
Enabled
Disabled
```

Suggested model:

```go
type BFramePolicy string

const (
    BFrameAuto     BFramePolicy = "auto"
    BFrameEnabled  BFramePolicy = "enabled"
    BFrameDisabled BFramePolicy = "disabled"
)

type VideoToolboxBFrameSettings struct {
    Policy BFramePolicy `json:"policy"`
    Count  *int         `json:"count,omitempty"`
}
```

Do not use a plain integer where zero can ambiguously mean unset, automatic, disabled, missing JSON, or Go zero value.

Backward compatibility:

```text
Missing policy in old profiles -> Auto
```

Never migrate a missing value to Disabled.

---

## 4. FFmpeg mapping

### Auto

Do not emit `-bf`.

```bash
-c:v hevc_videotoolbox
```

### Enabled

Emit a positive value only when the worker probe confirms effectiveness.

Default:

```bash
-bf 3
```

Suggested UI range:

```text
1–4
```

### Disabled

Emit:

```bash
-bf 0
```

Do not emit `-bf 0` because a field is absent, null, empty, or zero-initialized.

---

## 5. Defaults by job type

Suggested initial defaults:

```text
Offline conversion -> Auto
Best / High Quality -> Enabled only if a real probe confirms effectiveness
Realtime / low latency -> Disabled
Lab preview -> Auto unless explicitly low latency
Custom -> Respect user selection, subject to validation
```

The preset expresses quality intent; the capability resolver determines effective encoder strategy.

---

## 6. Find the current source of `-bf 0`

Before editing, trace where `-bf 0` currently comes from.

Inspect:

- backend profile defaults;
- preset tables;
- frontend form defaults;
- JSON serialization/deserialization;
- database persistence and migrations;
- profile normalization;
- capability resolver;
- fallback logic;
- FFmpeg command builder;
- old stored profiles.

Report the root cause in no more than 10 lines.

Do not fix this only by deleting `-bf 0` from the final command string. Fix the state-modeling issue at its source.

---

## 7. Real VideoToolbox probe

The output of:

```bash
ffmpeg -hide_banner -h encoder=hevc_videotoolbox
```

is not sufficient.

Use real probes with:

```bash
-f lavfi -i testsrc2=size=1280x720:rate=30
```

### Auto

```bash
ffmpeg -hide_banner -y \
  -f lavfi \
  -i testsrc2=size=1280x720:rate=30 \
  -t 5 \
  -c:v hevc_videotoolbox \
  -b:v 3M \
  -realtime 0 \
  output-auto.mp4
```

### Disabled

```bash
ffmpeg -hide_banner -y \
  -f lavfi \
  -i testsrc2=size=1280x720:rate=30 \
  -t 5 \
  -c:v hevc_videotoolbox \
  -b:v 3M \
  -realtime 0 \
  -bf 0 \
  output-bf0.mp4
```

### Enabled

```bash
ffmpeg -hide_banner -y \
  -f lavfi \
  -i testsrc2=size=1280x720:rate=30 \
  -t 5 \
  -c:v hevc_videotoolbox \
  -b:v 3M \
  -realtime 0 \
  -bf 3 \
  output-bf3.mp4
```

Run the same matrix for `h264_videotoolbox` when available.

Use temporary paths and clean up files afterward.

---

## 8. Effective B-frame validation

Do not treat exit code zero as proof.

Inspect frame types:

```bash
ffprobe -v error \
  -select_streams v:0 \
  -show_entries frame=pict_type \
  -of csv=p=0 \
  output.mp4
```

Interpretation:

```text
-bf 3 accepted + B-frames observed -> Enabled effective
-bf 3 accepted + zero B-frames -> Accepted but ineffective
-bf 0 accepted + zero B-frames -> Disabled effective
-bf 0 accepted + B-frames observed -> Option not respected
```

Also inspect where available:

- `has_b_frames`;
- DTS/PTS reordering;
- profile;
- level;
- decoder delay.

Persist enough evidence to reproduce the result.

---

## 9. Requested vs effective configuration

Store both separately.

Example success:

```json
{
  "requested": {
    "realtime": false,
    "bFramePolicy": "enabled",
    "bFrames": 3
  },
  "effective": {
    "realtime": false,
    "bFramePolicy": "enabled",
    "bFramesObserved": true,
    "observedBFrameCount": 100
  },
  "downgradeReason": null
}
```

Example fallback:

```json
{
  "requested": {
    "realtime": false,
    "bFramePolicy": "enabled",
    "bFrames": 3
  },
  "effective": {
    "realtime": false,
    "bFramePolicy": "auto",
    "bFramesObserved": false
  },
  "downgradeReason": "VideoToolbox accepted -bf 3 but produced no B-frames"
}
```

Always store requested settings, emitted command, probe evidence, effective settings, downgrade reason, and warnings.

---

## 10. Fallback rules

### Requested Enabled

- Effective -> Enabled
- Accepted but ineffective -> fallback to Auto
- Rejected -> fallback to Auto, unless the profile explicitly requires B-frames and should fail

### Requested Disabled

- Respected -> Disabled
- Accepted but B-frames remain -> fail strict low-latency profiles or continue only when policy allows; record effective behavior as unknown/auto, not Disabled

### Requested Auto

Emit no `-bf` and do not downgrade Auto.

---

## 11. B-frame-aware bitrate adjustment

The adaptive bitrate engine already calculates a base VideoToolbox target from the asset.

That base should consider:

- source video bitrate;
- effective output resolution after crop/scale;
- grain;
- motion;
- content type;
- frame rate;
- quality intent;
- source and output bit depth;
- calibration history.

Then apply an efficiency adjustment according to the effective B-frame strategy.

Initial configurable multipliers:

```text
B-frames effective -> 1.00
Auto, behavior unconfirmed -> 1.03
B-frames disabled or unsupported -> 1.08
```

These are initial heuristics, not universal constants.

Keep them centralized in backend, configurable, visible in the calculation explanation, and replaceable by measured calibration data.

Example:

```text
Base target from asset analysis: 1650 kbps
Effective B-frame strategy: Disabled
Efficiency multiplier: 1.08
Effective target: 1782 kbps
```

Round only to a practical kbps step such as 25k or 50k. Never round to whole Mbps.

---

## 12. Maxrate and buffer

Calculate them after the B-frame adjustment:

```text
effectiveTarget = baseTarget × bFrameMultiplier
maxrate = effectiveTarget × maxrateMultiplier
buffer = effectiveTarget × bufferMultiplier
```

For Recommended:

```text
maxrateMultiplier = 1.50
bufferMultiplier = 2.50
```

Example:

```text
Base target:      1650 kbps
B-frame adjust:   +8%
Effective target: 1780 kbps
Maxrate:          2670 kbps
Buffer:           4450 kb
```

Generate:

```bash
-b:v 1780k
-maxrate 2670k
-bufsize 4450k
```

Never round these values to whole megabits in the command builder.

---

## 13. Custom behavior

Custom must expose:

```text
Realtime
B-frame policy
Maximum B-frames
Target bitrate
Maxrate
Buffer
Auto-adjust bitrate for encoder strategy
```

When the user manually sets target, maxrate, or buffer:

- do not overwrite those values;
- validate coherence;
- record that they are manual;
- show warnings when needed.

The automatic B-frame adjustment must be disabled for a manual target unless the user explicitly enables:

```text
Auto-adjust bitrate for encoder strategy
```

---

## 14. UI requirements

In VideoToolbox controls show:

```text
Realtime
Off / On

B-frames
Auto / Enabled / Disabled

Maximum B-frames
1–4
```

Also display:

```text
Base target
B-frame policy
Efficiency adjustment
Effective target
Maxrate
Buffer
```

Show requested and effective settings and any downgrade reason.

---

## 15. Command examples

### Auto, offline

```bash
-c:v hevc_videotoolbox \
-realtime 0
```

No `-bf`.

### Enabled

```bash
-c:v hevc_videotoolbox \
-realtime 0 \
-bf 3
```

Only when the probe confirms effectiveness.

### Disabled, low latency

```bash
-c:v hevc_videotoolbox \
-realtime 1 \
-bf 0
```

---

## 16. Required tests

Add tests for:

1. `realtime=false` emits `-realtime 0`.
2. `realtime=true` emits `-realtime 1`.
3. Offline jobs default to realtime off.
4. Auto does not emit `-bf`.
5. Enabled emits the configured positive value.
6. Disabled emits `-bf 0`.
7. Missing policy becomes Auto.
8. Missing JSON does not become Disabled.
9. Empty UI values do not become zero.
10. Old profiles migrate to Auto.
11. B-frames effective use multiplier 1.00.
12. Auto/unverified uses the configured intermediate multiplier.
13. Disabled/unsupported uses the configured penalty multiplier.
14. Adjustment happens before maxrate and buffer calculation.
15. Maxrate and buffer use the effective target.
16. Values remain precise in kbps.
17. Custom manual values are not overwritten.
18. Auto-adjust for Custom works only when explicitly enabled.
19. Requested and effective states are persisted separately.
20. B-frame fallback recalculates the effective bitrate.
21. UI estimates update when B-frame policy changes.
22. VideoToolbox profile/pixel-format validation remains intact.
23. QSV and other encoders remain unchanged.
24. `-bf 3` accepted without B-frames is marked ineffective.
25. `-bf 0` accepted with B-frames is marked not respected.

---

## 17. Acceptance criteria

The task is complete when:

- `realtime` is restored in backend and UI;
- offline jobs explicitly emit `-realtime 0`;
- realtime/low-latency jobs can emit `-realtime 1`;
- Auto, Enabled, and Disabled B-frame policies are distinct;
- missing values no longer produce `-bf 0`;
- B-frame support is validated by inspecting output frames;
- requested and effective behavior are stored separately;
- the bitrate engine considers effective B-frame strategy;
- maxrate and buffer are recalculated from the final target;
- manual Custom values remain untouched unless auto-adjust is enabled;
- UI explains every adjustment and downgrade;
- other encoders are unaffected.

---

## Work instructions for Codex

Before editing:

1. Locate the VideoToolbox profile model.
2. Locate where `realtime` was removed or hidden.
3. Locate preset defaults.
4. Locate frontend defaults.
5. Locate JSON serialization/deserialization.
6. Locate persistence and migrations.
7. Locate the B-frame capability probe.
8. Locate the adaptive bitrate calculator.
9. Locate maxrate/buffer calculation.
10. Locate the FFmpeg command builder.
11. Explain in no more than 12 lines why `realtime` is absent, where `-bf 0` comes from, and whether bitrate currently ignores B-frame strategy.

During implementation:

- Keep scope limited to VideoToolbox.
- Do not refactor unrelated encoders.
- Use small pure functions.
- Preserve requested and effective configuration separately.
- Store probe evidence.
- Keep heuristic multipliers configurable.
- Do not hardcode assumptions for one Mac model.

After implementation:

1. List modified files.
2. Show previous and corrected commands.
3. Show Auto, Enabled, and Disabled examples.
4. Show realtime off/on examples.
5. Show bitrate calculations with effective and disabled B-frames.
6. Show requested/effective fallback examples.
7. Show unit-test results.
8. Show real probe results when available.
9. Document migrations.
10. Document limitations and pending validation.
