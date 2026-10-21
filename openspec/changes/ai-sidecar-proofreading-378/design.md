## Context

The existing Asset Info workflow already resolves exact external subtitle sources and captures content revisions for translation. Subtitle OCR review has a cancellable operation lifecycle, deterministic cue evidence, and explicit approval. ASS/SSA parsing preserves protected dialogue controls, and the shared Ollama provider reads the configured Translation model. See proposal.md and `specs/subtitle-proofreading/spec.md` for the user-facing contract.

## Goals / Non-Goals

**Goals:**
- Keep backend ownership, source revision, cue identity, provider validation, and publication authoritative.
- Reuse existing subtitle operation status/cancellation, source parsers, shared provider connection, OCR evidence, and internal revision infrastructure where compatible.
- Make Quick review selective and Extended review explicitly bounded.
- Keep AI findings advisory and operator decisions explicit.

**Non-Goals:**
- New OCR, translation, semantic-fidelity, media-context, provider-connection, or publication pipelines.
- Reviewer model configuration or model download behavior in V1.
- Editing embedded bitmap subtitles or modifying global Translation Settings.

## Decisions

### Ownership map

| Decision | Canonical owner |
| --- | --- |
| Asset and sidecar ownership, supported format, source language metadata, and exact revision | Backend Asset Info source resolver and subtitle revision utilities |
| SRT cue parsing and ASS/SSA structure/protected text | Existing canonical subtitle parsers and serializers |
| OCR-origin findings | Existing canonical OCR quality analyzer and evidence identity |
| Provider endpoint, configured model, readiness, timeout, and connection | Shared Ollama/Translation provider settings |
| Proofreading-specific task and response contract | Small adapter over the shared provider connection |
| Operation status and cancellation | Existing subtitle operation lifecycle |
| Selection, decision capture, confirmation, and presentation | Asset Info review dialog |
| Stale-source checks, prior revision retention, atomic apply/restore | Backend canonical sidecar revision/publication boundary |

### Data flow and state

The backend resolves an exact eligible external text sidecar and captures its path-bound content revision before parsing. It derives source type only from canonical evidence: compatible OCR snapshot, validated translation artifact metadata, external/imported inventory, or unknown when evidence conflicts. It reuses language metadata and exposes uncertainty rather than guessing. Quick mode runs deterministic checks first; only flagged cues and a small fixed count of adjacent cue excerpts enter the model request. Extended mode sends bounded sequential chunks with a strict cap. Both modes return cue ID, source hash, candidate text, reason code, category, model identity, contract version, and advisory confidence.

The frontend shows summary and filtered findings. Decisions stay local until the operator reviews the exact selected diff and confirms. Apply re-resolves ownership, re-reads the full revision, validates every selected cue/hash, language and format boundary, stages a complete replacement, preserves the prior revision and concise decisions internally, then atomically publishes. Restore is another explicit revision-checked atomic publication. Cancellation and all failures before commit retain the current sidecar.

### Provider contract and limits

The adapter uses the configured Translation model by default over the existing Ollama connection with a separate proofreader instruction and strict JSON response schema. V1 does not add a reviewer-model setting. The UI identifies that a Translation model reviewing its own output is not independent verification. Quick and Extended use the same provider contract. Request bytes, response bytes, cue count per chunk, neighbor count, total cues, elapsed time, and concurrent operations are bounded; there are no per-cue calls or retries without operator action. Credentials, prompts, and cue payloads are excluded from durable provenance.

### Revision and compatibility

The exact external sidecar is the publication target; the backend does not infer a replacement path. Apply preserves a restorable preimage in the existing internal subtitle revision area (generalized only as needed) rather than creating adjacent backup sidecars. Publication stages beside the target and uses the shared `publishStagedSubtitleFile` primitive under the existing subtitle publication lock, after revision revalidation. Proofreading-specific revision manifests record the approval provenance without introducing another file-promotion mechanism. Existing OCR approval, translation, sidecar editing, and extraction APIs retain their current contracts. No database migration or new dependency is required.

## Risks / Trade-offs

- [Proofreading can misread intentional dialogue or unfamiliar names] → Keep suggestions pending, show source-accurate reasons and context, and require operator approval.
- [Large ASS files may exceed memory or request limits] → Reuse bounded source reads, cap parsed cues/chunk bytes, and fail closed before provider access when limits are exceeded.
- [Sidecar changes can race with analysis] → Bind every report and decision to a content revision and reject stale apply atomically.
- [One configured model may review its own translation] → Surface model identity and the lack of independent verification; keep semantic fidelity out of scope.
- [Generic sidecars lack OCR evidence and structured lineage] → Report imported/external or unknown from canonical evidence and do not synthesize OCR or translation provenance.

## Migration Plan

No persistent schema migration is planned. Existing sidecars and operation records remain readable. Rollback consists of disabling the new action/API while retaining internal prior revisions; explicit restore remains available for any already-applied edit.

## Open Questions

None. V1 intentionally uses only the configured Translation model; a separately configured reviewer can be added only through a later explicit settings contract.
