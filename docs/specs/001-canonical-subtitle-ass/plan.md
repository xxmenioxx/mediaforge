# Implementation Plan — Canonical Converted Subtitle Output: ASS

**Status:** Implemented  
**Depends on:** `spec.md`

## 1. Current state

The canonical artifact path already has most of the required mechanics:

```text
ResolvedTrackPlan.SidecarOutputs
        |
        v
generateResolvedSubtitleArtifacts
        |
        +-- original ----------> originalSubtitleExtractionArgs
        |
        +-- converted text ----> textSubtitleExtractionArgs
        |
        +-- converted bitmap --> generateBitmapSubtitleAtPath
```

The main known canonical restriction is that resolved `converted` artifacts are
currently accepted only when their normalized format is `srt`.

The OCR implementation already accepts `srt` and `ass`, creates output paths
using the requested extension, validates the result, and maps ASS to SeConv's
ASS/SSA format identifier.

Therefore the implementation should prefer enabling and propagating an existing
capability rather than adding a new ASS renderer.

## 2. Design principles

1. **Pipeline owns semantics.** Do not add an Asset Info-specific conversion path.
2. **Format is data, not branching ownership.** `converted/srt` and `converted/ass` should flow through the same resolved artifact contract.
3. **Execution kind remains source-driven.**
   - bitmap source -> OCR;
   - supported text source -> FFmpeg text conversion.
4. **Original remains separate.** Native ASS/SSA extraction continues to use stream copy.
5. **Defaults are stable.** SRT remains the implicit/default converted format.
6. **Snapshot determinism is preserved.** Queue execution uses the resolved mode/format frozen in the track snapshot.

## 3. Target architecture

```text
Track Profile / Asset Info request
              |
              v
     canonical resolution
       mode + format
              |
              v
 ResolvedTrackPlan.SidecarOutputs
              |
              v
 generateResolvedSubtitleArtifacts
              |
       +------+------+
       |             |
    original      converted
       |             |
 stream copy      source kind
                 /           \
             text             bitmap
              |                 |
         FFmpeg text            OCR
          conversion       (SeConv/Tesseract)
              |                 |
           SRT/ASS            SRT/ASS
```

## 4. Subtitle attachment dependencies

ASS/SSA subtitles may depend on asset attachments, especially embedded font
files. That dependency is separate from subtitle format conversion and must not
be modeled by implicitly associating every attachment with every subtitle.

The intended relationship is:

```text
Asset
├── Subtitle stream
│     └── requires zero or more attachment dependencies
│
├── Font attachment
├── Font attachment
└── unrelated attachment
```

rather than:

```text
Subtitle stream
└── all asset attachments
```

### Original ASS/SSA

For `mode = original`, MVForge must preserve the native subtitle representation.

If the original ASS/SSA references fonts embedded as asset attachments, those
font attachments may be preserved or exported alongside the subtitle artifact
according to the existing Track Profile / attachment policy.

The subtitle and attachment must retain independent provenance and identity.

Image attachments such as JPG/PNG must not be considered subtitle dependencies
unless there is explicit evidence that the subtitle artifact requires them.

### Converted or OCR-generated ASS

For `mode = converted`, including bitmap OCR to ASS, the generated ASS is a new
textual artifact.

MVForge must not automatically associate source font attachments with that
generated ASS merely because those fonts exist in the source asset.

OCR-generated ASS should remain styling-neutral by default and should depend
only on attachments explicitly introduced or referenced by the generated
artifact.

### Dependency resolution boundary

If MVForge already has enough information to determine that an original ASS/SSA
references an embedded font, the implementation should preserve that existing
relationship.

If complete font-family-to-attachment dependency resolution is not currently
available, this feature must preserve the architectural boundary but does not
need to build a new global dependency resolver.

A dedicated follow-up may implement:

```text
ASS/SSA font-family reference
        ↓
asset font attachment inventory
        ↓
explicit subtitle → attachment dependency
```

This follow-up must not be required for canonical `converted/ass` support to
work.

### Implementation guardrails

* Do not bundle every asset attachment with every extracted subtitle.
* Do not treat JPG/PNG cover art as subtitle resources by default.
* Do not infer that OCR-generated ASS requires fonts from the bitmap source.
* Do not change Track Profile attachment-removal semantics unless required by a
  proven canonical dependency.
* Do not duplicate attachment discovery inside Asset Info.
* Preserve existing attachment provenance, stream/index identity, and artifact
  ownership.

## 5. Implementation phases

### Phase 1 — Resolve the canonical contract

Use Serena to identify the symbol(s) that:

- validate/normalize sidecar output format;
- build `ResolvedTrackPlan.SidecarOutputs`;
- default converted output to SRT;
- expose converted format in Track Profile data/API/UI.

The smallest correct change should make the canonical contract accept:

```text
converted: srt | ass
```

while preserving:

```text
default converted format: srt
```

Do not broaden support to arbitrary FFmpeg subtitle formats.

### Phase 2 — Enable canonical execution

Update the canonical resolved artifact validation so `converted/ass` is
accepted when the execution kind supports converted text.

Reuse the existing branches:

```text
bitmap_ocr -> generateBitmapSubtitleAtPath(..., format, ...)
text_ffmpeg -> textSubtitleExtractionArgs(..., format, ...)
```

Do not duplicate their command construction.

### Phase 3 — Verify OCR ASS behavior

Before adding new OCR logic, test the existing path.

Confirm that:

- `generateBitmapSubtitleAtPath(..., "ass", ...)` produces an ASS file;
- SeConv receives the expected ASS/SSA output selector;
- cleanup preserves valid timed ASS;
- `validSubtitleSidecar("ass", ...)` accepts the generated result.

Only change `subtitle_ocr.go` if a focused test demonstrates an actual missing behavior.

### Phase 4 — Surface the format selection

If Track Profile currently models converted output as SRT-only, extend its
selection/contract to allow ASS while retaining SRT as the default.

Asset Info may expose Generate ASS once the backend canonical capability is
available, but its handler/UI must request the same canonical conversion
semantics rather than build commands itself.

### Phase 5 — Tests

Prefer contract-focused tests over broad implementation tests.

Required coverage:

1. converted SRT remains valid/default;
2. converted ASS resolves as valid;
3. original ASS remains native `.ass`;
4. original SSA remains native `.ssa`;
5. text -> ASS selects canonical text conversion;
6. PGS -> ASS selects bitmap OCR;
7. VobSub -> ASS selects bitmap OCR;
8. invalid converted format is rejected;
9. Asset Info maps Generate ASS to canonical converted ASS, if exposed by this implementation;
10. existing SRT/OCR behavior remains green.

Runtime/tool-dependent OCR tests should use the repository's established test
strategy rather than making the full suite depend on host-installed OCR tools.

## 6. Expected code areas

These are navigation anchors, not permission to read entire files.

Likely canonical symbols/files:

```text
backend/internal/handlers/subtitle_transform.go
  generateResolvedSubtitleArtifacts
  resolvedSubtitleExecutionKind
  textSubtitleExtractionArgs
  validSubtitleSidecar

backend/internal/handlers/subtitle_ocr.go
  generateBitmapSubtitleAtPath
  runBitmapOCR
  runSeConvOCRPass
  seconvFormat
```

Codex should use Serena to locate the resolver/model/UI symbols responsible for
`ResolvedTrackPlan.SidecarOutputs` and converted format defaults before editing.

Asset Info integration may involve:

```text
backend/internal/handlers/assets.go
frontend/src/pages/AssetsPage.tsx
```

Do not assume those files require changes until references prove it.

## 7. Risks and guards

### Risk: ASS becomes the new implicit default

Guard: keep all empty/legacy converted format resolution mapped to SRT.

### Risk: original SSA is converted to ASS

Guard: original mode must continue using
`originalSubtitleExtractionFormat`/stream-copy semantics.

### Risk: duplicate conversion logic returns to Asset Info

Guard: Asset Info supplies intent; canonical backend resolves/executes it.

### Risk: bitmap ASS is described as lossless styling

Guard: UI/docs describe it as OCR-generated text in ASS format, not visual reconstruction.

### Risk: broad format support leaks in

Guard: accepted converted formats are an explicit finite set: `srt`, `ass`.

### Risk: all asset attachments become subtitle dependencies

Guard: subtitle-to-attachment relationships must be explicit or supported by
existing dependency information. Asset membership alone is not sufficient.

### Risk: OCR-generated ASS incorrectly inherits source fonts

Guard: converted/OCR ASS is styling-neutral by default and must not inherit
embedded fonts unless the generated ASS explicitly depends on them.

### Risk: original ASS/SSA loses required fonts

Guard: preserve the existing attachment policy and dependency information for
native ASS/SSA extraction. Do not remove a required font as an incidental
effect of adding converted ASS support.

### Risk: attachment scope expands this feature excessively

Guard: canonical ASS conversion may depend on the existing attachment contract,
but a new generalized font dependency resolver is a follow-up unless current
behavior is demonstrably incorrect and blocks this feature.


## 8. Validation strategy

Normal implementation validation:

```bash
./scripts/verify.sh --auto
```

Follow repository validation rules:

- focused checks may fail fast;
- rerun only the reported failing check when necessary;
- after fixing, rerun the same canonical verifier;
- completion requires `ALL CHECKS PASSED`.

## 9. Convergence review

After implementation, compare code/tests directly against `spec.md` and report only:

- requirements satisfied;
- any requirement intentionally deferred;
- validation scope/result;
- noteworthy limitation.

Passing tests alone is not sufficient if the implementation diverges from the spec.
