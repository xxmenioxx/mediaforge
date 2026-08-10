# MVForge QSV Advanced Features — Context for Codex

## Important: Do not revert the current QSV capability-driven behavior

The recent changes around `LA_ICQ`, `Look Ahead`, `Extended BRC`, `Adaptive I`, and `Adaptive B` were intentional.

They were made after testing the **actual Intel QSV runtime on the MVForge NAS worker**.

The core rule is:

> The presence of an FFmpeg option does NOT mean the active Intel/QSV runtime supports that option or that FFmpeg actually uses the requested mode.

MVForge must therefore be **capability-driven**, not merely flag-driven.

Do not restore previous logic that assumes these options are available because FFmpeg lists them.

---

# 1. Runtime environment tested

The main QSV worker is the MVForge NAS.

Relevant environment:

- Intel Alder Lake-P iGPU
- `/dev/dri/renderD128`
- Intel Media Driver
- oneVPL / libvpl
- FFmpeg QSV encoders
- `hevc_qsv` is the main HEVC hardware encoder

MVForge performs real encoder probes to determine which QSV combinations are actually usable.

These probes are the authority for the UI and effective encoding configuration.

---

# 2. Verified QSV combinations

The following combinations were successfully validated on the active NAS worker:

### ICQ

- QSV ICQ Main ✅
- QSV ICQ Main10 ✅

### CQP

- QSV CQP Main ✅
- QSV CQP Main10 ✅

### VBR

- QSV VBR Main ✅
- QSV VBR Main10 ✅
- QSV VBR + Extended BRC Main10 ✅

### CBR

- QSV CBR Main ✅
- QSV CBR Main10 ✅
- QSV CBR + Extended BRC Main10 ✅

### Adaptive frame decisions

- Adaptive I Main10 ✅
- Adaptive B Main10 ✅

These capabilities may vary by worker.

Do NOT globally assume they exist.

---

# 3. LA-ICQ behavior

This was one of the most important discoveries.

FFmpeg exposes `LA_ICQ` / LA-ICQ-related QSV configuration, but on the current worker the requested mode did not behave as true LA-ICQ.

During the runtime probe, MVForge requested LA-ICQ but the encoder effectively reported/used:

```text
ICQ
```

rather than true:

```text
LA_ICQ
```

Therefore:

```text
QSV LA-ICQ Main10
```

is currently considered:

```text
NOT VALIDATED / NOT SUPPORTED
```

on this worker.

This is intentional.

Do NOT change the capability check to:

```text
FFmpeg has LA_ICQ option
→ LA_ICQ supported
```

That would reintroduce the bug.

The required behavior is:

```text
request LA_ICQ
        ↓
run real probe
        ↓
verify effective encoder behavior
        ↓
if encoder uses ICQ instead
        ↓
LA_ICQ capability = false
```

The UI should only expose/use LA-ICQ when the active worker probe has positively confirmed Look Ahead / LA-ICQ behavior.

A user-facing explanation we previously adopted is essentially:

> LA-ICQ falls back to ICQ unless the active worker probe confirms Look Ahead.

---

# 4. Look Ahead

Look Ahead is also runtime-dependent.

The following combinations were tested and were NOT validated on the current NAS worker:

```text
VBR + Look Ahead Main10 ❌
CBR + Look Ahead Main10 ❌
LA-ICQ Main10 ❌
VBR Advanced combination ❌
```

Therefore Look Ahead must not be enabled merely because:

```text
-look_ahead_depth
```

or another Look Ahead-related FFmpeg option exists.

The correct decision is:

```text
active worker capability
        +
rate-control mode
        +
profile / bit depth
        ↓
is Look Ahead actually supported?
```

If not supported:

- UI should disable/hide the option where appropriate.
- command builder should not emit the unsupported combination.
- effective configuration should record any downgrade.
- diagnostics should explain the reason.

---

# 5. Extended BRC

Extended BRC is NOT a generic quality switch.

It is associated with bitrate-based QSV rate control.

Our intended policy is:

```text
ICQ      → Extended BRC OFF
LA-ICQ   → Extended BRC OFF
CQP      → Extended BRC OFF

VBR      → Extended BRC may be enabled
CBR      → Extended BRC may be enabled
```

