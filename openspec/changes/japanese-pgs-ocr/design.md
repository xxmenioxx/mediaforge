## Context

The existing canonical subtitle pipeline classifies PGS and VobSub as bitmap
inputs, extracts PGS to a temporary SUP when needed, runs SeConv with
Tesseract, applies the selected OCR cleanup mode, validates the resulting cues,
and then renders SRT and ASS from the canonical result. Asset Info, Queue, and
the other subtitle entry points must continue to use that pipeline; see the
proposal and the `subtitles` delta spec for the observable contract.

The current runtime image installs `tesseract-ocr-jpn`, while the application
can pass both `jpn` and `jpn_vert`. Package intent is not sufficient evidence
that an exact language identifier is usable by the running Tesseract binary.
SeConv output also distinguishes a genuine “No subtitles recognised” result
from other command or input failures only after its output is inspected.

## Goals / Non-Goals

**Goals:**

- Establish one runtime capability result per OCR operation before recognition
  begins, using the exact language identifiers passed to Tesseract.
- Keep requested, effective, attempted, and ultimately successful OCR language
  information distinct in diagnostics.
- Add only the bounded Japanese orientation retry required by issue #24,
  reusing the already prepared bitmap/PGS input and canonical workspace.
- Preserve one successful recognition result as the sole source for SRT, ASS,
  and logical `both` output.
- Keep failures concise, actionable, and compatible with the existing
  operation/artifact error model.

**Non-Goals:**

- Changing OCR cleanup modes, color-isolation policy, cue normalization,
  subtitle naming, sidecar discovery, publication, or operation ownership.
- Adding fallback between arbitrary languages or retrying generic tool failures.
- Introducing another OCR engine, another background-job system, or a separate
  Asset Info subtitle pipeline.
- Making OCR progress imply per-image completion when SeConv does not expose
  truthful per-image progress.

## Decisions

### 1. Probe the running Tesseract capability before OCR

Before a bitmap OCR attempt starts, resolve the effective language using the
existing canonical language normalization and run the runtime's Tesseract
language listing command through the existing argument-array process boundary.
Parse the exact language identifiers from that result and pass the immutable
capability set to the operation's recognition logic. The probe runs once per
operation/workspace, so a Japanese fallback does not perform another capability
probe; it also avoids a long-lived process cache becoming stale after a runtime
or test binary changes.

The container build must install or otherwise provide the traineddata required
for every Japanese identifier exposed by MVForge, and image validation must
assert that the running binary lists the exact identifiers `jpn` and
`jpn_vert`. Runtime preflight remains authoritative even when the image build
declares a package, and an unavailable identifier fails before SeConv/Tesseract
recognition is launched.

Alternative considered: infer support from Debian package names or from the UI
language list. Rejected because either source can diverge from the actual
runtime binary and cannot distinguish a missing traineddata file from a usable
language.

### 2. Use structured recognition outcomes for classification

Keep the canonical OCR operation but make its internal recognition result
distinguish at least:

- `language_unavailable`: preflight rejected the effective language;
- `zero_recognition`: the selected language was available, the OCR command
  completed the relevant input, and the canonical SeConv result explicitly
  indicates no recognized subtitle text; and
- `tool_or_input_failure`: process failure, malformed input, interrupted work,
  invalid output, or any result that is not the strict zero-recognition case.

The result carries the effective language, bounded image/input information when
available, command diagnostics, and the staged recognized cue output. User
messages are rendered from this structure so a missing-language failure does
not contain the full Batch Converter help/table output, while a genuine empty
recognition names the language actually attempted. Existing cancellation and
generic failure classification stays authoritative outside this recognition
classification.

Alternative considered: match any non-zero SeConv exit as zero recognition.
Rejected because damaged or empty PGS input and tool errors would be masked as
language problems and could trigger an unjustified orientation retry.

### 3. Retry only Japanese orientation after genuine zero recognition

