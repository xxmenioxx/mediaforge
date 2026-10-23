## Context

The canonical translation operation parses SRT/ASS cues, translates cue envelopes, calls `validateTranslationBatchResult`, serializes output, runs translated-sidecar validation, and publishes through the existing revision-fenced atomic barrier. The current operation response is the integration point for bounded quality evidence. OCR evidence is read-only and has a separate authority.

## Goals / Non-Goals

**Goals:** Add deterministic, explainable warning signals after structural validation; expose them with source/translation cue text for human review; support independently configured optional second opinion; persist provenance and decisions; allow explicit keep/edit review while preserving existing translation and publication authority.

**Non-Goals:** Change OCR evidence or its spec; add an autonomous grader or apply suggestions; add a provider runtime or publication pipeline; change cue IDs/timing/ASS layout.

## Decisions

- Place deterministic analysis after each canonical translated cue set has passed structural validation, using the canonical source/translated cue pairing. Signals are review prompts, never universal quality scores or publication blockers.
- Return bounded findings and counts through the existing translation operation result and persist bounded findings, source/output identity, reviewer model provenance, and explicit decisions in a translation-linked review row. Do not persist prompts, hidden reasoning, or raw model responses.
- Keep manual review tied to canonical cue identity and reuse existing validators, serializers, source revision fencing, staging, and Publisher. Provider output is never applied automatically. Persisted decisions are readable after restart.
- Load editable cue text on demand from the live translated sidecar through the existing review GET endpoint. The request is bound to the review and cue IDs; the backend revalidates source identity and all target revisions before and after reading, and rejects disagreeing SRT/ASS representations. The bounded finding text remains display-only.
- Capture the new target revision after promotion, then commit that revision and the human decision in one SQLite transaction at the Publisher's final publication checkpoint. If persistence fails, the existing Publisher compensates the file; its metadata rollback callback reconciles any commit-uncertain outcome. Rollback restores the destination mode and modification time so the previous revision remains valid; uncertain compensation marks the review recovery-required and disables further decisions.
- For manual SRT edits, validate parsed cues before and after canonical serialization: the count, order, timestamps, and unrelated cue text must match exactly, and SRT timing delimiters in edited text are rejected. Multiline cue text remains supported when those invariants hold.
- Read canonical OCR uncertainty only as distinct source evidence. It cannot increase translation confidence or be resolved by translation review.
- Do not call the existing #378 `Proofread` method as a translation second opinion: its contract lacks paired source and translated text. The optional reviewer uses the existing Ollama transport with a separately configured model, compares only deterministic flagged cue pairs, validates cue identity and response bounds, and returns advisory suggestions without applying them. Identical translator/reviewer model configuration fails closed.
- Recovery lists persisted review records by exact asset path and current source identity, so Asset Info can rediscover pending decisions after restart. Each edit reconstructs only the selected cue text through canonical SRT/ASS parsing and serialization, stages under the existing subtitle workspace, and calls the existing atomic Publisher. No durable edit staging is introduced.

## Risks / Trade-offs

- Heuristics can produce false positives, so findings remain advisory and expose their specific reasons.
- Human edits must not bypass canonical ASS/SRT serialization, source revision checks, or atomic publication. Publisher and review metadata are coupled so persistence failure cannot leave a successfully promoted sidecar paired with an obsolete stored revision.
- Reviewer model availability is not preloaded or inferred; an unavailable model leaves deterministic review available and reports a bounded unavailable state. Reviewer payload is limited to deterministic flagged cues.

## Migration Plan

AutoMigrate adds one translation-linked review model. Existing operations without quality evidence remain readable as not analyzed. New response fields and the review GET endpoint are backward compatible. OCR uncertainty remains read-only and distinct from quality review.
