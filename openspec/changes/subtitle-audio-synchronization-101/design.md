## Context

See `proposal.md` and the added requirements in `specs/subtitles/spec.md`. The current Asset Info subtitle path already resolves exact external sidecar ownership and revisions, and it has a cancellable in-process operation registry with private staging and guarded publication. The subtitle implementation also has protected ASS/SSA event parsing used by translation; the transform must preserve source bytes outside timing fields.

## Goals / Non-Goals

**Goals:**

- Ship a complete manual fixed-offset path for an exact external text sidecar.
- Require an explicit audio stream selected from the media asset and bind proposals to stable source/audio identities.
- Keep proposal generation read-only and make Apply a separate, cancellable operation.
- Preserve all source bytes except SRT cue timestamps or ASS/SSA Dialogue Start/End fields.
- Persist bounded applied provenance without adding user-visible backup files.

**Non-Goals:**

- Decoding/analyzing audio for automatic alignment in this first implementation.
- Syncing embedded subtitle streams directly; the first implementation targets standalone sidecars so the original embedded stream is never rewritten.
- OCR, translation, Track Profile or Library-wide automation, bitmap subtitles, and client subtitle selection.
- Introducing an external runtime dependency before its current command contract, license, NAS cost, and container availability are verified.

## Decisions

1. **Keep synchronization behind a reusable backend engine contract.** A fixed-offset engine accepts a manually selected offset and returns an engine-neutral proposal. The same proposal model can represent affine and segmented transforms, but runtime capability reports those modes unavailable until a real adapter is installed. Fabricating an automatic proposal from a manual input was rejected.

2. **Resolve only exact owned external SRT/ASS/SSA sidecars in V1.** Reuse the existing Asset Info ownership resolver, canonical subtitle inventory, bounded content reader, and source revision functions. Embedded sources need a canonical lossless extraction before a standalone artifact can be synchronized; that extra artifact planning step is deferred rather than mutating the media container.

3. **Use a timing-only source-span transform.** For SRT, retain cue text, separators, numbering, settings, and line endings while replacing parsed timestamp spans. For ASS/SSA, use the protected document parser and replace only Start/End spans on Dialogue events; do not round-trip through SRT or reconstruct unrelated document sections. Reparse the result and verify cue count, order, interval validity, and exact non-timing content before publication.

4. **Use audio identity as explicit proposal evidence.** Resolve the requested global stream index from the source's current ffprobe result and capture stream ID, codec, language, and title. The fixed-offset engine does not decode audio; the user supplied the correction while this explicit track records the reference selected for the decision. Recheck the stream identity before applying.

5. **Separate read-only proposal from Apply.** Preview returns a bounded sample of current and proposed cue times, source/audio identity, model, offset, and engine version. Apply re-resolves identities and recomputes the transformation from the captured request; it never trusts frontend cue data or rewrites content from the preview.

6. **Extend the existing subtitle operation lifecycle.** Register Apply in the current operation registry and reuse its cancellation endpoint, terminal states, and polling. Stage beside the target only long enough to keep publication on the same filesystem; write through the existing subtitle publication lock, recheck source revision immediately before replacement, and remove the operation-owned workspace on every terminal path.

7. **Persist only compact applied provenance.** Add one SQLite record per successful synchronization with source revision, selected audio identity, engine/model version, transform, quality state, timestamps, and operation ID. Do not persist cue text, raw engine logs, or credentials. The previous source remains in operation-owned staging only until publication and metadata commit have succeeded; no `.bak` file is exposed beside the subtitle.

8. **Defer automatic-engine selection.** The ffsubsync upstream README and CLI reference document the `ffs <media> -i <input> -o <output>` command and FFmpeg-backed audio reference; the package declares MIT and publishes through PyPI and GHCR. Its CLI parser recognizes subtitle extensions, but its documented CLI labels inputs and outputs as SRT and does not establish byte-preserving ASS/SSA output, so automatic synchronization is limited to SRT if a future adapter is approved. ALASS upstream documents `alass <media> <input.srt> <output.srt>`, `.srt`/`.ssa`/`.ass` support, and FFmpeg/ffprobe use; its repository carries GPL-3.0 and is not included. Neither upstream publishes enforceable CPU/memory limits for MVForge's NAS workloads, and no supported-NAS profile has been measured. Therefore automatic and segmented alignment remain unavailable with the backend reason `Automatic alignment is disabled until resource use is bounded for supported NAS deployments.` Fixed offset remains available without either dependency. Sources reviewed: [ffsubsync README](https://github.com/smacke/ffsubsync/blob/master/README.md), [ffsubsync CLI reference](https://subsync.readthedocs.io/en/latest/cli.html), [ffsubsync setup metadata](https://github.com/smacke/ffsubsync/blob/master/setup.py), [ALASS README](https://github.com/kaegi/alass/blob/master/README.md), and [ALASS license](https://github.com/kaegi/alass/blob/master/LICENSE).

## Risks / Trade-offs

- **[Risk]** A manual offset can be wrong even when the user selects an audio track. **Mitigation:** label the model as manual, show the exact reference stream and timing preview, and require explicit Apply.
- **[Risk]** ASS timestamps have centisecond precision while the UI may accept milliseconds. **Mitigation:** constrain ASS/SSA offset input to representable centiseconds and report the effective offset.
- **[Risk]** Replacing a subtitle and inserting provenance span a filesystem and SQLite boundary. **Mitigation:** keep a private operation-owned copy for rollback until the provenance insert succeeds, serialize in-process publication, and surface recovery errors without leaving adjacent temporary files.
- **[Risk]** A source can change between analysis and apply. **Mitigation:** use the existing content-bound revision and recheck it during the publication critical section.
- **[Trade-off]** V1 does not infer alignment automatically and does not operate on embedded tracks. The engine/proposal contract and explicit source identities keep those later additions on the same path. The external-engine review is preliminary and does not include NAS performance measurement or a license determination for ALASS integration.

## Migration Plan

AutoMigrate adds the compact provenance table without changing existing subtitle records. The new API and Asset Info action are additive. Rollback leaves the source subtitle intact and may retain completed provenance rows for already applied results; it does not remove prior sidecars or media.