But even with VBR/CBR:

> Extended BRC may only be used when the active worker capability probe confirms the corresponding combination.

On the current worker we positively validated:

```text
VBR + Extended BRC Main10 ✅
CBR + Extended BRC Main10 ✅
```

Do NOT infer support for untested profile/rate-control combinations.

For example, do not automatically assume:

```text
VBR Main 8-bit + ExtBRC
```

works simply because:

```text
VBR Main10 + ExtBRC
```

worked.

MVForge should use the actual capability matrix.

---

# 6. Quality preset policy for Extended BRC

We intentionally decided that advanced QSV options should not automatically be enabled on every quality preset.

For presets below:

```text
High Quality
```

the intended policy is to keep these advanced features disabled by default:

```text
Extended BRC
Adaptive I
Adaptive B
LA-ICQ / Look Ahead
```

The goal is predictable and stable encoder behavior.

At higher-quality presets these features may become available, but only if the active worker capability allows them.

Therefore the effective decision is roughly:

```text
Preset allows advanced feature
          AND
Rate control allows feature
          AND
Worker probe validates feature
          ↓
Feature may be enabled
```

Not:

```text
Preset == High Quality
→ blindly enable everything
```

---

# 7. Adaptive I

`Adaptive I` allows QSV to make adaptive I-frame decisions.

Current worker:

```text
Adaptive I Main10 ✅
```

This means the runtime supports the tested Adaptive-I combination.

However it is still optional.

The UI state must control whether it is requested.

If the user disables Adaptive I:

```text
qsvAdaptiveI = false
```

the effective preview/encoding command must NOT continue requesting:

```text
-adaptive_i 1
```

The analysis must distinguish between:

```text
Adaptive I configuration
```

and:

```text
actual I-frame structure found in the encoded video
```

Those are not the same thing.

A video can contain I-frames even when Adaptive I is disabled.

---

# 8. Adaptive B

The same rule applies to `Adaptive B`.

Current worker:

```text
Adaptive B Main10 ✅
```

Adaptive B allows QSV to adapt B/P frame decisions.

It does NOT mean:

```text
Adaptive B OFF
→ no B-frames
```

That interpretation is incorrect.

The encoder may continue producing B-frames when Adaptive B is disabled.

If the user disables Adaptive B:

```text
qsvAdaptiveB = false
```

the command should not request:

```text
-adaptive_b 1
```

However the resulting encoded stream can still contain B-frames because GOP/B-frame structure is controlled by additional encoder behavior/options.

This distinction is particularly important in Profile Lab.

---

# 9. Recent Profile Lab observation

We recently observed an encoded output approximately like:

```text
Source:
I 7
P 203
B 290
B ratio 58.0%
B-run max 3
Average GOP 82.7

Output:
I 2
P 0
B 477
B ratio 99.6%
B-run max 247
Average GOP 248
```

The output was generated using QSV.

This is an unusual/extreme B-frame structure and should generate an output-analysis warning.

However:

> That warning must NOT automatically be interpreted as proof that Adaptive B is enabled.

There are two separate concepts.

### Configuration warning

Example:

```text
Adaptive B requested but unsupported.
```

This warning depends on:

```text
qsvAdaptiveB
+
worker capability
```

### Output analysis warning

Example:

```text
Output contains 99.6% B-frames, no detected P-frames,
and a maximum B-run of 247 frames.
```

This warning depends on the actual encoded stream.

It can remain present even if:

```text
Adaptive B = OFF
```

if QSV still generated that frame structure.

Do not remove valid output-analysis warnings simply because the Adaptive B switch is disabled.

Instead verify that Profile Lab correctly generated a NEW preview using a command where:

```text
-adaptive_b 1
```

is absent when disabled.

The same applies to Adaptive I.

---

# 10. Capability resolver

The frontend has/should have centralized QSV capability logic rather than duplicating rules throughout pages.

The intended concept is similar to:

```ts
resolveQSVFeatures(capability, {
  main10,
  rateControl,
})
```

Consumers include areas such as:

```text
Assets
Profile Lab
Profiles
Preview
```

The resolver should determine whether features are available for the effective combination.

Relevant inputs include:

```text
worker capabilities
Main vs Main10
rate control
quality preset
```

The UI should not independently guess whether:

```text
LA-ICQ
Look Ahead
Extended BRC
Adaptive I
Adaptive B
```

are supported.

---

# 11. Requested vs Effective configuration

Another important architectural rule is that MVForge distinguishes:

```text
Requested configuration
```

from:

```text
Effective configuration
```

For example:

```text
Requested:
LA_ICQ

Worker capability:
LA_ICQ unavailable

Effective:
ICQ
```

or preferably the unsupported request can be prevented entirely.

If a downgrade occurs, MVForge should record it.

The same principle applies to:

```text
profile
pixel format
rate control
Look Ahead
Extended BRC
Adaptive I
Adaptive B
```

This is important for diagnostics and auditability.

---

# 12. Do not regress to FFmpeg-option detection

A previous/simple implementation could effectively behave like:

```text
FFmpeg lists -adaptive_b
→ expose Adaptive B

FFmpeg lists -extbrc
→ expose Extended BRC

FFmpeg lists LA_ICQ
→ expose LA_ICQ
```

That approach is incorrect for MVForge.

The actual runtime demonstrated that an option may:

- exist in FFmpeg,
- be accepted syntactically,
- but fail at runtime,
- fall back to another mode,
- or be incompatible with the selected profile/rate-control combination.

The correct architecture is:

```text
FFmpeg option exists
        ↓
real encoder probe
        ↓
runtime actually accepts combination
        ↓
verify effective behavior where possible
        ↓
store capability
        ↓
UI + command builder consume capability
```

---

# 13. Current intended matrix

For the current NAS worker, treat the matrix approximately as:

```text
QSV ICQ Main                 ✅
QSV ICQ Main10               ✅

QSV CQP Main                 ✅
QSV CQP Main10               ✅

QSV VBR Main                 ✅
QSV VBR Main10               ✅
QSV VBR + ExtBRC Main10      ✅
QSV VBR + LookAhead Main10   ❌
QSV VBR Advanced Main10      ❌

QSV CBR Main                 ✅
QSV CBR Main10               ✅
QSV CBR + ExtBRC Main10      ✅
QSV CBR + LookAhead Main10   ❌

QSV Adaptive I Main10        ✅
QSV Adaptive B Main10        ✅

QSV LA-ICQ Main10            ❌
QSV Low Power Main           ❌
QSV Low Power Main10         ❌
```

This matrix belongs to the current worker/runtime.

Future workers may produce a different matrix.

For example a newer Intel worker might legitimately validate:

```text
LA_ICQ ✅
Look Ahead ✅
```

MVForge should then expose those features for that worker without requiring code changes.

That is the purpose of the capability-driven architecture.

---

# 14. What Codex should preserve

When modifying QSV code, preserve these principles:

1. Do not restore LA-ICQ unconditionally.
2. Do not infer Look Ahead support from FFmpeg flags.
3. Do not enable ExtBRC for ICQ/CQP/LA-ICQ.
4. ExtBRC requires compatible bitrate rate control plus runtime capability.
5. Adaptive I and Adaptive B require worker capability.
6. Adaptive I/B switches control adaptive decisions, not the existence of I/B frames.
7. Advanced features below High Quality remain disabled by policy unless intentionally changed.
8. Profile Lab must send the current switch state into the effective preview request.
9. Preview analysis must inspect the newly generated output, not stale state.
10. Output frame-structure warnings are independent from Adaptive I/B configuration warnings.
11. Requested and effective configuration must remain distinguishable.
12. Capability probes are the source of truth for the active worker.
13. A capability is contextual: encoder + profile/bit depth + rate-control + feature combination.
14. Do not collapse contextual capabilities into one generic boolean such as `supportsLookAhead`.

---

# 15. Goal

The objective is not to enable the maximum number of QSV options.

The objective is:

> Generate stable, reproducible, auditable encodes using only combinations that have been verified on the worker that will actually execute the job.

This becomes even more important as MVForge adds remote workers.

The NAS/QSV worker and future Intel workers may support different combinations.

Therefore these changes are foundational for the distributed-worker architecture and must not be reverted to static/global QSV assumptions.