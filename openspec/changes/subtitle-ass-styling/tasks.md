## 1. Canonical style/settings model

- [ ] 1.1 Define the backend `SubtitleStylePreset` contract, immutable built-in IDs, collision-safe custom identity, provenance, and versioned `subtitleSettings.subtitleStyling`; verify missing settings resolve to safe defaults and existing fallback fields remain compatible.
- [ ] 1.2 Add normalized style-policy snapshot/version hashing and carry it through canonical resolved subtitle decisions, operation requests, Queue freeze metadata, and bounded artifact provenance; verify Settings edits do not affect a running/frozen operation.
- [ ] 1.3 Add focused backend tests for built-in immutability, custom ID/display-name independence, missing-setting defaults, backward-compatible reads, unknown-version safe fallback, and atomic invalid updates.

## 2. Validation and color conversion

- [ ] 2.1 Implement the backend-owned HEX validator/normalizer and ASS alpha/BGR conversion helper with explicit `00=opaque` and `FF=transparent` semantics; verify uppercase/lowercase, three-digit expansion, invalid rejection, opacity bounds, and round trips.
- [ ] 2.2 Add bounded palette normalization for the fixed palette, saved colors (maximum 32), and recent colors (maximum 12, newest-first); verify case-insensitive deduplication, deterministic eviction, and no unbounded AppSetting growth.
- [ ] 2.3 Add focused settings/color tests proving primary, outline, and shadow/back colors cannot emit malformed ASS color values.

## 3. Role/style resolver

- [ ] 3.1 Implement one canonical role-to-style resolver that consumes existing semantic roles and layout/geometry evidence without creating a second classifier; verify `signs`, configured mappings, explicit style selection, unknown fallback, and semantic-role precedence over geometry.
- [ ] 3.2 Keep style, role, alignment, and exact position separate in the canonical cue/document flow; verify valid `\\pos`, `\\move`, `\\an`, margins, and geometry evidence remain intact when style alignment is resolved.
- [ ] 3.3 Add deterministic resolver tests for dialogue, top policy hint, lyrics, sign, unknown, semantic-role/geometry disagreement, and explicit-position preservation.

## 4. ASS serializer integration

- [ ] 4.1 Extend the existing generated ASS rendering seam (`renderBitmapASSCues` or its shared canonical equivalent) to consume resolved style definitions while preserving the existing canonical cue/OCR evidence boundary; verify generated ASS has deterministic style definitions and event assignments.
- [ ] 4.2 Route generated SRT-to-ASS output through the same canonical style-aware ASS seam without adding a second subtitle cue model or serializer; verify SRT remains text/timing-only.
- [ ] 4.3 Preserve logical `both` fan-out so one text normalization/OCR result feeds unchanged SRT and styled ASS serializers; verify OCR invocation/evidence identity is single-use.
- [ ] 4.4 Add focused generated-output tests for styled SRT-to-ASS, bitmap OCR-to-ASS with geometry, and SRT/ASS `both` output parity.

## 5. Native source preservation and collision handling

- [ ] 5.1 Integrate the style policy with the existing ASS/SSA source-preserving parser/reconstructor; verify native style blocks, event references, representation, karaoke, inline tags, positions, margins, layers, drawing mode, text, timing, and order are unchanged by default.
- [ ] 5.2 Implement explicit override as eligible event-style assignment to deterministic namespaced generated styles while preserving source definitions and structural/inline tags; verify override requires the policy flag and does not rewrite protected fields.
- [ ] 5.3 Add collision regressions for source styles named `MVF_Default`/`MVF_Top`, different configured definitions, duplicate visible names, and deterministic generated naming.
- [ ] 5.4 Verify existing font attachment extraction/publishing remains the only attachment path and native attachments survive default preservation/restyle.

## 6. Settings UI and live preview

