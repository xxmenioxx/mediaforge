## Context

See proposal.md and the issue-30 requirements in `specs/subtitles/spec.md`.
The repository already has Pipeline subtitle planning, text extraction, and
bitmap OCR concerns, while Asset Info and other surfaces can select subtitle
outputs independently. The design must preserve source media, existing SRT
compatibility, and the distinction between original and converted artifacts.

## Goals / Non-Goals

**Goals:**

- Make one backend resolver/planner authoritative for source classification,
  mode, requested format, OCR need, stages, and artifact metadata.
- Reuse normalized OCR cues for both converted SRT and ASS outputs.
- Keep original ASS/SSA extraction as a stream-copy path.
- Make all entry points consume the same plan and report equivalent artifacts.

**Non-Goals:**

- Spatial reconstruction for bitmap OCR to ASS.
- Subtitle translation, cancellation, global defaults, or OCR runtime/preflight
  redesign beyond what is necessary to keep this contract working.
- Video conversion, unrelated Track Profile behavior, or publication policy.

## Decisions

1. **Extend the existing Pipeline resolver rather than add an Asset Info path.**
   The resolver returns a complete per-stream decision; callers only provide
   policy/input and consume the resulting artifact plan. A separate ASS helper
   was rejected because it would recreate precedence and naming rules.
2. **Represent converted output format explicitly.** SRT remains the default,
   while ASS is selected only by an explicit format request. Original ASS/SSA
   requests bypass normalization and use stream copy. Inferring mode from file
   extension alone was rejected because it loses the original-versus-converted
   distinction.
3. **Use one normalized cue model after OCR.** Bitmap OCR emits cues consumed by
   both SRT and ASS renderers. Rejecting bitmap streams before OCR was rejected
   because it prevents supported PGS/VobSub conversions.
4. **Persist and expose the resolved artifact plan.** Queue freezes it for worker
   execution; Asset Info, Profiles, and Lab use the same response metadata. This
   avoids duplicate extraction and lets only sidecars already identified by the
   canonical pipeline satisfy one transform. No new sidecar discovery or
   matching semantics are introduced.
5. **Treat legacy Queue snapshots as authoritative.** Snapshots created before
   the new subtitle fields exist continue to drive their jobs and retries; they
   are not silently re-resolved or rewritten during execution. Only newly
   created snapshots use the expanded canonical artifact plan.

## Risks / Trade-offs

- [Risk] OCR quality or availability differs by runtime → report explicit
  conversion failure and never fabricate an artifact.
- [Risk] ASS conversion is intentionally lossy relative to original styling →
  preserve original mode as the lossless option and label converted artifacts.
- [Risk] Existing clients assume SRT-only metadata → retain SRT defaults and
  additive format/mode fields for compatibility.
- [Risk] OCR and extraction add NAS workload → reuse planned artifacts, avoid
  duplicate sidecar work, and keep OCR scoped to bitmap conversion requests.

## Migration Plan

Deploy the resolver and caller changes together with focused regression tests.
Existing queued snapshots remain authoritative, including legacy snapshots that
predate the new subtitle fields; they are not silently re-resolved. New jobs
persist the canonical artifact plan. Rollback is code-only and leaves source
media and already published sidecars untouched.
