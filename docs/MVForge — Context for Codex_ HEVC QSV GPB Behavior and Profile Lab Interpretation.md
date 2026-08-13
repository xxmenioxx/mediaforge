# MVForge — Context for Codex: HEVC QSV GPB Behavior and Profile Lab Interpretation

## Purpose

This context documents a newly confirmed behavior of `hevc_qsv` on the current MVForge NAS worker.

Adaptive I, Adaptive B and P-pyramid support have already been implemented and tested separately.

Do NOT revisit or revert that work.

The remaining issue is how MVForge should interpret HEVC QSV frame structure when Intel GPB is active.

---

# 1. Important discovery

The current NAS worker uses:

```text
FFmpeg 8.1.2
--enable-libvpl
--disable-libmfx
```

and FFmpeg is dynamically linked against:

```text
/opt/intel/lib/x86_64-linux-gnu/libvpl.so.2
```

Therefore this is NOT a legacy MediaSDK-only path.

The active stack is based on oneVPL.

The worker also uses the Intel `iHD` media driver.

---

# 2. FFmpeg exposes GPB control

`hevc_qsv` exposes:

```text
-gpb <boolean>
```

with the documented meaning:

```text
1 = GPB / generalized P/B frame
0 = regular P frame
default = true
```

The encoder also exposes:

```text
-p_strategy
-adaptive_i
-adaptive_b
```

Those features have already been handled separately.

GPB must now be treated as its own QSV concept.

---

# 3. NAS runtime tests

Codex already ran synthetic FFmpeg tests inside:

```text
mediaforge-backend
```

using temporary generated video.

No assets or database entries were modified.

The important tests are summarized below.

---

# 4. Baseline with `-bf 0`

Testing:

```text
-bf 0
```

did NOT result in conventional P-frames.

The observed bitstream was approximately:

```text
149 B
1 I
```

Adding any of the following did not materially change that result:

```text
adaptive_i
adaptive_b
p_strategy 1
p_strategy 2
adaptive I/B + simple P strategy
adaptive I/B + pyramid P strategy
```

The output continued to appear as almost entirely B frames according to:

```text
ffprobe frame=pict_type
```

This initially looked suspicious.

---

# 5. GPB was then investigated

FFmpeg confirmed that `hevc_qsv` exposes:

```text
-gpb
```

The test was repeated using:

```text
-bf 0
-gpb 0
```

with Main10.

The resulting frame counts were:

```text
148 B
2 I
0 P
```

A verbose QSV log showed:

```text
GopPicSize: 75
GopRefDist: 1
IdrInterval: 1
BRefType: off
GPB: ON
```

This is important.

MVForge requested:

```text
-gpb 0
```

but the effective QSV runtime still reported:

```text
GPB: ON
```

---

# 6. Explicit `-gpb false` was also tested

To rule out boolean parsing differences, the test was repeated using:

```text
-gpb false
```

The QSV runtime still reported:

```text
GopPicSize: 75
GopRefDist: 1
BRefType: off
GPB: ON
```

Therefore both:

```text
-gpb 0
```

and:

```text
-gpb false
```

were ineffective on this worker/runtime.

---

# 7. Main 8-bit and Main10 were compared

The same test was executed for both profiles.

## Main 8-bit

Requested:

```text
-profile:v main
-pix_fmt nv12
-g 75
-bf 0
-gpb 0
```

Effective QSV settings:

```text
GopPicSize: 75
GopRefDist: 1
RateControlMethod: ICQ
BRefType: off
GPB: ON
```

ffprobe:

```text
148 B
2 I
```

Output stream:

```text
HEVC Main
yuv420p
```

## Main10

Requested:

```text
-profile:v main10
-pix_fmt p010le
-g 75
-bf 0
-gpb 0
```

Effective QSV settings:

```text
GopPicSize: 75
GopRefDist: 1
RateControlMethod: ICQ
BRefType: off
GPB: ON
```

ffprobe:

```text
148 B
2 I
```

Output stream:

```text
HEVC Main 10
yuv420p10le
```

Conclusion:

```text
GPB disable is ineffective on this worker
for both Main and Main10.
```

---

# 8. Important interpretation of `GopRefDist = 1`

The observed runtime reports:

```text
GopRefDist: 1
BRefType: off
GPB: ON
```

This is critical for interpreting the output.

With this structure, the frames reported by ffprobe as:

```text
B
```

must NOT automatically be interpreted as conventional bidirectional B-frames.

For HEVC QSV with GPB enabled, these frames are generalized P/B pictures.

Conceptually, the output is closer to:

```text
I GPB GPB GPB GPB ...
```

