# Design: French and Korean bitmap OCR runtime support

The supported backend container remains the sole runtime packaging authority.
It will install `tesseract-ocr-fra` and `tesseract-ocr-kor` alongside the
existing `eng`, `spa`, `jpn`, and `jpn_vert` packages. The Docker build will
fail unless Tesseract itself lists all six canonical language codes.
The engine package is pinned to `5.5.0-1+b1`; all six traineddata packages are
pinned to `1:4.1.0-2` for reproducible image builds.

The OCR language normalizer reuses `normalizedTrackLanguageValue` to canonicalize
catalogued tags such as `fr-FR` and `ko-KR`, then preserves its existing OCR
aliases for `fra`, `fre`, `french`, `kor`, and `korean`. The canonical result is
passed to `requireOCRLanguageAvailable`, which checks the running Tesseract
capability report before recognition. Unknown or unsupported values retain the
existing `eng` default; this change does not broaden that behavior or alter the
OCR algorithm.

A dedicated runtime image smoke inspects the running language list and
installed package sizes, then initializes the French and Korean models against
a synthetic blank PGM. It reports container image size and package installed
sizes. The synthetic probe measures model initialization only and is not an
accuracy or production-throughput benchmark. Its report marks runtime
capability as PASS after all checks complete and marks SeConv PGS/VobSub E2E as
`NOT_RUN_BY_THIS_SMOKE`; a separately executed E2E status is recorded as
PASS/FAIL only when supplied from that run.

Versioned fixture inputs consist of locally authored French/Korean PNGs and
their synthetic PGS SUPs. The fixture manifest pins SHA-256 for these source
files. The standalone SeConv E2E does not require a checked-in MKV: it copies
the verified SUP files into a temporary directory, then uses FFmpeg from the
supported backend image with `-copyts` to mux a synthetic black FFV1 video and
two PGS tracks. ffprobe must confirm the FFV1 video, subtitle stream indexes 1
(`fra`) and 2 (`kor`), PGS codecs, titles, and cue-boundary packet timestamps
before the OCR test runs. The generated MKV has no fixed hash because FFmpeg
version and container metadata can vary; its SHA-256 is instead compared
before and after OCR, and the temporary directory is removed automatically.
The Go E2E then executes real SeConv and Tesseract in the supported image and
validates effective language, stream identity, cue timing, bitmap geometry,
and valid SRT/ASS sidecars. Recognition text only needs to be non-empty, so
this functional capability check does not imply exact-text recognition
accuracy. OCR accuracy results remain separate from runtime capability.

No real media, NAS path, production database, or new package source is used.

## Lane ownership and parallel safety

Lane #419 owns `backend/Dockerfile`, the narrow
`normalizedOCRLanguage` change in `backend/internal/handlers/subtitle_ocr.go`,
dedicated runtime-language tests and smoke files, and this issue-specific
OpenSpec delta. Lane #267 owns the separate OCR-quality suggestion path and its
own issue artifacts. Those source paths do not overlap, so the implementation
lanes remain `PARALLEL_SAFE`; Docker and canonical verification are serialized
through `shared-validation`.
