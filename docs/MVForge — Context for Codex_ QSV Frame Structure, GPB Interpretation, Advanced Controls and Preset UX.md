# MVForge — Context for Codex: QSV Frame Structure, GPB Interpretation, Advanced Controls and Preset UX

## Purpose

Implement and preserve the new QSV frame-structure behavior in MVForge without reverting the work already completed around:

- GOP
- B Frames
- Adaptive I
- Adaptive B
- P Strategy / P Pyramid
- B Pyramid / BRefType
- GPB interpretation
- Requested vs Effective state
- Profile Lab frame-structure analysis

The main goal is:

> Keep advanced QSV controls visible and editable, but add a simple user-facing `Frame Structure Mode` dropdown that populates those controls with sensible values.

The advanced fields must remain visible and modifiable.

---

# 1. Core UX decision

Add a dropdown:

```text
Frame Structure Mode

Compatible
Balanced
Maximum Compression
Custom
```

This dropdown is a preset helper.

It must NOT replace or hide the existing advanced controls.

The user should still see and be able to modify:

```text
GOP
B Frames
Adaptive I
Adaptive B
P Strategy
B Strategy / B Pyramid
GPB effective state
```

Potential future fields may include:

```text
Forced IDR
Closed GOP
```

but those are not the main requirement of this change.

---

# 2. Preset behavior

Selecting a preset populates the advanced fields.

Conceptually:

```text
Compatible
        ↓
advanced fields get conservative values

Balanced
        ↓
advanced fields get recommended VOD values

Maximum Compression
        ↓
advanced fields get more aggressive values

Custom
        ↓
advanced fields remain manually controlled
```

If the user selects:

```text
Balanced
```

and then manually changes any preset-managed field, for example:

```text
B Frames: 3 → 2
```

the mode should automatically become:

```text
Custom
```

Do not silently change it back.

---

# 3. Presets are intent, not hardcoded encoder truth

The visible presets represent user intent:

```text
Compatible
Balanced
Maximum Compression
```

but the actual values are encoder-specific.

For example:

```text
Balanced on QSV
```

does not need to map to exactly the same low-level settings as:

```text
Balanced on VideoToolbox
Balanced on libx265
```

The UI concept may be shared.

The implementation underneath must remain encoder-aware.

---

# 4. QSV preset starting points

These are reasonable initial mappings for QSV.

Do not treat them as universal hardcoded truth if the existing recommendation engine has better asset-specific values.

## Compatible

Goal:

```text
maximize predictability / compatibility
use simpler reference structure
```

Suggested starting point:

```text
GOP:
use asset recommendation
typically around ~2 seconds

B Frames:
0

Adaptive I:
ON if supported

Adaptive B:
OFF

P Strategy:
Simple or Pyramid

B Pyramid:
OFF / N/A

GPB:
effective/runtime-controlled
```

Important:

With QSV HEVC on the current NAS:

```text
-bf 0
```

does NOT result in conventional P frames.

It results in:

```text
GopRefDist = 1
GPB = ON
```

and ffprobe may report the GPB frames as `B`.

Therefore `Compatible` does not mean “no B reported by ffprobe”.

It means:

```text
no regular B-frame distance
simple GPB/P-like reference structure
```

---

## Balanced

This should be the default recommendation for general VOD use.

Suggested starting point:

```text
GOP:
asset-derived recommendation
typically ~2.5–4 seconds

B Frames:
3

Adaptive I:
ON if supported

Adaptive B:
ON if supported

P Strategy:
OFF / N/A when bf > 0

B Pyramid:
ON / Auto if supported

GPB:
effective/runtime-controlled
```

This mode should allow QSV to use a more efficient reference structure without becoming excessively aggressive.

---

## Maximum Compression

Goal:

```text
prioritize compression efficiency while keeping validated playback behavior
```

Suggested starting point:

```text
GOP:
asset recommendation with a moderately longer target

B Frames:
3–4 depending on capability

Adaptive I:
ON

Adaptive B:
ON

P Strategy:
OFF / N/A when bf > 0

B Pyramid:
ON if supported

GPB:
effective/runtime-controlled
```

