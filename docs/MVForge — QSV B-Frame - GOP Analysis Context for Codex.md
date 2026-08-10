# MVForge — QSV B-Frame / GOP Analysis Context for Codex

## Purpose

This context explains a specific QSV output-analysis issue observed in MVForge Profile Lab.

The goal is to make sure future changes do NOT incorrectly equate:

```text
Adaptive B enabled
```

with:

```text
B-frames exist in the output
```

or incorrectly suppress valid frame-structure warnings when `Adaptive B` is disabled.

---

# 1. Asset analyzed

A representative source asset was analyzed before and after a QSV preview/conversion.

The source frame structure was:

```text
Source:
I frames: 7
P frames: 203
B frames: 290
B ratio: 58.0%

Maximum consecutive B-run: 3
Average GOP: 82.7
```

MVForge described the source as:

```text
The stream uses a strong B-frame structure.
```

This source structure is considered normal/reasonable.

It contains:

- regular I frames,
- a substantial number of P frames,
- B frames,
- short B-frame runs,
- a moderate GOP length.

Conceptually:

```text
I → P → B B → P → B B B → P ...
```

The important characteristic is that the stream has a balanced I/P/B structure.

---

# 2. QSV output observed

Using QSV, a generated output produced:

```text
Output:
I frames: 2
P frames: 0
B frames: 477
B ratio: 99.6%

Maximum consecutive B-run: 247
Average GOP: 248.0
```

MVForge's generic assessment also said:

```text
The stream uses a strong B-frame structure.
```

That description is too weak for this result.

The important differences are:

```text
Source B ratio:
58.0%

Output B ratio:
99.6%
```

and:

```text
Source B-run max:
3

Output B-run max:
247
```

and especially:

```text
Output P frames:
0
```

This represents a very large structural change.

---

# 3. Interpretation

Do NOT automatically classify this output as broken or corrupted.

Some hardware encoders, QSV implementations, FFmpeg reporting behavior, and HEVC reference structures can produce results that look unusual when analyzed only by frame type.

However, MVForge also should NOT classify this as simply:

```text
healthy strong B-frame structure
```

without qualification.

For MVForge this should be treated as:

```text
UNUSUAL FRAME STRUCTURE
```

or:

```text
WARNING / REVIEW RECOMMENDED
```

because the output differs dramatically from the source.

The important signal is not merely that B-frames exist.

The signal is the combination:

```text
P frames == 0

AND

B ratio extremely high

AND

maximum B-run extremely high

AND

large deviation from source structure
```

---

# 4. Suggested analysis rule

A reasonable first-pass rule would be:

```text
if output.PFrames == 0
   and output.BRatio >= 0.95
   and output.MaxBRun is very large
then
   verdict = warning
```

The warning could say:

```text
Unusual GOP/frame structure detected.
The encoded output is almost entirely composed of B-frames
and contains no detected P-frames.
```

If source statistics are available, compare the two.

For example:

```text
Source B ratio: 58.0%
Output B ratio: 99.6%

Source max B-run: 3
Output max B-run: 247
```

Then MVForge should additionally report:

```text
The output frame structure changed substantially from the source.
Review compatibility and visual quality before adopting this
configuration as a standard profile.
```

---

# 5. Do NOT tie this warning directly to Adaptive B

This distinction is critical.

`Adaptive B` is a QSV encoder option that allows the encoder to make adaptive decisions involving B/P frame usage.

It does NOT mean:

```text
Adaptive B ON
→ use B-frames
```

and it does NOT mean:

```text
Adaptive B OFF
→ no B-frames
```

Even with:

```text
qsvAdaptiveB = false
```

QSV may still generate B-frames.

B-frame presence/quantity is influenced by the encoder's GOP/reference-frame strategy and other parameters.

Therefore:

```text
Adaptive B state
```

and:

```text
actual output B-frame structure
```

must be analyzed independently.

---

# 6. Adaptive I has the same distinction

Similarly:

```text
Adaptive I OFF
```

does NOT mean:

```text
output contains no I-frames
```

Adaptive I controls adaptive I-frame decisions.

It does not disable I-frame generation.

Do not interpret frame counts as direct proof that Adaptive I or Adaptive B was enabled.

---

# 7. Profile Lab behavior to verify

When a user changes:

```text
Adaptive I
Adaptive B
```

in Profile Lab, MVForge must generate a NEW preview using the current draft configuration.

If Adaptive B is disabled, the effective FFmpeg command must not continue requesting:

```text
-adaptive_b 1
```

Likewise, if Adaptive I is disabled, the command must not continue requesting:

```text
-adaptive_i 1
```

Acceptable behavior is either:

```text
option absent
```

or an explicit disabled value if that is how the command builder is designed.

The important point is that the new preview must reflect the current UI state.

---

# 8. Potential bug being investigated

A possible Profile Lab bug was observed/reported:

> After disabling Adaptive I and Adaptive B, the frame-structure analysis continued showing warnings about the B-frame-heavy output.

Do NOT assume this means the warning itself is incorrect.

There are two possibilities.

