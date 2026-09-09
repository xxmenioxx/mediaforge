# Canonical Converted Subtitle Output: ASS

**Status:** Draft / SDD pilot  
**Baseline:** MVForge `main` after commit `8d5d7f1`  
**Scope:** Subtitle sidecar generation and the UI/contracts that select its output format

## 1. Problem

MVForge has a canonical subtitle artifact pipeline, but its resolved `converted`
sidecar path currently accepts only SRT.

The OCR implementation already understands both SRT and ASS output. This leaves
the system in an inconsistent state:

- canonical Pipeline conversion exposes SRT only;
- bitmap OCR already has ASS-capable primitives;
- Asset Info was recently corrected so it no longer owns a separate,
  noncanonical ASS conversion path.

The next step is to make ASS a canonical converted subtitle output rather than
reintroducing feature-specific conversion behavior.

## 2. Goal

Allow a resolved subtitle sidecar with:

```text
mode = converted
format = ass
```

to use the same canonical execution path as converted SRT.

SRT remains the default converted output.

## 3. Non-goals

This feature MUST NOT:

- reconstruct or claim to preserve the visual styling of bitmap PGS/VobSub subtitles;
- introduce a second Asset Info-only subtitle conversion implementation;
- replace or redesign the OCR engine;
- change OCR language selection or OCR cleanup policy except where required for correct ASS output;
- change lossless `original` extraction semantics;
- silently convert original ASS/SSA when the user requested `original`;
- change SRT from the default converted format.

OCR-generated ASS is a textual representation of recognized cues. It is not a
lossless representation of the original bitmap appearance.

## 4. Terms

### Original

`mode = original`

Extract the subtitle in its supported native representation without intentional
text conversion.

Examples:

- ASS -> `.ass`
- SSA -> `.ssa`
- SubRip -> `.srt`
- PGS -> `.sup`

### Converted

`mode = converted`

Produce a textual sidecar in an explicitly selected canonical output format.

Supported converted formats after this feature:

```text
srt
ass
```

Default when a converted format is not explicitly selected:

```text
srt
```

## 5. Requirements

### R1 — Canonical format support

The canonical resolved subtitle artifact pipeline MUST support both:

```text
converted + srt
converted + ass
```

No caller may need a private conversion implementation to produce either format.

### R2 — SRT remains default

Existing profiles, requests, or resolved decisions that rely on the current
default converted format MUST continue to resolve to SRT unless ASS is explicitly selected.

### R3 — Bitmap source to SRT

For supported bitmap subtitle codecs such as PGS and VobSub:

```text
bitmap source
-> converted/srt
-> canonical OCR
-> valid .srt sidecar
```

Existing behavior must remain intact.

### R4 — Bitmap source to ASS

For supported bitmap subtitle codecs such as PGS and VobSub:

```text
bitmap source
-> converted/ass
-> canonical OCR
-> valid .ass sidecar
```

The artifact MUST be identified and published as ASS.

### R5 — Text source to ASS

For supported text subtitle codecs:

```text
text source
-> converted/ass
-> canonical text conversion
-> valid .ass sidecar
```

The canonical text conversion helper/path must be reused.

### R6 — Text source to SRT

Existing canonical text-to-SRT conversion MUST remain unchanged.

### R7 — Original extraction is unchanged

`mode = original` MUST continue to preserve the native supported format.

In particular:

```text
ASS + original -> .ass via original extraction
SSA + original -> .ssa via original extraction
```

Selecting `original` must never mean “convert to ASS”.

### R8 — Shared behavior

Pipeline is the source of truth for subtitle artifact semantics.

Asset Info MUST consume the same canonical rules. Once canonical ASS conversion
is available, Asset Info may expose Generate ASS only by requesting the
canonical converted behavior.

Asset Info MUST NOT construct its own ASS transcoding/OCR rules.

### R9 — Validation

Generated ASS MUST pass the existing canonical ASS sidecar validation or its
correctly extended equivalent.

At minimum a generated ASS sidecar must be structurally recognizable as ASS
timed subtitle content.

### R10 — Failure behavior

Unsupported codec/format combinations MUST fail explicitly.

A failure in one artifact must preserve the current canonical rollback/error
semantics for the sidecar set.

### R11 — Subtitle attachment dependencies

Subtitle-related attachments MUST remain distinct from subtitle conversion.

For original ASS/SSA extraction:

- font attachments required by the subtitle MAY be preserved/exported alongside
  the subtitle artifact;
- MVForge MUST preserve attachment provenance and identity;
- unrelated attachments MUST NOT be treated as subtitle dependencies.

For OCR-generated or otherwise converted ASS:

- MVForge MUST NOT automatically associate source font attachments unless the
  generated ASS explicitly depends on them;
- OCR-generated ASS remains a neutral textual representation by default.

Image attachments such as JPG/PNG MUST NOT be assumed to be subtitle
dependencies solely because they are attached to the same asset.

### R12 — Attachment dependency resolution

When an ASS/SSA subtitle references a font family, MVForge SHOULD resolve that
requirement against the asset's font attachments.

The relationship should be represented explicitly:

Subtitle artifact
    ↓ requires
Font attachment(s)

rather than treating all asset attachments as belonging to every subtitle.


## 6. Acceptance scenarios

| Source | Mode | Requested format | Expected result |
|---|---|---:|---|
| ASS | original | ass | lossless/native `.ass` |
| SSA | original | ssa | lossless/native `.ssa` |
| SubRip | original | srt | native `.srt` |
| PGS | original | sup | native `.sup` |
| SubRip | converted | srt | canonical `.srt` |
| ASS | converted | srt | canonical `.srt` |
| SubRip | converted | ass | canonical `.ass` |
| SSA | converted | ass | canonical `.ass` |
| PGS | converted | srt | OCR -> `.srt` |
| PGS | converted | ass | OCR -> `.ass` |
| VobSub/DVD subtitle | converted | srt | OCR -> `.srt` |
| VobSub/DVD subtitle | converted | ass | OCR -> `.ass` |

## 7. Compatibility requirements

- Existing Track Profiles that select converted SRT require no migration.
- Existing Queue snapshots must retain their frozen format/mode semantics.
- Artifact identity/path generation must continue to distinguish mode and format.
- Preview/Test Encode/Queue must not acquire divergent subtitle format rules.
- Existing SRT OCR and SRT text-conversion tests must remain green.

## 8. Definition of done

The feature is complete only when:

1. the canonical resolved pipeline accepts `converted/ass`;
2. both text and bitmap sources can produce valid ASS through canonical paths;
3. SRT remains the default;
4. original ASS/SSA extraction remains native/lossless;
5. Asset Info does not contain a separate ASS conversion implementation;
6. focused tests cover the acceptance matrix at the appropriate resolver, planning, command, and OCR boundaries;
7. `./scripts/verify.sh --auto` reports `ALL CHECKS PASSED`;
8. implementation is compared against this spec and no requirement is left silently unmet.

## Implementation status

**Status:** Implemented
**Implemented in:** `a19b99f`
**Hardened in:** `3b9c869`

### Deferred follow-up

The generalized ASS/SSA font dependency resolver is intentionally deferred.

Future work may resolve:

ASS/SSA font-family reference
→ asset font attachment inventory
→ explicit subtitle → attachment dependency

The current implementation preserves existing attachment policy and provenance,
does not associate unrelated attachments with subtitles, and does not implicitly
attach source fonts to converted/OCR-generated ASS.

This follow-up is related to R12 but is not required for canonical
`converted/ass` support.