- [ ] 6.1 Add the Settings subtitle styling card using the existing AppSetting mutation/query invalidation flow; verify built-in/custom preset editing, palette bounds, preservation defaults, and explicit override warning.
- [ ] 6.2 Add immediate dark/light/mixed browser preview examples for top note, dialogue, lyrics, and sign; verify color-picker changes do not invoke FFmpeg, OCR, or a subtitle operation and display approximation caveats.
- [ ] 6.3 Add focused frontend tests for HEX normalization/error display, palette deduplication/bounds, preview rendering, default source preservation, and override warning.

## 7. Asset Info Restyle / Regenerate backend

- [ ] 7.1 Add backend request/response contracts for one selected sidecar and exact source identity (path/artifact, revision, representation, language, role/provenance, policy version); verify same-language siblings cannot be substituted.
- [ ] 7.2 Implement ASS/SSA restyle using the existing source-preserving document path without OCR/provider work; verify default preservation and explicit override behavior.
- [ ] 7.3 Implement selected SRT-to-ASS sibling generation through the canonical cue/style path while leaving source SRT unchanged; verify output eligibility and exact artifact identity.
- [ ] 7.4 Implement provenance-authorized embedded/bitmap regeneration by delegating to the existing canonical extraction/OCR lifecycle; verify no duplicate OCR or regeneration pipeline is introduced.
- [ ] 7.5 Add backend operation tests for restyle, SRT sibling, and canonical bitmap regeneration modes, including unsupported/missing provenance failures.

## 8. Asset Info modal/UI

- [ ] 8.1 Add Restyle / Regenerate mode selection to the existing Asset Info subtitle actions for backend-authorized exact sources; verify the UI displays source identity, representation, role, current policy, and mode eligibility without client-side inference.
- [ ] 8.2 Add explicit replacement opt-in and a native-style override warning; verify sibling-by-default behavior and that unavailable/stale sources are not offered as valid targets.
- [ ] 8.3 Add focused frontend lifecycle tests for mode eligibility, exact identity display, override confirmation, progress, terminal cancellation, failure, and retry.

## 9. Cancellation, publication, and provenance

- [ ] 9.1 Reuse the existing subtitle operation registry/status/poll/cancel/retry/context path for Restyle / Regenerate; verify lightweight restyle does not start OCR and cancellation reaches a terminal cancelled-equivalent state after cleanup.
- [ ] 9.2 Reuse operation-owned staging, validation, source revision recheck, publication mutex, atomic sibling/replacement publication, rollback, and bounded cleanup; verify failure/cancel preserves source/current sidecars and replacement is opt-in.
- [ ] 9.3 Record bounded style-policy/source identity provenance and verify no full settings payload, credentials, or unbounded cue content is persisted.
- [ ] 9.4 Add deterministic stale-revision, cancellation, failure-cleanup, atomic-publication, and replace-opt-in regressions.

## 10. Deterministic regressions

- [ ] 10.1 Add backend color, settings, resolver, native ASS/SSA, generated ASS, bitmap geometry, `both`, font-boundary, and collision test fixtures described by the subtitle delta spec.
- [ ] 10.2 Add source-style preservation fixtures with multiple styles, custom colors, karaoke, inline override tags, explicit positioning, layers, margins, and drawing mode; verify default processing does not flatten them.
- [ ] 10.3 Repeat operation lifecycle tests for source conflicts, cancellation during staging/OCR, failed validation, retry identity, and Settings mutation during frozen execution.
- [ ] 10.4 Add focused frontend tests for Settings preview/palette and Asset Info Restyle / Regenerate controls without changing the known global lint baseline.

## 11. OpenSpec and full verifier

- [ ] 11.1 Reconcile implementation evidence against every task and update only completed OpenSpec task checkboxes; verify no production task is marked complete during the design-only phase.
- [ ] 11.2 Run `openspec validate subtitle-ass-styling --type change --strict` and resolve only planning-artifact validation errors.
- [ ] 11.3 After implementation is later authorized, run `./scripts/verify.sh --auto`, confirm `ALL CHECKS PASSED`, review `git diff --stat` and `git status --short`, then commit and push the scoped change.