## Case A — real bug

The switches are OFF, but the newly generated FFmpeg command still contains:

```text
-adaptive_i 1
-adaptive_b 1
```

or Profile Lab reuses stale preview/analysis data.

Then the bug is in:

```text
UI draft
→ preview request
→ backend profile
→ FFmpeg command
→ preview cache/state
```

This should be fixed.

## Case B — expected behavior

The switches are OFF.

The effective command correctly does NOT request Adaptive I/B.

A fresh preview is created.

The new output still contains:

```text
P = 0
B ≈ 100%
very large B-run
```

Then the warning is valid.

That means QSV is still producing an unusual GOP/frame structure without Adaptive B.

In that case, investigation should move to:

```text
-bf
-g
GOP/reference structure
rate control
QSV encoder behavior
effective encoder options
```

rather than removing the warning.

---

# 9. Configuration warning vs output warning

MVForge should keep two different classes of warnings.

## Configuration/capability warning

Example:

```text
Adaptive B was requested but is not supported by this worker.
```

This warning depends on:

```text
requested configuration
+
worker capability probe
```

It should disappear if Adaptive B is not requested.

## Output-analysis warning

Example:

```text
Output contains 99.6% B-frames, no detected P-frames,
and a maximum consecutive B-run of 247.
```

This depends only on the output that was actually encoded.

It must remain visible even if Adaptive B is OFF, as long as the actual encoded output still has that structure.

Do NOT merge these two warning categories.

---

# 10. Avoid the current generic assessment

The current/previous text:

```text
The stream uses a strong B-frame structure.
```

is insufficient for an output like:

```text
I 2
P 0
B 477
B ratio 99.6%
B-run max 247
```

It makes a potentially abnormal structure sound desirable.

Instead, assessment should distinguish normal B-frame usage from extreme structures.

For example:

```text
Normal:
The stream uses B-frames with a balanced I/P/B structure.
```

versus:

```text
Warning:
The output has an unusually B-frame-dominant structure
with no detected P-frames and very long B-frame runs.
```

---

# 11. Source-relative analysis is important

Do not evaluate the output only using fixed thresholds.

When possible, compare source and output.

Example:

```text
Source:
B ratio 58%
B-run 3
Average GOP 82.7

Output:
B ratio 99.6%
B-run 247
Average GOP 248
```

This is a strong deviation.

A future fidelity metric could evaluate values such as:

```text
B-ratio delta
GOP delta
P-frame disappearance
I-frame density change
maximum B-run change
```

The goal is NOT to preserve the exact source GOP.

Re-encoding naturally changes frame structure.

The goal is to identify unusually large structural changes that may affect:

- seek behavior,
- decoder compatibility,
- playback devices,
- editing behavior,
- quality consistency,
- unexpected encoder configuration.

---

# 12. Specific QSV configuration observed

The problematic/interesting result was observed with approximately:

```text
Encoder: QSV
Preset: Slow
Quality: 29
```

Do NOT conclude:

```text
Quality 29 is the cause.
```

Quality level and GOP/frame structure are separate concerns.

Changing quality from 29 to 28/27 should not be the first attempted fix.

First inspect the actual command and GOP/B-frame-related behavior.

---

# 13. Extended BRC is not the fix

Do NOT enable Extended BRC to try to correct this frame structure.

Extended BRC is related to bitrate-based rate control.

It is not a GOP/B-frame repair mechanism.

In MVForge policy:

```text
ICQ → ExtBRC OFF
CQP → ExtBRC OFF
LA-ICQ → ExtBRC OFF

VBR/CBR → ExtBRC may be available when the worker probe validates it.
```

Do not couple ExtBRC to this B-frame warning.

---

# 14. What Codex should preserve

When working on this code:

1. Preserve output frame-structure analysis independently from Adaptive I/B state.
2. Do not remove B-frame warnings merely because Adaptive B is OFF.
3. Verify that disabling Adaptive B actually removes the requested encoder option.
4. Verify that disabling Adaptive I actually removes the requested encoder option.
5. Ensure Profile Lab analyzes the newly generated preview, not stale output.
6. Separate capability/config warnings from actual-output warnings.
7. Treat `P=0 + ~100% B + huge B-run` as unusual.
8. Compare output structure with source structure when possible.
9. Do not infer Adaptive B state from output B-frame counts.
10. Do not infer Adaptive I state from output I-frame counts.
11. Do not use Extended BRC as a workaround.
12. Do not automatically blame QSV quality value 29.
13. Preserve the capability-driven QSV architecture.
14. Prefer inspection of the effective FFmpeg command before changing encoder policy.

---

# Desired outcome

MVForge should be able to say:

```text
Adaptive B:
Disabled as requested.

Effective command:
Adaptive B option not present.

Actual output:
99.6% B-frames
0 P-frames
max B-run 247

Verdict:
Unusual frame structure detected.

Conclusion:
The unusual output structure is produced by the effective QSV
configuration even without Adaptive B. Investigate GOP/reference-frame
behavior rather than changing the Adaptive B switch logic.
```

That distinction is the core of this issue.