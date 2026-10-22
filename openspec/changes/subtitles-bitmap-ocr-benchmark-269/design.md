## Context

The canonical implementation is `runBitmapOCRLanguageAttempt` and `runSeConvOCRPass` in `backend/internal/handlers/subtitle_ocr.go`. A normal pass invokes SeConv with the Tesseract engine, selected language, and output path. Accurate mode optionally runs a second pass preserving colors, then uses the existing language-aware OCR score to select a valid sidecar. SeConv owns bitmap extraction/segmentation and its internal Tesseract invocation. MVForge does not pass PSM, OEM, threshold, or DAWG options. Existing SeConv cleanup remains downstream and bounded; this change does not modify it.

## Goals / Non-Goals

**Goals:** Record actual local tool capabilities, run a deterministic synthetic benchmark with ground truth, and make an evidence-based adoption decision.

**Non-Goals:** Change the canonical OCR pipeline, correction/approval policy, output serializers, or subtitle evidence lifecycle; use copyrighted or production media; claim NAS performance from a developer machine.

## Decisions

- Keep the existing OCR contract and skip spec deltas because this benchmark does not introduce production behavior.
- Use a Python standard-library harness that renders deterministic 5x7 PGM cues and invokes the installed Tesseract executable directly. It needs no image package and writes only beneath the caller-supplied work directory.
- Use CER/WER against embedded fixture ground truth, plus child elapsed/CPU time, observed child peak RSS, and recognized-line-count delta. The harness labels its limits: it is not a PGS/VobSub or SeConv benchmark and cannot measure correction false approvals or SRT/ASS integrity.
- Do not adopt preprocessing from these data. The fixture glyphs are intentionally crude and do not represent broadcast fonts or bitmap palettes; local SeConv is 5.0.0 while the issue's stated version is 5.1.0-rc15. A real labeled local PGS/VobSub corpus and target runtime are required before production selection can be justified.

## Verified Capability Evidence

- Local `SeConv --version`: `5.0.0`. Its help identifies Subtitle Edit 5.0 batch conversion, Tesseract as a selectable OCR engine, color isolation as enabled by default, and `--no-vobsub-isolate-colors` / `--no-pgs-isolate-colors` switches. Help describes isolation as binarizing to black-on-white and dropping outline colors; PGS help says this leaves the white glyph fill available on the OCR canvas.
- Local Tesseract: `5.5.3`; `eng`, `spa`, `jpn`, and `jpn_vert` traineddata are installed. Its CLI documents PSM 6/7, OEM selection, and `-c` configuration. This does not establish that MVForge's SeConv integration can pass those flags.
- Alpha compositing, internal thresholding, bitmap segmentation details, exact Tesseract arguments used internally by SeConv, and the behavior of the issue's expected SeConv 5.1.0-rc15 are unverified.
- SeConv's visible output help and MVForge's argument builder confirm the existing isolated-color primary pass and optional preserve-colors accurate pass. Candidate scoring and correction lifecycle remain authoritative.

## Synthetic Benchmark Results (Measured)

Command (the result directory must be disposable):

```bash
python3 backend/internal/handlers/benchmark/bitmap_ocr_synthetic.py \
  --workdir /private/tmp/mvforge-dupla-269/attempt-1/synthetic-ocr \
  --repeats 3
```

On the local Tesseract 5.5.3 host, 72 candidate rows ran: 3 cue layouts, shadow off/on, 1x/2x/3x nearest-neighbor glyph scale, PSM 6/7, and DAWG enabled/disabled, with 3 repeats each. Measured CER ranged 0.417–1.000 and WER 0.500–1.000. Per-row median elapsed time ranged 133.0–237.6 ms, child CPU time 127.2–226.9 ms, and observed child peak RSS reached 29.1 MiB. Recognized line count matched the fixture in 60/72 rows and differed in 12/72.

These results show this tiny-font fixture is hard for Tesseract and that scale/segmentation can change output; they do not establish a useful or safe improvement on real subtitle imagery. Estimates for NAS behavior: none. No false-correction/false-approval, alpha/color, PGS/VobSub segmentation, cue timing, geometry, or SRT/ASS integrity measurement was possible in this fixture.

## Risks / Trade-offs

- [Synthetic glyphs do not represent PGS/VobSub] → Treat the benchmark strictly as a reproducible harness smoke test and retain unchanged production behavior.
- [Local SeConv differs from the issue baseline] → Re-run qualification on the exact supported SeConv version with labeled disposable bitmap cues before reconsidering selective OCR.
- [Resource results are local] → Record them as local measurements only; require target-runtime measurements before a NAS claim.

## Migration Plan

No migration or rollback is needed because production OCR behavior and dependencies remain unchanged.