Do not blindly maximize values.

Still respect:

```text
worker capability
encoder limits
asset recommendation
validation
```

---

# 5. P Strategy rule

This is important.

FFmpeg exposes:

```text
-p_strategy 0
-p_strategy 1
-p_strategy 2
```

with:

```text
0 = default
1 = simple
2 = pyramid
```

and indicates that P Strategy is intended for:

```text
bf = 0
```

Therefore MVForge should enforce:

```text
bf == 0
→ P Strategy available

bf > 0
→ P Strategy disabled / not recommended
```

Do not recommend:

```text
bf = 3
p_strategy = 1 or 2
```

as a normal preset combination.

If such a combination is manually entered, it may remain Custom if existing behavior allows it, but presets should not generate it.

---

# 6. B Pyramid / B reference strategy

When:

```text
bf > 0
```

B-reference behavior becomes relevant.

The current NAS runtime demonstrated:

```text
-bf 3
```

producing:

```text
GopRefDist: 4
BRefType: pyramid
```

Therefore for Balanced / Maximum Compression:

```text
bf > 0
→ B Pyramid may be enabled if capability allows
```

Do not confuse B Pyramid with GPB.

They are different concepts.

---

# 7. Adaptive I

Adaptive I controls whether QSV can dynamically convert appropriate P/B pictures into I pictures.

It does NOT mean:

```text
Adaptive I OFF
→ no I frames
```

I frames still exist because GOP/keyframe structure requires them.

Recommended interpretation:

```text
Adaptive I
→ allow QSV to adapt I-frame placement
```

Good use cases:

```text
scene changes
anime cuts
film cuts
content with variable scene structure
```

For general VOD:

```text
Adaptive I = ON
```

is a reasonable Balanced starting point when the worker capability confirms support.

---

# 8. Adaptive B

Adaptive B controls whether QSV can dynamically convert B pictures into P-like/reference behavior where appropriate.

It does NOT mean:

```text
Adaptive B OFF
→ no B frames
```

and:

```text
Adaptive B ON
→ enable B frames
```

are both incorrect interpretations.

Adaptive B is useful when:

```text
bf > 0
```

because regular B positions actually exist.

Therefore:

```text
bf = 0
→ Adaptive B generally OFF / not useful

bf > 0
→ Adaptive B may be ON if supported
```

This should guide preset generation.

---

# 9. GPB discovery — critical behavior

The current NAS worker uses:

```text
FFmpeg 8.1.2
--enable-libvpl
--disable-libmfx
```

and links against:

```text
/opt/intel/lib/x86_64-linux-gnu/libvpl.so.2
```

Therefore this is not legacy MediaSDK-only behavior.

The current worker uses oneVPL + Intel iHD driver.

FFmpeg exposes:

```text
-gpb
```

with:

```text
1 = GPB
0 = regular P
default = true
```

However, runtime testing demonstrated that:

```text
-gpb 0
```

and:

```text
-gpb false
```

both remain effectively:

```text
GPB: ON
```

on this worker.

This was tested for:

```text
HEVC Main
HEVC Main10
```

and both behave the same way.

Therefore current worker capability should effectively be:

```text
canDisableGPB = false
```

for both tested profiles.

Do NOT expose a user-facing GPB ON/OFF switch as if OFF works.

Instead show effective/runtime state.

Example:

```text
GPB
Effective: ON
Controlled by QSV runtime
```

---

# 10. Important ffprobe limitation

For HEVC QSV with GPB active:

```text
ffprobe frame=pict_type
```

may report GPB pictures as:

```text
B
```

Therefore:

```text
P = 0
B ≈ 100%
```

does NOT automatically mean:

```text
the stream contains almost only conventional bidirectional B frames
```

The effective QSV state must be used to interpret the raw frame counts.

---

# 11. QSV frame interpretation modes

Profile Lab should have at least these semantic modes.