than:

```text
I B B B B ...
```

in the traditional bidirectional B-frame sense.

---

# 9. Real asset example: Arbegas

A real Arbegas asset produced this frame analysis.

Source:

```text
Frames sampled: 2400
I: 57
P: 2343
B: 0
B share: 0%
B-run max: 0
Average GOP: 75.4
```

QSV output:

```text
Frames sampled: 2399
I: 35
P: 0
B: 2364
B share: 98.5%
B-run max: 74
Average GOP: 75.0
```

The effective command included:

```text
hevc_qsv
-global_quality 29
-preset slow
-profile:v main10
-pix_fmt p010le
-g 75
-bf 0
```

The important observation is:

```text
Requested GOP: 75
Actual average GOP: 75.0
```

The GOP was respected almost exactly.

The apparent:

```text
P → B
```

transformation should therefore NOT be interpreted automatically as:

```text
P frames disappeared and were replaced by 74 conventional B frames.
```

The runtime evidence instead strongly supports:

```text
P-like structure
→ GPB structure
```

where ffprobe reports the GPB pictures as `B`.

---

# 10. Existing Lab interpretation is misleading

Current/previous Profile Lab analysis may report warnings such as:

```text
P-frames disappeared
B-frame share increased to ~99%
Longest B-frame run increased dramatically
```

For generic codecs those can be meaningful warnings.

For `hevc_qsv` when the effective runtime shows:

```text
GopRefDist = 1
GPB = ON
```

those warnings are misleading.

The Lab currently does not distinguish:

```text
regular B frame
```

from:

```text
QSV GPB frame reported as B by ffprobe
```

This needs to be fixed.

---

# 11. Required conceptual change

Do NOT globally change the generic frame analyzer.

Instead, add an encoder-aware interpretation layer.

The raw frame analysis should remain raw:

```text
I count
P count
B count
B ratio
B-run
GOP
```

Then apply encoder-specific interpretation.

Conceptually:

```text
Raw frame metrics
       ↓
Encoder context
       ↓
Effective QSV structure
       ↓
Interpretation / verdict
```

For example:

```text
encoder = hevc_qsv
GopRefDist = 1
GPB = ON
```

should affect how:

```text
P = 0
B ≈ 100%
B-run ≈ GOP-1
```

is interpreted.

---

# 12. Do not throw away raw metrics

Keep raw ffprobe values.

For diagnostics they are still useful.

Example:

```json
{
  "iFrames": 35,
  "pFrames": 0,
  "bFrames": 2364,
  "bRatio": 0.985,
  "maxBRun": 74,
  "averageGop": 75.0
}
```

But add effective encoder context such as:

```json
{
  "encoder": "hevc_qsv",
  "gopRefDist": 1,
  "bRefType": "off",
  "gpb": true
}
```

Then interpretation can become:

```text
Raw reported B frames are consistent with QSV GPB behavior.
```

---

# 13. Suggested result model

Do not over-engineer this, but conceptually the analysis needs room for:

```go
type FrameStructureContext struct {
	Encoder    string `json:"encoder,omitempty"`

	GopRefDist *int   `json:"gopRefDist,omitempty"`
	GPB        *bool  `json:"gpb,omitempty"`
	BRefType   string `json:"bRefType,omitempty"`
}
```

or equivalent fields in the existing inspection/result model.

A derived interpretation could contain:

```go
type FrameStructureInterpretation struct {
	Mode       string   `json:"mode"`
	Severity   string   `json:"severity"`
	Summary    string   `json:"summary"`
	Reasons    []string `json:"reasons"`
}
```

Possible mode:

```text
generic
qsv_gpb
```

Use existing project conventions rather than forcing these exact structs if there is already a better place for the data.

---

# 14. Detection rule for QSV GPB interpretation

A conservative initial rule:

```text
if encoder == hevc_qsv
and effective GPB == ON
and GopRefDist == 1
then
    interpret reported B frames as GPB-capable structure
```

If `BRefType` is also known:

```text
BRefType == off
```

that further supports the interpretation.

Do NOT infer GPB solely from:

```text
P == 0
B > 95%
```

because that could hide a genuine abnormal B structure on another encoder/runtime.

Use effective encoder information.

---

# 15. Suggested Profile Lab wording

For QSV GPB mode, instead of:

```text
Review · P-frames disappeared
Review · large B-frame increase
```

display something closer to:

```text
QSV GPB structure detected
```

and:

```text
The HEVC QSV encoder is using Generalized P/B frames.
ffprobe reports these pictures as B frames, but with
GopRefDist=1 they should not be interpreted as a conventional
long bidirectional B-frame chain.
```

