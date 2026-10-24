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
- Local Tesseract: `5.5.3`; `eng`, `spa`, `jpn`, and `jpn_vert` traineddata are installed. `--help-extra` documents PSM/OEM selection and `-c VAR=VALUE`; the synthetic run used PSM 6/7 and system/frequency DAWG toggles. This does not establish that MVForge's SeConv integration can pass those flags.
- SeConv help exposes a generic `--settings:[path.json]` override for libse defaults, but does not document a Tesseract flag pass-through. MVForge does not use this option.
- The current `origin/main` backend Dockerfile pins the Linux x64 SeConv release tag `v5.1.0-rc15` with SHA-256 `e06126665545979edf2d4e859b135008c4e25458c629d29723d61d4ccca849e7`. The exact archive was fetched and matched that checksum, then run in an isolated Debian Trixie container with `libicu76`. Its executable reports `5.0.0` and the help banner says Subtitle Edit 5.0; record the pinned release tag, checksum, and executable banner separately.
- That release help confirms color isolation is on by default and documents `--no-vobsub-isolate-colors` / `--no-pgs-isolate-colors`; `--settings` overrides libse defaults. `dump-settings` exposes style colors but no documented alpha-compositing, threshold, scaling, segmentation, or Tesseract PSM/OEM pass-through controls. Internal bitmap segmentation, alpha treatment, and Tesseract arguments remain unverified.
- The SeConv 5.1.0-rc15 artifact identifies Tesseract as a selectable subprocess engine. Debian Trixie's current `tesseract-ocr` package resolved to 5.5.0 in an isolated minimal container. `origin/main` installs `eng`, `spa`, `jpn`, and `jpn_vert` traineddata for the backend runtime, not `kor` or `fra`. The local host is Tesseract 5.5.3 and has additional languages, including `kor` and `fra`; that is host-only evidence.
- MVForge's `runSeConvOCRPass` passes the input, format, Tesseract engine, language, output filename, overwrite, and optional track number. Its accurate second pass adds only the two preserve-colors flags. It does not expose Tesseract PSM/OEM or `-c` settings through this integration.
- SeConv's visible output help and MVForge's argument builder confirm the existing isolated-color primary pass and optional preserve-colors accurate pass. Candidate scoring and correction lifecycle remain authoritative.

## Synthetic Benchmark Results (Measured)

Command (the result directory must be disposable):

```bash
python3 backend/internal/handlers/benchmark/bitmap_ocr_synthetic.py \
  --workdir /private/tmp/mvforge-dupla-269/attempt-3/synthetic-ocr \
  --repeats 3
```

On the local Tesseract 5.5.3 host, 72 candidate rows ran: 3 cue layouts, shadow off/on, 1x/2x/3x nearest-neighbor glyph scale, PSM 6/7, and DAWG enabled/disabled, with 3 repeats each. Measured CER ranged 0.417–1.000 and WER 0.500–1.000. Per-row median elapsed time ranged 128.8–158.1 ms, child CPU time 125.5–154.8 ms, and the run-wide child peak RSS was 28.9 MiB. RSS is reported only as one run-wide high-water mark; it is not attributable to individual candidates. Recognized line count matched the fixture in 60/72 rows and differed in 12/72.

These results show this tiny-font fixture is hard for Tesseract and that scale/segmentation can change output; they do not establish a useful or safe improvement on real subtitle imagery. Estimates for NAS behavior: none. No per-candidate memory comparison, false-correction/false-approval, alpha/color, PGS/VobSub segmentation, cue timing, geometry, or SRT/ASS integrity measurement was possible in this fixture. No local PGS/VobSub fixture was available, and no user-authorized Samurai Champloo fixture was provided. A human-labeled representative corpus and target runtime are required before comparing the unchanged SeConv baseline with preprocessing or considering production selection.

**Decision: NO-GO for production preprocessing or new dependencies on current evidence.** This is a pending-evidence decision, not evidence that preprocessing cannot help. Do not change canonical OCR behavior until representative bitmap measurements on the target SeConv/runtime demonstrate a material improvement.

## Risks / Trade-offs

- [Synthetic glyphs do not represent PGS/VobSub] → Treat the benchmark strictly as a reproducible harness smoke test and retain unchanged production behavior.
- [Local SeConv differs from the issue baseline] → Re-run qualification on the exact supported SeConv version with labeled disposable bitmap cues before reconsidering selective OCR.
- [Resource results are local] → Record them as local measurements only; require target-runtime measurements before a NAS claim.

## Pending Qualification

- Obtain authorized disposable English PGS/VobSub cue images, including verified `|`, `I`, `l`, `1`, and outline/shadow cases. Establish human-verified text ground truth; do not commit copyrighted subtitle or video media.
- On the target SeConv/Tesseract runtime, compare the unchanged baseline with only supported variants. Record CER/WER, false corrections and approvals, review precision/recall where available, per-cue CPU/RAM/latency, and SRT/ASS integrity before revisiting the no-go decision.

## Migration Plan

No migration or rollback is needed because production OCR behavior and dependencies remain unchanged.