## Generic

Use when:

```text
encoder != hevc_qsv
```

or GPB state is unknown.

Traditional I/P/B interpretation applies.

---

## QSV GPB

Use when:

```text
encoder = hevc_qsv
GPB = ON
GopRefDist = 1
```

Interpretation:

```text
no regular B-frame positions
frames reported as B may be GPB/P-like pictures
```

In this mode:

```text
P = 0
B ≈ 100%
long B-run
```

must NOT automatically generate conventional B-frame warnings.

---

## QSV Mixed B/GPB

Use when:

```text
encoder = hevc_qsv
GPB = ON
GopRefDist > 1
```

In this mode there may be:

```text
regular B frames
reference B frames / B pyramid
GPB pictures
```

and ffprobe cannot reliably separate all of them using `pict_type`.

Therefore use wording:

```text
Reported B / GPB
```

instead of claiming:

```text
all frames are GPB
```

or:

```text
all frames are conventional B
```

---

# 12. Arbegas example — mixed B/GPB

Real asset:

```text
/media/raw/anime/ARBEGAS/...
```

Source analysis:

```text
Frames sampled: 2400

I: 57
P: 2343
B: 0

B share: 0%
Longest B run: 0

Average GOP: 75.4
```

Profile command included approximately:

```text
hevc_qsv
-global_quality 29
-profile:v main10
-g 75
-bf 3
-preset slow
-pix_fmt p010le
-adaptive_i 1
-adaptive_b 1
-p_strategy 1
```

The effective runtime showed:

```text
GopPicSize: 75
GopRefDist: 4
TargetUsage: 4
RateControlMethod: ICQ
AdaptiveI: ON
AdaptiveB: ON
BRefType: pyramid
PRefType: simple
GPB: ON
```

Output analysis:

```text
Frames sampled: 2399

I: 35
P: 0
Reported B / GPB: 2364

Reported B / GPB share: 98.5%
Longest reported B / GPB run: 74

Average GOP: 75.0
```

Important interpretation:

```text
Source GOP: 75.4
Requested GOP: 75
Actual GOP: 75.0
```

The GOP is being respected extremely well.

Because:

```text
GopRefDist = 4
GPB = ON
BRefType = pyramid
```

this is an advanced mixed QSV B/GPB reference structure.

Do NOT call this:

```text
74 conventional B frames in a row
```

Do NOT warn simply because:

```text
P = 0
B reported ≈ 99%
```

Recommended interpretation:

```text
QSV advanced B/GPB reference structure.

The encoder is using GPB together with a B-reference pyramid.
ffprobe reports these pictures as B and cannot reliably separate
conventional B pictures from GPB using pict_type alone.

The measured GOP closely matches the requested/source structure.
```

Verdict:

```text
Healthy / expected QSV behavior
```

unless other validation fails.

---

# 13. Rayearth example — GPB structure

Real asset:

```text
/media/raw/anime/Rayearth/01-El Momento De Decisión.mkv
```

Source:

```text
Frames sampled: 2399

I: 39
P: 813
B: 1547

B share: 64.5%
Longest B run: 3

Average GOP: 71.7
```

Command:

```text
hevc_qsv
-global_quality 31
-profile:v main
-g 72
-bf 0
-preset slow
-pix_fmt nv12
-p_strategy 2
```

Effective runtime:

```text
GopPicSize: 72
GopRefDist: 1
TargetUsage: 4
RateControlMethod: ICQ
AdaptiveI: unknown
AdaptiveB: unknown
BRefType: off
PRefType: pyramid
GPB: ON
```

Output:

```text
Frames sampled: 2398

I: 35
P: 0
Reported B / GPB: 2363

Reported B / GPB share: 98.5%
Longest reported B / GPB run: 71

Average GOP: 72.0
```

Important:

```text
Source GOP: 71.7
Requested GOP: 72
Actual GOP: 72.0
```

This is an excellent match.

Because:

```text
GopRefDist = 1
BRefType = off
GPB = ON
```