For the Arbegas example:

```text
Source:
I 57
P 2343
B 0
Average GOP 75.4

Output:
I 35
P 0
Reported B/GPB 2364
Average GOP 75.0

Effective QSV:
GopRefDist 1
BRefType off
GPB ON

Verdict:
Expected QSV GPB structure.
Output GOP closely matches the requested/source structure.
```

---

# 16. UI terminology

For QSV GPB output, consider changing labels dynamically.

Instead of:

```text
B-frames
```

show:

```text
B / GPB reported frames
```

or:

```text
Reported B/GPB
```

Instead of:

```text
Longest B-frame run
```

show:

```text
Longest reported B/GPB run
```

Only do this in the QSV GPB interpretation context.

Do NOT globally rename B-frame metrics for other encoders.

---

# 17. Severity behavior

A QSV GPB pattern such as:

```text
P = 0
B/GPB ≈ 99%
B-run ≈ GOP - 1
GopRefDist = 1
GPB = ON
```

should NOT automatically generate a warning.

It may be informational/healthy if:

```text
actual GOP is close to requested GOP
no decode errors
no timestamp problems
no unexpected keyframe behavior
no visual-quality regression
```

For example:

```text
requested GOP = 75
actual GOP = 75.0
```

should be considered a strong positive signal.

---

# 18. What should still produce warnings

Do NOT suppress all QSV frame-structure warnings.

Warnings should remain possible for things such as:

```text
requested GOP differs dramatically from actual GOP
unexpected GopRefDist
unexpected BRefType
GPB state differs from expected capability
decode errors
invalid timestamps
very irregular keyframe intervals
encoder configuration not matching effective configuration
```

The change is specifically:

> `P=0 + high B count` is not enough to call HEVC QSV GPB output abnormal.

---

# 19. GPB capability result for current worker

For the current NAS worker, the tested behavior is approximately:

```text
HEVC QSV Main:
  GPB available: yes
  GPB default/effective: ON
  GPB disable requested: accepted syntactically
  GPB disable effective: no

HEVC QSV Main10:
  GPB available: yes
  GPB default/effective: ON
  GPB disable requested: accepted syntactically
  GPB disable effective: no
```

This should be treated similarly to other capability-driven QSV behavior.

Do NOT assume:

```text
because FFmpeg exposes -gpb
→ GPB OFF is supported
```

The runtime probe is the source of truth.

---

# 20. Capability architecture

Preserve the existing MVForge principle:

```text
FFmpeg exposes option
       ↓
runtime probe
       ↓
verify effective behavior
       ↓
store capability
       ↓
UI + Lab + command logic consume capability
```

For GPB, the important capability is not merely:

```text
supportsGPB
```

because GPB is already active.

The interesting capability is:

```text
canDisableGPB
```

or equivalent.

For the current worker:

```text
canDisableGPB = false
```

for both tested Main and Main10 combinations.

Keep it contextual if the existing capability model supports profile-specific results.

---

# 21. Do not add a user-facing GPB switch yet

Do NOT immediately add:

```text
GPB ON/OFF
```

as a normal Profile Lab setting.

On the current worker, OFF has already been demonstrated to be ineffective.

A user-facing switch that appears controllable but is ignored by the effective runtime would be misleading.

First model the capability and effective state correctly.

A future worker where:

```text
-gpb 0
→ GPB: OFF
```

could expose the control.

---

# 22. Do not modify Adaptive/P-pyramid work

Adaptive I, Adaptive B and P-pyramid have already been implemented/tested.

Do NOT use this GPB discovery as justification to remove or revert them.

They are different controls.

In particular:

```text
Adaptive B
```

does NOT mean:

```text
enable/disable GPB
```

and:

```text
P strategy
```

does NOT guarantee conventional P frames when GPB remains active.

Keep the existing features capability-driven.

---

# 23. Do not treat `-bf 0` as “P-only”

This is another important correction.

For generic reasoning it is tempting to interpret:

```text
-bf 0
```

as:

```text
I/P structure only
```

That assumption is wrong for the observed HEVC QSV runtime.

On this worker:

```text
-bf 0
```

results in:

```text
GopRefDist = 1
GPB = ON
```

and ffprobe reports GPB pictures as `B`.

Therefore MVForge must distinguish:

```text
no regular B-frame distance
```

from:

```text
no B pict_type reported
```

Those are not equivalent for HEVC QSV GPB.

---

# 24. Requested vs effective state

GPB should follow MVForge's existing requested/effective model.

Example:

