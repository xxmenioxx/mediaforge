# French and Korean bitmap OCR runtime support

## Why

Canonical language normalization already resolves French aliases to `fra` and
Korean aliases to `kor`, but the supported backend image does not currently
ship those Tesseract language models. Bitmap subtitle OCR therefore fails its
runtime capability check for these languages.

## What changes

- Include the French and Korean Tesseract traineddata packages in the supported
  backend image while retaining the existing English, Spanish, and Japanese
  data.
- Verify the exact runtime language capability during image construction and
  with a repeatable smoke against a built image.
- Keep the current canonical language normalization and fail-closed OCR
  availability check as the authorities.

## Out of scope

- OCR preprocessing or accuracy changes, including the #269 benchmark.
- Translation, dictionary, AI proofreading, or semantic subtitle correction.
- Any claim about recognition accuracy without a labeled ground-truth corpus.