the reported B frames should primarily be interpreted as QSV GPB/P-like reference pictures rather than conventional B-frame chains.

The fact that:

```text
Longest reported run = 71
GOP = 72
```

is consistent with:

```text
I + 71 GPB
```

rather than:

```text
I + 71 conventional B
```

Recommended verdict:

```text
Healthy / expected QSV GPB structure
```

---

# 14. Arbegas vs Rayearth — why both are important

These two assets demonstrate different source structures.

## Arbegas source

```text
essentially I/P only
B = 0
```

## Rayearth source

```text
normal I/P/B source
B share ≈ 64.5%
B-run = 3
```

Yet both QSV outputs may show:

```text
P = 0
reported B/GPB ≈ 98–99%
```

This demonstrates that:

> QSV output frame-type counts must not be interpreted by direct comparison against the source without considering effective GPB/GopRefDist state.

These should become regression fixtures.

---

# 15. Profile Lab display

Keep raw data visible.

For example:

```text
Source

I frames
P frames
B frames
Average GOP
```

For QSV output:

```text
I frames
P frames
Reported B / GPB frames
Reported B / GPB share
Longest reported B / GPB run
Average GOP
```

Then add:

```text
Effective QSV Structure
```

with:

```text
GOP Size
Reference Distance
Adaptive I
Adaptive B
B Reference Type
P Reference Type
GPB
Rate Control
Target Usage
```

Example:

```text
Effective QSV Structure

GOP size:           75
Reference distance: 4
Adaptive I:         ON
Adaptive B:         ON
B references:       Pyramid
P references:       Simple
GPB:                ON
Rate control:       ICQ
```

---

# 16. Interpretation wording

Do not repeatedly show:

```text
Expected QSV GPB structure
```

for every row without context.

Prefer one overall explanation plus row-specific neutral wording.

For example:

```text
P-frames:
Reported as 0 under QSV GPB structure

Reported B / GPB:
QSV-specific frame classification

Average GOP:
Within expected tolerance
```

Then below:

```text
QSV advanced frame structure detected.

GPB is active and ffprobe may classify GPB pictures as B.
The raw B count therefore does not represent conventional B frames only.
```

---

# 17. GOP recommendation

GOP should be recommended by asset analysis.

Do not blindly copy exact source GOP.

Use approximately:

```text
target GOP =
FPS × desired keyframe seconds
```

while considering source GOP as a signal.

Examples:

```text
Rayearth:
source ~71.7
recommended ~72

Arbegas:
source ~75.4
recommended ~75
```

These examples show that preserving roughly the source temporal interval can work very well.

Recommendation should use:

```text
FPS
source average GOP
content type
motion
compatibility target
encoder
```

---

# 18. Frame sampling

Do not return to fixed:

```text
500 frames
```

sampling.

Sampling should be time-based.

Preferred:

```text
5 windows
20 seconds each
distributed through the asset
```

Suggested positions:

```text
10%
30%
50%
70%
90%
```

or equivalent non-edge positions.

Total:

```text
~100 seconds sampled
```

This avoids FPS-dependent sample duration.

Do not concatenate B-runs or GOPs across separate windows.

Each window must be analyzed independently before aggregation.

---

# 19. Recommendation confidence

Frame-structure recommendation should consider consistency between the five windows.

Example:

```text
High confidence
→ windows show similar structure

Medium
→ moderate variation

Low
→ strong GOP/cadence/frame-structure differences
```

Do not base confidence only on total frame count.

---

# 20. Recommendation vs validation

These are separate stages.

```text
5-window source analysis
        ↓
recommendation
```

Then:

```text
preview encode
        ↓
5-window output analysis
        ↓
validation
```

Do not mark something `Recommended` simply because source analysis produced values.

Preferred concepts:

```text
Recommended
Validated
```

A recommendation becomes validated only after actual output behavior is inspected.

---

# 21. Requested vs Effective vs Actual

Keep these three states distinct.

Example Arbegas:

```text
Recommended
GOP 75
B Frames 3
Adaptive I ON
Adaptive B ON

Requested
-g 75
-bf 3
-adaptive_i 1
-adaptive_b 1

Effective QSV
GopPicSize 75
GopRefDist 4
AdaptiveI ON
AdaptiveB ON
BRefType pyramid
GPB ON

Actual Output
Average GOP 75.0
```

This is the preferred architecture.

Do NOT infer effective state from raw ffprobe counts.

---

# 22. Capability-driven behavior

Preserve the existing architecture:

```text
FFmpeg option exists
        ↓
runtime probe
        ↓
effective behavior verified
        ↓
worker capability
        ↓
UI + command builder
```

Examples already confirmed:

```text
Adaptive I supported
Adaptive B supported
P Strategy supported
B Pyramid supported

GPB disable ineffective
```

Do not revert these findings.

---

# 23. GPB capability

Current NAS:

```text
HEVC Main:
GPB effective = ON
GPB disable effective = false

HEVC Main10:
GPB effective = ON
GPB disable effective = false
```

Do not assume this applies universally to every future Intel worker.

Probe each worker independently.

---

# 24. Advanced fields must remain visible

The purpose of presets is not to hide complexity.

Desired UI:

```text
Frame Structure Mode
[ Balanced ▼ ]

GOP                 75
B Frames              3
Adaptive I           ON
Adaptive B           ON
P Strategy           N/A
B Pyramid            ON
GPB              ON (runtime)
```

The user can see what Balanced actually means.

A technical user can modify any field.

After modification:

```text
Frame Structure Mode
→ Custom
```

---

# 25. Capability interaction with presets

Preset values are requested intent.

They still must pass through capability resolution.

Example:

```text
Balanced wants:
Adaptive B = ON
```

but worker says:

```text
Adaptive B unsupported
```

Then:

```text
Requested preset:
Balanced

Effective:
Adaptive B OFF

Reason:
unsupported by active worker
```

Do not silently claim the preset was fully applied.

---

# 26. Dynamic control availability

Recommended UI behavior:

```text
B Frames == 0
→ P Strategy enabled
→ Adaptive B disabled/not relevant
→ B Pyramid disabled/not relevant

B Frames > 0
→ P Strategy disabled
→ Adaptive B available if supported
→ B Pyramid available if supported
```

Adaptive I can remain available independently when supported.

This keeps impossible/confusing combinations out of presets.

---

# 27. Do not use GPB as a user-controlled preset field

For the current NAS:

```text
-gpb 0
```

is accepted but:

```text
effective GPB = ON
```

Therefore:

```text
GPB
```

should currently be an informational/effective field rather than normal preset input.

Possible UI:

```text
GPB
ON · QSV runtime controlled
```

Future workers may make it configurable if probe confirms:

```text
GPB OFF effective
```

---

# 28. What should trigger warnings

GPB-aware interpretation must not disable legitimate warnings.

Still warn for:

```text
requested GOP greatly differs from actual GOP

requested Adaptive I but effective AdaptiveI != ON

requested Adaptive B but effective AdaptiveB != ON

requested P strategy but PRefType differs

requested bf resulting in unexpected GopRefDist

unexpected BRefType

decode errors

timestamp issues

keyframe irregularity

large quality regression
```

---

# 29. What should NOT trigger warnings by itself

For HEVC QSV when effective state explains the output:

```text
P = 0
reported B / GPB ≈ 100%
long reported B / GPB run
```

must not automatically mean:

```text
bad frame structure
```

Example Rayearth:

```text
GopRefDist = 1
GPB = ON
GOP requested 72
GOP actual 72.0
```

is healthy.

Example Arbegas:

```text
GopRefDist = 4
BRefType = pyramid
GPB = ON
GOP requested 75
GOP actual 75.0
```

is also healthy if no other validation fails.

---

# 30. Recommended initial QSV modes

Use these only as first-pass policy.

## Compatible

