---
name: mvforge-subtitle-change
description: Implement or review MVForge subtitle changes while preserving the canonical subtitle/OCR/translation pipeline, cue identity, ASS structure, cancellation, provenance, and safe publication. Use for SRT, ASS/SSA, PGS, VobSub, OCR, translation, styling, positioning, extraction, or subtitle sidecar work.
license: MIT
metadata:
  project: MVForge
  version: "1.0"
---

# MVForge Subtitle Change

Use this skill together with the repository's applicable `AGENTS.md` and OpenSpec artifacts.

## First principles

- Reuse the canonical subtitle pipeline.
- Do not create a second OCR, translation, extraction, serialization, cancellation, or publication path.
- Preserve source media and source subtitle artifacts.
- Keep cue identity, timing, language, role, and provenance explicit.
- Fail closed on malformed or incomplete transformations.
- Prefer one canonical cue representation before format-specific serialization.

## Investigation

Use Serena MCP to locate:
- the canonical planner/resolver;
- the affected operation model;
- serializer/parser;
- OCR or translation integration;
- publication/staging path;
- directly relevant tests.

Do not read large handler files end-to-end if symbols are known.

## Text subtitles

For SRT/ASS/SSA:
- preserve source timing unless the approved change explicitly performs sync;
- keep format conversion separate from semantic transformation;
- preserve encoding/newline handling through canonical utilities;
- do not overwrite the source sidecar by default.

## ASS / SSA

When ASS/SSA is involved:
- preserve protected tags, positioning, styles, attachments/font dependencies, and role/layout metadata where required;
- do not flatten native ASS styling unless explicit behavior allows it;
- generated styling must go through the canonical ASS generation path;
- SRT fallback remains independent from authoritative ASS output.

## Bitmap subtitles

For PGS/VobSub:
- reuse canonical OCR evidence and lifecycle;
- do not rerun OCR when valid canonical evidence exists;
- preserve geometry/position evidence when available;
- distinguish OCR recognition from cleanup, translation, styling, and publication.

## Translation

When translation is involved:
- source cues remain authoritative;
- preserve cue IDs/count and timings;
- provider output must normalize back to the canonical contract;
- bounded context may include neighboring cues, media metadata, glossary/TM, and aligned reference evidence when supported;
- advisory references must not silently replace source meaning;
- provider failure must not publish partial output.

## Cancellation and publication

- propagate cancellation through active work;
- stage before publication;
- publish atomically;
- clean only operation-owned temporary artifacts;
- preserve existing sidecars unless replacement is explicitly requested.

## Tests

Add focused coverage for the changed invariant, including malformed/failure cases when relevant.

Then run:

```bash
./scripts/verify.sh --auto
```

Follow repository completion rules in `AGENTS.md`.

## Review checklist

Before completion verify:
- no parallel subtitle path was introduced;
- source untouched;
- cue/timing contract preserved;
- ASS structure preserved when applicable;
- OCR reuse remains canonical;
- cancellation works;
- publication is atomic/non-destructive;
- provenance remains explainable.
