## Context

See [proposal.md](proposal.md) for the motivation and scope. The current OCR path already binds human decisions to exact cue text and source/artifact revisions, commits corrections through one publication barrier, and stores contextual dictionary provenance. The memory must consume only that committed decision boundary.

## Goals / Non-Goals

**Goals:**

- Keep memory separate from Translation Memory and the contextual OCR dictionary.
- Reuse existing correction operations and canonical OCR snapshots; memory is a retrieval/evidence layer only.
- Keep storage, queries, and prompt enrichment bounded for NAS-hosted deployments.
- Make memory settings and entries manageable from the subtitles settings surface.

**Non-Goals:**

- Automatic correction, dictionary rule creation, provider fine-tuning, or OCR pipeline replacement.
- Retaining images, complete subtitle files, or full prompts.
- Inferring title/series identity from a media filename. Scopes may use stable asset/library/group identities only when the existing inventory resolves them.

## Decisions

### Capture after the canonical correction commit

Capture only from the existing subtitle OCR correction operation after the artifact/snapshot publication barrier has succeeded. Source text comes from the pre-correction canonical cues and corrected text from the validated final cues. Only a human `edit` or explicit user `apply` of a contextual dictionary candidate is eligible; approval-only, pending, automatic dictionary preparation, and unconfirmed AI results are excluded. A failed memory write is surfaced as a capture warning without misreporting an already committed subtitle correction as failed.

**Alternative considered:** collect examples during analysis or when a provider returns a candidate. Rejected because neither event establishes user approval or a committed correction.

### Separate normalized examples from immutable evidence

Use one bounded entry row for the deduplicated language/text/context/scope example, plus bounded evidence rows for each approved source/artifact revision. Evidence preserves the first and later provenance without retaining media paths or cue bitmaps. A unique entry fingerprint merges duplicate approvals; evidence is deduplicated by entry, source revision, artifact revision, and cue ID. New conflicting approved text can supersede an older active entry in the same exact language/scope/context slot, with both rows retained for review and revocation.

**Alternative considered:** add corrections to the OCR dictionary. Rejected because examples are approval evidence and must not become global replacement rules.

### Resolve scope from managed identities and default conservatively

Use exact OCR source language, stable `AssetRecord` identity, and `LibraryID` when available. Capture defaults to the narrowest resolved group scope (series or title), then library, and falls back to asset when inventory does not resolve a broader managed identity. Users may deliberately narrow or broaden scope through the manager. Global scope requires an explicit user choice. Asset-scoped retrieval checks the recorded source identity. For broader scopes, if inventory resolves the same asset but its source revision differs from the stored evidence, exclude that example until it is captured again; other assets in the resolved group can receive it as an advisory suggestion.

**Alternative considered:** infer title/series from filenames or parent names. Rejected because naming conventions are not reliable identity authority and can silently leak corrections between unrelated assets.

### Deterministic bounded retrieval, optional prompt use

Return at most three same-language suggestions per finding. Rank exact text plus context fingerprint, exact text, then conservative normalized-token similarity, with scope specificity breaking ties. Every result remains advisory and carries entry/source provenance. No model is loaded for retrieval. Prompt enrichment is a separate setting and contributes at most four short approved examples to the existing optional OCR AI request; OCR AI remains off and memory prompt use remains off by default.

**Alternative considered:** retrieve only during AI inference. Rejected because deterministic review suggestions must remain usable without a model and must not incur inference merely to search local examples.

### Retention and bounded data

Store original/corrected cue text only up to a fixed per-field limit, hash a bounded normalized previous/current/next cue context, cap entries/evidence/page size, and expire rows according to the configured retention period. Prune expired memory rows on bounded memory operations, never as a side effect of changing the learning toggle. Settings validation restricts retention to a supported range.

**Alternative considered:** persist complete neighboring cues for later fuzzy matching. Rejected because it retains unnecessary transcript content; a fingerprint is sufficient for exact-context ranking.

## Risks / Trade-offs

- [A broader scope can reuse a valid correction in unrelated media] → Require explicit scope changes, exact language matching, visible provenance, bounded similarity, and user confirmation.
- [Memory DB write can fail after subtitle publication] → Preserve the committed artifact state, report the capture warning, and keep the correction retry safe through evidence deduplication.
- [Old examples become irrelevant after source changes] → Bind asset-level lookup to source identity and expire entries; edits to stored text invalidate approval.
- [SQLite growth can be unbounded under repeated approvals] → Cap rows/evidence, dedupe examples, prune by retention, and enforce per-operation page and suggestion limits.

## Migration Plan

1. AutoMigrate the new entry/evidence tables and create the independent memory settings key with learning and prompt enrichment off by default.
2. Do not backfill entries from existing OCR snapshots or dictionary rules; only new committed user approvals are eligible.
3. Rollback can leave the additive tables/settings in place; disabling learning prevents new capture/retrieval without deleting user data. Explicit export/delete controls remain available.
