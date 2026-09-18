## 1. Canonical identity and evidence contract

- [ ] 1.1 Define the versioned canonical OCR identity and deterministic normalization for source identity/fingerprint, stream index, OCR language, OCR mode, source representation, and pipeline/runtime provenance; verify equivalent requests produce identical identities and every OCR-affecting change produces a different identity.
- [ ] 1.2 Define the serializer-neutral canonical OCR evidence payload containing cues, timing, source ordinals, optional bitmap geometry, requested/effective language, identity, provenance, completeness, and integrity metadata; verify malformed, incomplete, or mismatched evidence is rejected without affecting published sidecars.
- [ ] 1.3 Add the bounded internal evidence cache with atomic manifest/payload commits, explicit provenance validation, safe cache misses, and age/size retention; verify successful evidence is reusable internally while partial and stale entries are ignored or cleaned.

## 2. Shared canonical OCR lifecycle

- [ ] 2.1 Add synchronized in-flight identity coordination around the existing canonical bitmap OCR stage; verify equivalent concurrent requests produce one OCR execution and receive the same immutable evidence without creating a second planner or job system.
- [ ] 2.2 Route existing Asset Info, Queue, and other canonical subtitle callers through the lifecycle manager while preserving the existing logical `both` transform, canonical cue/geometry result, effective-language metadata, and serializer fan-out; verify SRT plus ASS consume one OCR result.
- [ ] 2.3 Implement mode-specific lifecycle semantics: raw serializes canonical recognized cues directly, clean cleans once then fans out, and accurate keeps internal passes private, selects one canonical result, and cleans once; verify raw, clean, and accurate publication and execution counts.
- [ ] 2.4 Remove public raw-intermediate publication from the lifecycle and keep raw OCR, accurate-pass, cleanup, and canonical temporary files in operation workspace or internal evidence only; verify no adjacent `.raw.srt` or `.raw.ass` is created beside media.

## 3. Workspace, publication, and cancellation integration

- [ ] 3.1 Preserve the existing operation-owned workspace and atomic publication barrier while serializing only requested final sidecars; verify temporary OCR workspace is cleaned after successful, failed, and cancelled terminal paths and existing sidecars are preserved.
- [ ] 3.2 Implement subscriber-aware cancellation for shared in-flight OCR using the existing operation context and child-process termination path; verify one cancelled consumer does not interrupt remaining consumers, while last-consumer cancellation terminates OCR and commits no evidence.
- [ ] 3.3 Preserve terminal cancellation/failure and retry semantics; verify cancelled or failed operations remain terminal, retries receive new operation identities, and retries either reuse matching valid evidence or execute fresh OCR when evidence was not committed.
- [ ] 3.4 Verify cancellation and completion continue to respect the existing all-output publication barrier for logical `both`, with no partial SRT/ASS publication and no destructive cleanup of prior completed sidecars.

## 4. Regression coverage and validation

- [ ] 4.1 Add focused concurrent lifecycle coverage for equivalent requests, SRT plus ASS reuse, logical `both`, cache reuse, provenance invalidation, and cache integrity rejection; verify OCR execution counts and canonical cue identity.
- [ ] 4.2 Add focused mode/publication coverage for raw, clean, and accurate outputs, including one cleanup for clean/accurate and requested-format-only publication; verify absence of adjacent `.raw.srt` and `.raw.ass` artifacts.
- [ ] 4.3 Add focused filesystem and lifecycle coverage for temporary workspace cleanup on success, failure, cancellation, and stale internal-cache entries; verify reusable evidence is retained only in the internal cache with explicit provenance.
- [ ] 4.4 Add focused shared-cancellation and retry coverage for cancellation while in-flight OCR is shared, cancellation after failure, retry identity creation, child-process termination, and no reuse of cancelled/failed evidence.
- [ ] 4.5 Run `openspec validate canonical-ocr-lifecycle --type change --strict` and verify the complete change artifacts pass strict validation.
- [ ] 4.6 Run `./scripts/verify.sh --auto` and verify the repository reports `ALL CHECKS PASSED`.
