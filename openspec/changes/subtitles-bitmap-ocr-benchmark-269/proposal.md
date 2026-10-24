## Why

Bitmap subtitle OCR errors can reach contextual correction and review even when the underlying image could support a better recognition result. Measurements are needed before adding preprocessing or changing canonical OCR behavior, especially because production workers have limited shared CPU and memory.

## What Changes

- Record a reproducible, disposable-fixture benchmark and verified runtime capability report for the installed SeConv/Tesseract path.
- Compare supported image and segmentation candidates and record accuracy, output structure, latency, and resource measurements where measurable.
- Adopt selective preprocessing only when evidence demonstrates a material benefit without unacceptable regressions; otherwise document a no-go and keep production OCR unchanged.

## Capabilities

This is a benchmark and qualification change. It does not alter observable subtitle behavior or change an existing requirement, so it intentionally has no spec delta (`skip_specs: true`).

## Impact

- Adds a standard-library synthetic benchmark harness and issue-specific OpenSpec evidence.
- No API, persistence, dependency, translation, frontend, or production OCR behavior changes are intended without measured justification.
- Tests use generated disposable fixtures only. NAS and production media are outside scope.