Keep PGS extraction as a separate canonical preparation stage. For PGS, the
first recognition attempt writes/reads the extracted SUP from the existing
operation-owned OCR workspace; if it returns the structured genuine
zero-recognition outcome and the effective language is Japanese, the operation
checks the already probed capability set for the opposite orientation and may
run exactly one alternate recognition attempt against that same SUP. VobSub
uses the same rule against its already available bitmap input; no new input
extraction or sidecar discovery is introduced.

Each attempt uses distinct temporary recognition filenames within the same
workspace, and only the successful attempt's canonical cue result proceeds to
the existing cleanup and validation stages. The operation records both the
primary effective language and the language that ultimately succeeded, or both
attempted languages in the final concise failure. A missing primary language,
an unavailable alternate, a tool/input failure, cancellation, or a non-Japanese
language never enters this fallback branch.

Alternative considered: retry the whole bitmap OCR function including PGS
extraction. Rejected because it duplicates expensive extraction and makes it
harder to prove that only the orientation changed.

### 4. Preserve canonical single-result fan-out

Treat a successful recognition as a canonical normalized cue result plus
effective-language metadata. The existing SRT and ASS serializers consume that
result; a logical `both` request performs one recognition attempt sequence
(including at most the one Japanese fallback) and renders both formats from the
same successful result. Serializer calls cannot invoke OCR or independently
select an OCR language.

The fallback is therefore resolved before publication and before format fan-out.
It does not alter artifact identity or output naming, and it preserves the
existing shared-result behavior for text subtitles, non-Japanese bitmap
subtitles, and Japanese conversions that succeed on their first attempt.

Alternative considered: run OCR separately for SRT and ASS and let each
serializer retry. Rejected because it duplicates recognition, can select
different orientations, and violates the canonical `both` contract.

### 5. Keep runtime and diagnostic changes additive

Add the exact traineddata/runtime validation and focused diagnostics without a
database migration or a new persistent model. Preserve existing operation
status/error boundaries, staged artifact validation, progress semantics, and
sidecar reuse. If an artifact or operation response needs to expose the
successful language, add it as additive diagnostic metadata while retaining the
existing requested OCR language field and artifact identity.

## Risks / Trade-offs

- **[Risk]** Runtime Tesseract language listings can differ across supported
  architectures or custom images. **Mitigation:** validate exact identifiers in
  the built runtime and fail clearly at preflight rather than guessing from
  package metadata.
- **[Risk]** A Japanese fallback adds OCR cost for tracks that genuinely produce
  no text. **Mitigation:** limit it to Japanese, require strict zero
  recognition, probe availability once, and allow at most one alternate pass.
- **[Risk]** SeConv wording or exit behavior can vary by version. **Mitigation:**
  keep the zero-recognition classifier narrow, test the deployed SeConv version,
  and preserve bounded raw diagnostics for non-matching failures.
- **[Risk]** A fallback could accidentally re-extract PGS or duplicate OCR for
  `both`. **Mitigation:** test extraction invocation counts and serializer
  fan-out against a shared canonical result in disposable fixtures.
- **[Risk]** Installing additional Japanese traineddata increases image size and
  build time. **Mitigation:** install only the identifiers exposed by the
  product, pin the runtime dependency, and validate once during image build.

## Migration Plan

No database or Queue snapshot migration is required. Deploy the runtime image
and backend changes together so the runtime capability probe and exact
diagnostics are available wherever canonical bitmap OCR runs. Existing
non-Japanese conversions and already published sidecars remain unchanged.

If the runtime image does not expose a required Japanese language, the new
preflight reports that limitation without publishing an artifact; installing
the missing traineddata and retrying the same canonical conversion is the
recovery path. Rollback is code/image-only and does not remove source media,
existing sidecars, or completed artifacts.

## Open Questions

None that affect the contract or implementation sequence. The exact packaging
mechanism for `jpn_vert` can be selected during implementation after checking
the target Debian architecture, provided the built runtime passes the exact
`tesseract --list-langs` validation.
