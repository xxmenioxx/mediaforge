## 1. Canonical quality and provenance contracts

- [x] 1.1 Extend the canonical OCR evidence/cue contract with serializer-neutral quality findings and text-only correction metadata, preserving cue IDs, timing, order, tags, styles, geometry, and positioning; verify with focused canonical cue round-trip tests
- [x] 1.2 Define versioned analyzer, repair, and accurate-candidate-scoring provenance linked to canonical OCR identity, prepared-input provenance, OCR language/mode/runtime evidence, candidate source, validation outcome, and user/provider selection; verify unchanged evidence reuses valid internal records and changed scorer/provenance fails closed
- [x] 1.3 Persist a bounded published canonical OCR cue snapshot containing serializer-neutral cue/provenance/integrity data while keeping the seven-day/256 MiB #6 execution cache evictable and prepared bitmaps out of durable storage; verify analysis still works after execution-cache eviction
- [x] 1.4 Define bounded published SRT/ASS revision identity including artifact/path identity, size, mtime, full content hash, and logical `both` membership; verify revision records are backward-compatible and stale records fail closed
- [x] 1.5 Add legacy OCR fallback that reconstructs only text evidence from MVForge-owned SRT/ASS when provenance is sufficient; verify missing provenance does not trigger full OCR or complete-media scanning
- [x] 1.6 Commit the corrected canonical cue snapshot, new published revision, and correction lineage only with successful atomic publication; verify revision R advances to one new baseline, prior findings become stale, subsequent analysis sees corrected text, logical `both` advances one baseline, and commit failure leaves publication/baseline state consistent
- [x] 1.7 Define snapshot ownership, replacement, artifact-removal invalidation, source-identity fail-closed behavior, bounded provenance retention, and orphan cleanup; verify cleanup never deletes published sidecars or creates Library-visible artifacts

## 2. Deterministic Quality Analyzer

- [x] 2.1 Implement bounded lexical/dictionary evidence and known OCR-confusion detection over canonical cues; verify clean, suspicious, and review counts and cue-level reasons with deterministic analyzer tests
- [x] 2.2 Replace/extend the structural-only `accurate` score with the versioned deterministic structural, replacement/confusion, supported-language lexical, dictionary, and bounded plausibility contract; verify a valid-looking OCR error can change candidate choice only when deterministic evidence supports it and selection occurs before cleanup/serialization
- [x] 2.3 Add safe handling for proper nouns, unknown words, insufficient evidence, and conflicting candidates; verify uncertain cues remain unchanged or reviewable rather than being silently rewritten
- [x] 2.4 Make analysis read-only and reusable from the published cue snapshot or still-valid canonical evidence; verify `Analyze OCR` works after #6 cache eviction, changes neither published sidecar bytes nor artifact metadata, and does not start duplicate full OCR

## 3. Selective OCR repair

- [x] 3.1 Gate selective secondary OCR on a bounded supported cue-input mechanism such as prepared representation, persisted locator, or safe extraction; verify eligible DVD/VobSub cues can be processed while unavailable input leaves text unchanged/reviewable without full OCR, complete-media hashing, or heuristic scanning
- [x] 3.2 Compare original, canonical/accurate, selective-secondary-OCR, and optional contextual-provider candidates using deterministic validation; verify providers receive only suspicious/review candidates in bounded batches, propagate operation cancellation, can suggest text only, cannot change structure, and cannot perform translation
- [x] 3.3 Record candidate, correction, runtime, and validation provenance and clean temporary repair workspace through the existing bounded cleanup path; verify terminal success, failure, and cancellation leave no Library-visible temporary files

## 4. Asset Info review and operation integration

- [x] 4.1 Expose Asset Info `Analyze OCR` results with clean/suspicious/review counts, original-versus-suggestion details, reasons, and provenance through the existing subtitle operation model; verify focused backend/API and frontend review tests
- [x] 4.2 Add keep, validated apply, manual text edit, and safe batch actions using the existing operation identity, progress, cancellation, and retry semantics; verify manual/individual/batch actions check the recorded published revision before staging, manual edits are text-only, and uncertain findings are not batch-applied
- [x] 4.3 Preserve subscriber/shared-flight ownership and canonical OCR cancellation behavior while adding quality work; verify one cancelled subscriber does not cancel another active equivalent request and last-subscriber cancellation cleans without committing evidence

## 5. Canonical serialization and publication

- [x] 5.1 Render SRT and ASS only after corrected canonical cues exist, using one corrected cue set for logical `both`; verify corrected text parity, preserved ASS tags/styles/geometry/positioning, and SRT text/timing-only output
- [x] 5.2 Validate every requested output before the existing atomic publication barrier and recheck the complete published revision immediately before commit; verify external edits and concurrent operation A/B publication cause stale correction failure, preserve the current sidecar, and prevent `both` from publishing only one newly corrected output
- [x] 5.3 Keep `.bak`, `.raw`, and temporary staging internal and retain bounded revision/provenance suitable for future undo without adjacent media files; verify filesystem and publication metadata regression tests
- [x] 5.4 Enforce legacy representation boundaries; verify SRT-only reconstructed baselines can correct only SRT, cannot silently synthesize ASS geometry or become a `both` source, legacy ASS uses only safely parsed structure, and missing evidence routes to canonical subtitle conversion

## 6. Compatibility and performance regressions

- [x] 6.1 Verify native ASS/SSA extraction, Japanese OCR fallback, bitmap positioning, existing OCR modes, and conversions without explicit repair remain unchanged through focused regression tests
- [x] 6.2 Verify NAS-safe bounded lookup uses canonical/prepared-input identity rather than whole-media hashing, respects the seven-day/256 MiB execution-cache policy, uses the durable cue snapshot after eviction, and avoids repeated expensive runtime probes
- [x] 6.3 Verify cancellation, failed correction, successful retry, external sidecar edits, concurrent correction races, existing sidecar preservation, and internal workspace cleanup across text and bitmap subtitle paths
- [x] 6.4 Add lifecycle regressions for post-correction baseline advancement, corrected-text reanalysis, stale prior findings, logical `both` baseline lineage, artifact removal, ownership changes, and orphan snapshot cleanup

## 7. Validation

- [x] 7.1 Run focused backend and frontend quality-analysis, accurate-scoring, snapshot/cache, selective-input capability, revision-conflict, provider batching/cancellation, serializer, publication, cancellation, cleanup, and provenance tests and record the passing evidence
- [x] 7.2 Run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED` before marking the implementation complete