```text
Requested:
GPB OFF

Effective:
GPB ON

Worker capability:
GPB disable ineffective
```

This is much better than pretending:

```text
-gpb 0
```

worked because the command accepted it.

If the user did not explicitly request GPB OFF, simply record:

```text
Effective GPB: ON
```

as runtime information.

---

# 25. Recommended implementation order

Keep this change small and focused.

### Step 1

Capture/retain effective QSV GPB information from the probe/log if the existing QSV inspection infrastructure already parses:

```text
GopPicSize
GopRefDist
AdaptiveI
AdaptiveB
BRefType
RateControlMethod
```

Add:

```text
GPB
```

to the same parser/result.

Do not build a second parser if one already exists.

### Step 2

Record capability/result for GPB disable.

Current tested worker should resolve to:

```text
canDisableGPB = false
```

for Main and Main10.

### Step 3

Pass effective GPB/GopRefDist context into frame-structure interpretation.

### Step 4

Adjust Profile Lab warning/severity text for:

```text
hevc_qsv + GPB ON + GopRefDist 1
```

### Step 5

Add tests using the observed Arbegas pattern.

---

# 26. Tests to add

## QSV GPB interpretation

Input:

```text
encoder = hevc_qsv
GPB = ON
GopRefDist = 1

I = 35
P = 0
B = 2364
B ratio = 98.5%
max B-run = 74
average GOP = 75
requested GOP = 75
```

Expected:

```text
no generic "P-frames disappeared" warning
no generic "extreme B-run" warning
mode = qsv_gpb
severity = info/healthy
GOP considered within tolerance
```

---

## Generic HEVC/non-QSV comparison

Same frame metrics:

```text
P = 0
B ≈ 99%
B-run = 74
```

but:

```text
encoder != hevc_qsv
```

Expected:

```text
generic warning remains
```

This prevents accidentally weakening analysis for other encoders.

---

## QSV without GPB context

If:

```text
encoder = hevc_qsv
GPB unknown
```

do NOT silently assume GPB.

Prefer conservative generic interpretation or an explicit:

```text
GPB state unknown
```

depending on existing UI patterns.

---

## QSV GPB with bad GOP

Example:

```text
requested GOP = 75
actual average GOP = 240
GPB = ON
GopRefDist = 1
```

Expected:

```text
GPB-specific P/B warning suppressed
BUT
GOP deviation warning remains
```

---

# 27. Real-world test fixture

Use the Arbegas observation as the primary regression case.

Source:

```text
I = 57
P = 2343
B = 0
B ratio = 0%
max B-run = 0
average GOP = 75.4
```

Output:

```text
I = 35
P = 0
B = 2364
B ratio = 98.5%
max B-run = 74
average GOP = 75.0
```

Effective QSV:

```text
GopPicSize = 75
GopRefDist = 1
BRefType = off
GPB = ON
```

Desired conclusion:

```text
The output uses the expected HEVC QSV GPB structure.
The apparent disappearance of P frames is explained by GPB behavior.
The GOP remains closely aligned with the source/requested structure.
```

Do not classify this case as a pathological 74-frame conventional B-run.

---

# 28. Current technical conclusion

The confirmed behavior is:

```text
FFmpeg option exists:
-gpb

Requested:
-gpb 0 / false

Current NAS effective runtime:
GPB ON

Main:
same result

Main10:
same result

GopRefDist:
1

BRefType:
off
```

Therefore:

> GPB is effectively mandatory/unavoidable on the tested worker configuration, even though the FFmpeg option is exposed.

Important wording:

Do NOT generalize this to:

```text
all Intel QSV hardware always forces GPB
```

The conclusion applies to:

```text
this tested worker/runtime combination
```

Future Intel workers must be probed independently.

---

# 29. Why this matters

Without this change, Profile Lab can incorrectly tell the user that a healthy QSV GPB encode has:

```text
P-frames disappeared
extreme B-frame usage
abnormal B-frame run
```

That creates false alarms and can lead MVForge to recommend unnecessary encoder changes.

The goal is not to hide unusual structures.

The goal is to interpret the actual structure correctly.

---

# 30. Core principle

For HEVC QSV:

```text
ffprobe pict_type=B
```

does NOT by itself prove:

```text
conventional bidirectional B-frame
```

when the effective encoder state is:

```text
GopRefDist = 1
GPB = ON
```

MVForge must use:

```text
raw bitstream metrics
+
effective encoder state
+
worker capability
```

to generate the final verdict.

For the current NAS worker, the Arbegas output should be interpreted as a normal QSV GPB structure with an accurately respected GOP, not as a pathological disappearance of P-frames.