```text
B Frames = 0

Adaptive I = ON if supported

Adaptive B = OFF

P Strategy = Simple or Pyramid

B Pyramid = N/A

GOP = asset recommended
usually shorter/moderate
```

---

## Balanced

Preferred default:

```text
B Frames = 3

Adaptive I = ON

Adaptive B = ON

P Strategy = N/A

B Pyramid = ON / Auto

GOP = asset recommended
```

---

## Maximum Compression

```text
B Frames = 3–4

Adaptive I = ON

Adaptive B = ON

P Strategy = N/A

B Pyramid = ON

GOP = moderately longer asset recommendation
```

Do not increase GOP or B depth without limits.

---

# 31. Custom mode

Custom means:

```text
user explicitly controls advanced fields
```

Do not automatically rewrite Custom values unless capability resolution requires a downgrade.

If a downgrade occurs, show it.

---

# 32. Do not change rate-control policy here

This frame-structure work is separate from:

```text
ICQ
VBR
CBR
CQP
LA-ICQ
ExtBRC
Look Ahead
```

Existing rules remain.

In particular:

```text
ExtBRC
```

is not a GOP/B-frame fix.

Do not enable it based on frame structure.

Look Ahead is also not a generic fix for GPB.

---

# 33. Existing QSV runtime findings remain valid

Preserve:

```text
ICQ works
CQP works
VBR works
CBR works

VBR + ExtBRC Main10 works
CBR + ExtBRC Main10 works

Adaptive I works
Adaptive B works

P Strategy works

B Pyramid works

LA-ICQ / Look Ahead not validated on current worker

GPB disable ineffective
```

Do not revert capability-driven decisions.

---

# 34. Suggested regression tests

Add tests for preset behavior.

## Compatible

Expected:

```text
bf = 0
adaptiveB = false
pStrategy available
```

---

## Balanced

Expected:

```text
bf = 3
adaptiveI = true if supported
adaptiveB = true if supported
pStrategy disabled
B pyramid enabled if supported
```

---

## Manual modification

Start:

```text
Balanced
bf = 3
```

User changes:

```text
bf = 2
```

Expected:

```text
mode = Custom
```

---

# 35. Frame-analysis regression fixtures

Use both real observed examples.

## Arbegas

Source:

```text
I 57
P 2343
B 0
GOP 75.4
```

Output:

```text
I 35
P 0
reported B/GPB 2364
GOP 75.0
```

Effective:

```text
GopRefDist 4
AdaptiveI ON
AdaptiveB ON
BRefType pyramid
PRefType simple
GPB ON
```

Expected:

```text
qsv_mixed_b_gpb
healthy
```

---

## Rayearth

Source:

```text
I 39
P 813
B 1547
B-run 3
GOP 71.7
```

Output:

```text
I 35
P 0
reported B/GPB 2363
B/GPB run 71
GOP 72.0
```

Effective:

```text
GopRefDist 1
BRefType off
PRefType pyramid
GPB ON
```

Expected:

```text
qsv_gpb
healthy
```

---

# 36. Main implementation principle

Do NOT build the UI around low-level Intel terminology.

The normal user chooses:

```text
Compatible
Balanced
Maximum Compression
Custom
```

MVForge translates that into advanced QSV configuration.

But advanced users can still see and edit the underlying values.

The final UX should feel like:

```text
simple choice
        ↓
transparent advanced configuration
        ↓
effective runtime verification
        ↓
actual output validation
```

---

# 37. Final architecture

The final flow should be:

```text
Asset
   ↓
5-window source analysis
   ↓
Frame Structure Recommendation
   ↓
Frame Structure Mode
   ├── Compatible
   ├── Balanced
   ├── Maximum Compression
   └── Custom
   ↓
Advanced fields populated
   ↓
Capability resolution
   ↓
Requested QSV command
   ↓
Effective QSV runtime state
   ↓
5-window output analysis
   ↓
Encoder-aware interpretation
   ↓
Validated / Review
```

This is the intended design.

Do not revert existing QSV advanced-feature work while implementing this layer.