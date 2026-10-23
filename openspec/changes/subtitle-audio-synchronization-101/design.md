## Context

See `proposal.md` and the added requirements in `specs/subtitles/spec.md`. The current Asset Info subtitle path already resolves exact external sidecar ownership and revisions, and it has a cancellable in-process operation registry with private staging and guarded publication. The subtitle implementation also has protected ASS/SSA event parsing used by translation; the transform must preserve source bytes outside timing fields.

## Goals / Non-Goals

**Goals:**

- Ship a complete manual fixed-offset path for an exact external text sidecar.
- Require an explicit audio stream selected from the media asset and bind proposals to stable source/audio identities.
- Offer a bounded, review-only automatic fixed-offset proposal for SRT when the backend runtime exposes its limiter and FFmpeg.
- Keep proposal generation read-only and make Apply a separate, cancellable operation.
- Preserve all source bytes except SRT cue timestamps or ASS/SSA Dialogue Start/End fields.
- Persist bounded applied provenance without adding user-visible backup files.

**Non-Goals:**

- Drift correction, segmented alignment, and automatic ASS/SSA alignment.
- Syncing embedded subtitle streams directly; the first implementation targets standalone sidecars so the original embedded stream is never rewritten.
- OCR, translation, Track Profile or Library-wide automation, bitmap subtitles, and client subtitle selection.
- Introducing an external runtime dependency before its current command contract, license, NAS cost, and container availability are verified.

## Decisions

1. **Keep synchronization behind a reusable backend engine contract.** The manual fixed-offset engine accepts a manually selected offset. A separate bounded audio-activity engine can propose one fixed offset for SRT and returns the same canonical proposal shape with confidence and review state. Affine and segmented transforms remain unavailable; a manual offset never fabricates automatic confidence.

2. **Resolve only exact owned external SRT/ASS/SSA sidecars in V1.** Reuse the existing Asset Info ownership resolver, canonical subtitle inventory, bounded content reader, and source revision functions. Embedded sources need a canonical lossless extraction before a standalone artifact can be synchronized; that extra artifact planning step is deferred rather than mutating the media container.

3. **Use a timing-only source-span transform.** For SRT, retain cue text, separators, numbering, settings, and line endings while replacing parsed timestamp spans. For ASS/SSA, use the protected document parser and replace only Start/End spans on Dialogue events; do not round-trip through SRT or reconstruct unrelated document sections. Reparse the result and verify cue count, order, interval validity, and exact non-timing content before publication.

4. **Use audio identity as explicit proposal evidence.** Resolve the requested global stream index from the source's current ffprobe result and capture stream ID, codec, language, and title. The fixed-offset engine does not decode audio; the user supplied the correction while this explicit track records the reference selected for the decision. Recheck the stream identity before applying.

5. **Separate read-only proposal from Apply.** Preview returns a bounded sample of current and proposed cue times, source/audio identity, model, offset, and engine version. Apply re-resolves identities and recomputes the transformation from the captured request; it never trusts frontend cue data or rewrites content from the preview.

6. **Extend the existing subtitle operation lifecycle.** Register Apply in the current operation registry and reuse its cancellation endpoint, terminal states, and polling. Stage beside the target only long enough to keep publication on the same filesystem; write through the existing subtitle publication lock, recheck source revision immediately before replacement, and remove the operation-owned workspace on every terminal path.

7. **Persist only compact applied provenance.** Add one SQLite record per successful synchronization with source revision, selected audio identity, engine/model version, transform, quality state, timestamps, and operation ID. Do not persist cue text, raw engine logs, or credentials. The previous source remains in operation-owned staging only until publication and metadata commit have succeeded; no `.bak` file is exposed beside the subtitle.

8. **Keep automatic analysis dependency-free and bounded.** The optional SRT engine decodes only the selected audio stream through the bundled Linux limiter and FFmpeg, downmixes to mono 8 kHz, and streams PCM into 100 ms RMS activity windows without retaining raw audio. It rejects subtitles beyond three hours, serializes analysis to one slot, caps child address space at 512 MiB and CPU at 90 seconds, applies a 120-second wall timeout, limits FFmpeg to one decode/filter thread and a 128 MiB single-allocation ceiling, and caps decoded output to the requested duration. Only a strong, separated cue/activity correlation produces an `automatic_review` fixed offset; ambiguous or low-confidence results are review-only and Apply rejects them. Runtime capability is backend-authoritative and requires Linux, the limiter, and FFmpeg; manual fixed offset remains available if any are absent. Complex drift, segmented alignment, and automatic ASS/SSA remain unavailable. No external alignment dependency is added. The reviewed ffsubsync MIT CLI does not provide the process resource boundary used here, while ALASS carries GPL-3.0; neither is included. Sources reviewed: [ffsubsync README](https://github.com/smacke/ffsubsync/blob/master/README.md), [ffsubsync CLI reference](https://subsync.readthedocs.io/en/latest/cli.html), [ffsubsync setup metadata](https://github.com/smacke/ffsubsync/blob/master/setup.py), [ALASS README](https://github.com/kaegi/alass/blob/master/README.md), [ALASS license](https://github.com/kaegi/alass/blob/master/LICENSE), and [FFmpeg command-line documentation](https://www.ffmpeg.org/ffmpeg.html).

## Risks / Trade-offs

- **[Risk]** A manual offset can be wrong even when the user selects an audio track. **Mitigation:** label the model as manual, show the exact reference stream and timing preview, and require explicit Apply.
- **[Risk]** ASS timestamps have centisecond precision while the UI may accept milliseconds. **Mitigation:** constrain ASS/SSA offset input to representable centiseconds and report the effective offset.
- **[Risk]** Replacing a subtitle and inserting provenance span a filesystem and SQLite boundary. **Mitigation:** keep a private operation-owned copy for rollback until the provenance insert succeeds, serialize in-process publication, and surface recovery errors without leaving adjacent temporary files.
- **[Risk]** A source can change between analysis and apply. **Mitigation:** use the existing content-bound revision and recheck it during the publication critical section.
- **[Trade-off]** V1 does not infer alignment automatically and does not operate on embedded tracks. The engine/proposal contract and explicit source identities keep those later additions on the same path. The external-engine review is preliminary and does not include NAS performance measurement or a license determination for ALASS integration.

## Migration Plan

AutoMigrate adds the compact provenance table without changing existing subtitle records. The new API and Asset Info action are additive. Rollback leaves the source subtitle intact and may retain completed provenance rows for already applied results; it does not remove prior sidecars or media.
