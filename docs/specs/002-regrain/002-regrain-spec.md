# Regrain Restoration Stage

**Status:** Draft / SDD  
**Scope:** Restoration pipeline, Smart Upscale, Profiles/Lab, snapshot recommendations, Preview/Test Encode/Queue parity.

## 1. Goal

Add an optional canonical **Regrain** stage that restores controlled fine texture after restoration.

Validated manual baseline:

```text
hqdn3d=1.0:1.0:3.0:3.0
-> scale=1280:720:flags=lanczos
-> setsar=1
-> cas=strength=0.08
-> noise=c0s=2:c0f=t
```

`noise=c0s=2:c0f=t` is the initial **Fine · Medium** baseline: luma-only, temporal, Gaussian.

Regrain is `Off` by default and MUST NOT automatically change denoise, upscale, sharpen, encoder, quality, GOP, B-frames, or cadence.

## 2. Canonical behavior

Regrain MUST be a first-class Restoration stage and geometry-neutral.

Canonical order:

```text
motion
-> deflicker
-> deblock
-> crop
-> chroma cleanup
-> denoise
-> deband
-> image adjustments
-> color normalization
-> smart upscale
-> SAR normalization
-> final sharpen
-> regrain
-> field metadata
```

Critical invariant:

```text
denoise/degrain -> upscale -> sharpen -> regrain
```

Consequences:

- Smart Upscale MUST resolve the same geometry with Regrain enabled or disabled.
- Regrain MUST run at final output resolution.
- Canonicalization MUST NOT move Regrain before upscale or final sharpen.
- Preview, Test Encode, Queue, and Worker MUST consume the same resolved Regrain behavior.
- Frozen Queue jobs MUST keep their resolved Regrain configuration after later Profile edits.
- `Video Codec = copy` MUST report that Regrain requires video re-encoding.
- Adding Regrain MUST NOT weaken handling of unrelated unknown Advanced Filters.

## 3. Presets and Custom

Canonical presets:

```text
Off           -> no noise filter
Fine · Light  -> noise=c0s=1:c0f=t
Fine · Medium -> noise=c0s=2:c0f=t
Fine · Strong -> noise=c0s=3:c0f=t
```

Standard presets are luma-only, temporal, Gaussian.

Custom MUST support:

```text
Luma strength
Chroma strength
Temporal: on/off
Distribution: Gaussian/Uniform
```

Custom rendering MUST be deterministic. Invalid or non-finite values MUST be rejected. Safe numeric bounds belong in `plan.md`.

The resolved Restoration state SHOULD expose:

```text
requested Regrain
resolved Regrain
effective filter
```

## 4. Snapshot / Advisor recommendation

Regrain MUST integrate with the existing Restoration recommendation plan:

```text
id = regrain
domain = Regrain
```

Recommendations MUST use persisted snapshot/restoration evidence and existing recommendation/apply-lock semantics.

### Evidence gate

An actionable automatic recommendation requires:

```text
grain availability = available
confidence >= medium
severity = low | medium | high
```

If evidence is unavailable, ambiguous, low-confidence, or severity is unknown:

```text
state = manual_review or no_recommendation
patch = none
```

The current `bitplanenoise` signal MUST NOT be treated as calibrated grain evidence while it cannot reliably distinguish grain from random noise or fine detail. Do not invent severity thresholds.

### Context gate

Reliable grain evidence alone is insufficient. Initial actionable recommendation requires:

```text
reliable grain evidence
+
denoise/degrain active or recommended
+
Smart Upscale applied or recommended
```

Other contexts may surface manual review but MUST NOT auto-apply Regrain.

### Suggested level

When both gates pass:

```text
low    -> Fine · Light
medium -> Fine · Medium
high   -> Fine · Strong
```

Applying the recommendation MUST patch only Regrain intent. It MUST NOT modify denoise, upscale, sharpen, encoder, quality, GOP, B-frames, or cadence.

Re-running analysis may change the recommendation only from newly persisted evidence. Frozen Queue jobs remain unchanged.

## 5. Compatibility / Non-goals

Existing profiles without Regrain configuration MUST behave exactly as before and should not require migration.

This feature does NOT include:

```text
automatic grain detection calibration
source-grain matching
scene-adaptive grain
automatic HQDN3D/upscale/CAS configuration
encoder-specific grain tuning
external grain synthesis dependencies
full Grain Management (Preserve/Reduce/Refine/Remove)
```

Synthetic Regrain MUST NOT be presented as restoration of information lost from the source.

## 6. Acceptance

| Case | Expected |
|---|---|
| Existing/no Regrain config | No `noise` filter; behavior unchanged |
| Fine · Light | `noise=c0s=1:c0f=t` |
| Fine · Medium | `noise=c0s=2:c0f=t` |
| Fine · Strong | `noise=c0s=3:c0f=t` |
| Upscale + CAS + Medium | `...scale...,setsar=1,cas=...,noise=c0s=2:c0f=t` |
| Medium without upscale | Regrain runs at final source geometry |
| Smart Upscale + Regrain | No geometry-unresolved warning caused by Regrain |
| Preview/Test Encode/Queue | Same resolved mode, filter, and order |
| Video copy + Regrain | Explicitly reports re-encode requirement |
| Ambiguous/low-confidence grain evidence | No actionable recommendation |
| Reliable medium grain + denoise + upscale | Suggest Fine · Medium |
| Apply recommendation | Only Regrain configuration changes |

## 7. Follow-up boundary

A future **Grain Management** feature may combine analysis, degrain, upscale, and Regrain into higher-level `Preserve / Reduce / Refine / Remove` behavior. That is outside this spec.
