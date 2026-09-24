## 1. Freeze the canonical SamplingPlan contract

- [ ] 1.1 Extend the existing `SamplingPlan`/`SamplingWindow` model with schema/policy/plan identity, source revision, bounded evidence manifest, stable window identity, roles/reasons, score summaries, fallback state, and limits; verify legacy position-only plans remain readable and new plan serialization contains only bounded fields.
- [ ] 1.2 Implement canonical source-timeline normalization, request normalization, frame/fallback tolerance, and window identity derivation; verify clamping, invalid intervals, equivalent intervals, and identity stability with table-driven tests.
- [ ] 1.3 Define the versioned role priorities, score quantization, evidence-confidence handling, per-role candidate limit, global maximum including primary, and deterministic final ordering; verify equal-score tie cases do not depend on map or input enumeration order.
- [ ] 1.4 Implement bounded consolidation that unions roles/reasons for equivalent candidates while retaining materially different overlapping candidates; verify overlap tolerance, one-window/multiple-role behavior, and bounded discarded/consolidated summaries.

## 2. Reuse canonical evidence and provenance

- [ ] 2.1 Add SamplingPlan evidence adapters for existing source fingerprints, snapshot component/version manifests, distributed frame/GOP/frame-signal windows, cadence/interlace evidence, restoration noise/grain/blocking/ringing/detail evidence, stream metadata/bitrate, bounded packet/timing evidence, and existing LAB/reference windows; verify each adapter preserves availability, confidence, source, and version without inventing missing roles.
- [ ] 2.2 Make interval-specific measured evidence outrank global metadata and keep stale, ambiguous, or diagnostic-only inputs non-authoritative; verify a global bitrate alone cannot produce a high-complexity or worst-predicted claim.
- [ ] 2.3 Reuse matching persisted and LAB evidence by source revision and evidence identity, with explicit call accounting around probe/analyzer reuse; verify equivalent plan resolution does not launch duplicate expensive full-file or semantically equivalent analysis.
- [ ] 2.4 Persist bounded selected-window provenance, evidence references, fallback reason, and deterministic top discarded/consolidated summaries; verify frame arrays, decoded images, complete packet datasets, full probe payloads, and unbounded logs are excluded.

## 3. Implement bounded candidate resolution and fallback

- [ ] 3.1 Build the canonical candidate pipeline from explicit primary, distributed baseline, and available evidence candidates through normalization, bounded role scoring, consolidation, cap enforcement, and plan identity; verify same fingerprint/evidence/policy inputs produce byte-stable selected windows and ordering.
- [ ] 3.2 Preserve the current distributed positions, short-asset adaptation, and baseline order as the deterministic fallback when interval hard-scene evidence is insufficient; verify fallback provenance, unavailable evidence, no random selection, and non-fatal LAB/Test Encode behavior.
- [ ] 3.3 Verify bounded selection across typical, high-motion, high-texture-or-grain, dark-or-gradient, high-complexity, and worst-predicted roles, including optional roles, shared winners, deterministic tie-breaking, and the hard maximum including primary.

## 4. Freeze and invalidate plans at lifecycle boundaries

- [ ] 4.1 Freeze the SamplingPlan with source revision, policy/version, relevant evidence identities, selected windows, roles/reasons, normalization, fallback, and plan identity at LAB operation start and Queue `PREPARING`; verify later settings/evidence changes cannot alter a frozen execution.
- [ ] 4.2 Add the same frozen-plan identity contract to Test Encode and Validation evaluation records; verify a changed source revision, evidence version, or policy version marks an unfrozen plan stale and never silently remaps a frozen plan.
- [ ] 4.3 Preserve backward compatibility for persisted snapshots and operations without the new fields by treating them as legacy distributed baseline plans; verify no migration reinterprets legacy positions as hard-scene evidence.

## 5. Integrate LAB without breaking preview-first ownership

- [ ] 5.1 Feed the canonical plan from LAB's explicit `Start` + `Seconds` selection while retaining that interval as the primary Compatible Preview/Frame Fidelity window; verify the primary request/artifact/generation identity is reused and no second primary encode is requested.
- [ ] 5.2 Route additional selected windows through the existing Compatible Preview artifact path with frozen effective configuration, stable window identity, role/reason summaries, bounded evidence references, and the existing sequential heavy-work guard; verify additional windows execute one at a time in canonical order.
- [ ] 5.3 Preserve independent Preview Ready and Fidelity lifecycle semantics while exposing additional hard-scene role/reason provenance where useful; verify primary readiness remains usable if additional analysis is incomplete, canceled, or falls back.
- [ ] 5.4 Add LAB regression coverage for an explicit primary outside the hard-scene winners, primary/candidate consolidation, no primary re-encode, sequential additional windows, bounded maximum, cancellation, partial results, and stale source/generation identity.

## 6. Add read-only future consumer contracts

- [ ] 6.1 Add the #69 perceptual-quality consumer contract to accept the frozen SamplingPlan and evaluate the same window identities without invoking selection; verify it reports plan/policy provenance and cannot replace unavailable roles with a new scene set.
- [ ] 6.2 Add the #9 QSV pumping consumer contract for existing `high_motion`, `high_texture_or_grain`, and `worst_predicted` windows; verify role filtering preserves original identities/reasons and returns explicit unavailability without creating another selector.
- [ ] 6.3 Add Validation/reporting integration that distinguishes selection evidence from output verdicts and preserves window identity/provenance; verify Frame Fidelity/canonical verdict authority remains unchanged.

## 7. Validate the bounded architecture

- [ ] 7.1 Add deterministic table-driven tests for same inputs/same plan, changed source revision, changed policy/version, insufficient evidence, distributed fallback, overlapping candidate consolidation, multiple roles on one window, tie-breaking, bounded maximum, and role omission.
- [ ] 7.2 Add evidence reuse and call-accounting tests covering snapshot reuse, no duplicate expensive analysis, interval-specific versus global precedence, stale evidence rejection, and bounded provenance truncation without sleeps or real NAS media.
- [ ] 7.3 Add integration tests for LAB primary preservation/no re-encode, sequential additional Fidelity windows, Queue/Test Encode freeze semantics, #69 same-window consumption, #9 role consumption, and legacy-plan compatibility.
- [ ] 7.4 Run `openspec validate hard-scene-sampling --type change --strict` and the canonical repository verification `./scripts/verify.sh --auto`; require `ALL CHECKS PASSED` before implementation delivery.
