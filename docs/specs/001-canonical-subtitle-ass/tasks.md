# Tasks — Canonical Converted Subtitle Output: ASS

**Status:** Ready for implementation after spec review  
**Execution model:** Complete in order; keep changes small and independently reviewable where practical.

## Task 1 — Discover the canonical format contract

- [ ] Follow `AGENTS.md`.
- [ ] Read `spec.md` and `plan.md`.
- [ ] Use Serena MCP semantic navigation.
- [ ] Locate the symbols that create/normalize `ResolvedTrackPlan.SidecarOutputs`.
- [ ] Locate the current SRT default.
- [ ] Locate all canonical guards that reject converted formats other than SRT.
- [ ] Locate Track Profile/API/UI format constraints, if any.
- [ ] Report before editing:
  1. current root cause;
  2. smallest safe implementation;
  3. exact symbols/files to modify.

**Exit condition:** We know where ASS is blocked and whether the data model already carries arbitrary `format` values.

## Task 2 — Extend canonical resolution to ASS

- [ ] Permit only `srt` and `ass` for `mode=converted`.
- [ ] Preserve SRT as the default.
- [ ] Preserve `original` mode semantics.
- [ ] Do not add generic arbitrary-format support.
- [ ] Add focused resolver/planning tests.

**Acceptance:**

```text
converted + empty/default -> srt
converted + srt           -> srt
converted + ass           -> ass
converted + unsupported   -> explicit rejection
```

## Task 3 — Enable canonical execution

- [ ] Ensure `generateResolvedSubtitleArtifacts` accepts resolved `converted/ass`.
- [ ] Reuse `textSubtitleExtractionArgs` for supported text sources.
- [ ] Reuse `generateBitmapSubtitleAtPath` for supported bitmap sources.
- [ ] Preserve rollback/error behavior.
- [ ] Add focused execution-selection tests.

**Acceptance:**

```text
text + converted/ass   -> text_ffmpeg
bitmap + converted/ass -> bitmap_ocr
```

## Task 4 — Prove existing OCR ASS capability

- [ ] Add/adjust focused tests around the existing OCR ASS path.
- [ ] Confirm `ass` maps to SeConv ASS/SSA output.
- [ ] Confirm generated ASS passes canonical sidecar validation.
- [ ] Confirm PGS and VobSub use the same canonical OCR pipeline.
- [ ] Do not rewrite OCR logic unless a test proves a missing capability.

**Acceptance:**

```text
PGS    -> OCR -> valid ASS
VobSub -> OCR -> valid ASS
```

The output is textual OCR-derived ASS; visual bitmap styling preservation is not required.

## Task 5 — Expose canonical ASS selection

- [ ] If Track Profile has a converted output format selector, allow ASS.
- [ ] Keep SRT selected/default for existing and new default behavior.
- [ ] If Asset Info exposes Generate ASS, map it to canonical `converted/ass`.
- [ ] Do not restore a private Asset Info FFmpeg/OCR conversion branch.
- [ ] Add focused frontend/API tests only where behavior changes.

## Task 6 — Preserve subtitle attachment invariants

* [ ] Use Serena to locate the existing attachment inventory, attachment policy,
  and any subtitle/font dependency representation before editing attachment
  code.
* [ ] Confirm original ASS/SSA extraction does not lose existing font
  attachment behavior.
* [ ] Confirm converted/OCR-generated ASS does not automatically inherit all
  source attachments.
* [ ] Confirm JPG/PNG and other unrelated attachments are not classified as
  subtitle dependencies merely because they belong to the same asset.
* [ ] Preserve attachment provenance and identity.
* [ ] Do not introduce a generalized font dependency resolver unless an
  existing failing requirement proves it is necessary for this feature.

### Acceptance

```text
original ASS/SSA
    +
existing required font relationship
    ->
relationship remains available/preserved
```

```text
OCR PGS/VobSub
    ->
converted ASS
    ->
no implicit dependency on source fonts
```

```text
asset
├── subtitle
├── font.ttf
└── cover.jpg

does NOT imply:

subtitle -> font.ttf + cover.jpg
```

* [ ] If MVForge already resolves ASS/SSA font requirements against embedded
  fonts, add or preserve focused regression coverage.
* [ ] If that resolver does not yet exist, document the missing capability as a
  follow-up rather than expanding this task silently.

**Exit condition:** ASS support does not weaken or incorrectly broaden the
existing subtitle/attachment contract.


## Task 7 — Regression coverage

Verify at minimum:

- [ ] ASS original -> native `.ass`.
- [ ] SSA original -> native `.ssa`.
- [ ] SRT converted -> `.srt`.
- [ ] text converted -> `.ass`.
- [ ] PGS converted -> `.srt`.
- [ ] PGS converted -> `.ass`.
- [ ] VobSub converted -> `.srt`.
- [ ] VobSub converted -> `.ass`.
- [ ] unsupported converted format -> explicit error.
- [ ] Asset Info and Pipeline resolve equivalent intent consistently.
- [ ] original ASS/SSA keeps existing required-font attachment behavior;
- [ ] converted/OCR ASS does not automatically inherit source fonts;
- [ ] unrelated JPG/PNG attachments remain unrelated to subtitle artifacts;
- [ ] attachment identity/provenance remains unchanged;


## Task 8 — Canonical validation

Run only:

```bash
./scripts/verify.sh --auto
```

If it fails, follow the verifier's reported failing command according to `AGENTS.md`.
After the fix, rerun `--auto`.

**Exit condition:**

```text
ALL CHECKS PASSED
```

## Task 9 — Converge against the spec

Before declaring completion:

- [ ] Re-read only `spec.md`.
- [ ] Check R1–R12 explicitly against the implementation/tests.
- [ ] Confirm all acceptance scenarios are implemented or explain any evidence-backed limitation.
- [ ] Confirm no Asset Info-specific subtitle conversion path was introduced.
- [ ] Confirm SRT remains default.
- [ ] Confirm R11 subtitle attachment semantics are preserved.
- [ ] Confirm R12 dependency resolution is either satisfied by existing
      behavior or explicitly recorded as a follow-up.

Final report should be concise:

```text
Spec:
- R1-R10 satisfied / exceptions

Changed:
- files/symbols

Validation:
- ./scripts/verify.sh --auto
- scope/check count
- ALL CHECKS PASSED

Limitations:
- only material limitations
```
