## 1. Canonical quality and provenance contracts

- [ ] 1.1 Extend the canonical OCR evidence/cue contract with serializer-neutral quality findings and text-only correction metadata, preserving cue IDs, timing, order, tags, styles, geometry, and positioning; verify with focused canonical cue round-trip tests
- [ ] 1.2 Define versioned analyzer and repair provenance linked to canonical OCR identity, prepared-input provenance, OCR language/mode/runtime evidence, candidate source, validation outcome, and user/provider selection; verify unchanged evidence reuses valid internal records and changed provenance fails closed
- [ ] 1.3 Define bounded internal quality/revision records and backward-compatible reads without introducing Library-visible backup/raw artifacts; verify existing evidence and sidecars remain readable

## 2. Deterministic Quality Analyzer

- [ ] 2.1 Implement bounded lexical/dictionary evidence and known OCR-confusion detection over canonical cues; verify clean, suspicious, and review counts and cue-level reasons with deterministic analyzer tests
- [ ] 2.2 Integrate existing `accurate` candidate evidence into deterministic candidate scoring without format-specific OCR or a full OCR rerun; verify equivalent SRT, ASS, and `both` inputs produce the same scores and selected canonical candidate
- [ ] 2.3 Add safe handling for proper nouns, unknown words, insufficient evidence, and conflicting candidates; verify uncertain cues remain unchanged or reviewable rather than being silently rewritten
- [ ] 2.4 Make analysis read-only and reusable from canonical OCR evidence; verify `Analyze OCR` changes neither published sidecar bytes nor artifact metadata and does not start duplicate full OCR

## 3. Selective OCR repair

- [ ] 3.1 Reuse canonical prepared bitmap/PGS/VobSub evidence for suspicious cues and add bounded DVD/VobSub OCR-only preprocessing where eligible; verify ineligible cues do not trigger broad OCR or unbounded media analysis
- [ ] 3.2 Compare original, canonical/accurate, selective-secondary-OCR, and optional contextual-provider candidates using deterministic validation; verify providers can suggest text only, cannot change structure, and cannot perform translation
- [ ] 3.3 Record candidate, correction, runtime, and validation provenance and clean temporary repair workspace through the existing bounded cleanup path; verify terminal success, failure, and cancellation leave no Library-visible temporary files

## 4. Asset Info review and operation integration

- [ ] 4.1 Expose Asset Info `Analyze OCR` results with clean/suspicious/review counts, original-versus-suggestion details, reasons, and provenance through the existing subtitle operation model; verify focused backend/API and frontend review tests
- [ ] 4.2 Add keep, validated apply, manual text edit, and safe batch actions using the existing operation identity, progress, cancellation, and retry semantics; verify manual edits are text-only and uncertain findings are not batch-applied
- [ ] 4.3 Preserve subscriber/shared-flight ownership and canonical OCR cancellation behavior while adding quality work; verify one cancelled subscriber does not cancel another active equivalent request and last-subscriber cancellation cleans without committing evidence

## 5. Canonical serialization and publication

- [ ] 5.1 Render SRT and ASS only after corrected canonical cues exist, using one corrected cue set for logical `both`; verify corrected text parity, preserved ASS tags/styles/geometry/positioning, and SRT text/timing-only output
- [ ] 5.2 Validate every requested output before the existing atomic publication barrier and preserve the currently published sidecar on failure or cancellation; verify `both` never publishes only one newly corrected output
- [ ] 5.3 Keep `.bak`, `.raw`, and temporary staging internal and retain bounded revision/provenance suitable for future undo without adjacent media files; verify filesystem and publication metadata regression tests

## 6. Compatibility and performance regressions

- [ ] 6.1 Verify native ASS/SSA extraction, Japanese OCR fallback, bitmap positioning, existing OCR modes, and conversions without explicit repair remain unchanged through focused regression tests
- [ ] 6.2 Verify NAS-safe bounded lookup uses canonical/prepared-input identity rather than whole-media hashing and that repeated equivalent analysis reuses valid internal evidence without repeated expensive runtime probes
- [ ] 6.3 Verify cancellation, failed correction, successful retry, existing sidecar preservation, and internal workspace cleanup across text and bitmap subtitle paths

## 7. Validation

- [ ] 7.1 Run focused backend and frontend quality-analysis, correction, serializer, publication, cancellation, cleanup, and provenance tests and record the passing evidence
- [ ] 7.2 Run `./scripts/verify.sh --auto` and require `ALL CHECKS PASSED` before marking the implementation complete
