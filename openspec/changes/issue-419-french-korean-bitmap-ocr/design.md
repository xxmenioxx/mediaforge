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

A dedicated runtime image smoke will inspect the running language list and
installed package sizes, then initialize the French and Korean models against
a synthetic blank PGM. It reports container image size and package installed
sizes. The synthetic probe measures model initialization only and is not an
accuracy or production-throughput benchmark.

The repository does not contain a decodable synthetic PGS/VobSub fixture or a
generator that can exercise actual SeConv recognition. Existing PGS test
fixtures cover parser/geometry behavior and stub SeConv/Tesseract. Therefore the
runtime capability smoke reports package/list/model initialization as PASS and
SeConv-to-PGS/VobSub recognition E2E as BLOCKED; the PGM probe is not presented
as an OCR recognition substitute.

No real media, NAS path, production database, or new package source is used.

## Lane ownership and parallel safety

Lane #419 owns `backend/Dockerfile`, the narrow
`normalizedOCRLanguage` change in `backend/internal/handlers/subtitle_ocr.go`,
dedicated runtime-language tests and smoke files, and this issue-specific
OpenSpec delta. Lane #267 owns the separate OCR-quality suggestion path and its
own issue artifacts. Those source paths do not overlap, so the implementation
lanes remain `PARALLEL_SAFE`; Docker and canonical verification are serialized
through `shared-validation`.
