## Context

See `proposal.md` and `specs/lab-v2-workspace/spec.md`.

## Authority and visual reference

- **Behavior and domain:** [GitHub issue #183](https://github.com/xxmenioxx/mediaforge/issues/183), this OpenSpec change, and existing MediaForge code.
- **Durable visual direction:** the approved design sections in [GitHub issue #183](https://github.com/xxmenioxx/mediaforge/issues/183). The issue contains no direct Figma/Make URL or attached screenshot, so there is no separately versioned pixel reference in this change.
- **Figma Make:** design-generation/reference material used during implementation only. It is not a runtime or production-code dependency. The reviewed screenshot contains third-party media artwork, and the repository has no redistribution license for that artwork; the screenshot is therefore not committed.

The approved design's sample asset, metric values, labels, menu state, and sample thumbnails are illustrative, not application data.

The production route already runs inside `AppLayout`, which owns the collapsed navigation rail, mobile drawer, top-level search, and account controls. `ProfileLabPage` owns the selected asset, video/audio/track drafts, profile scope, preview range, compatible preview identities, and Fidelity evidence. `TestEncodeDialog` owns Test Encode creation and canonical lifecycle actions; `api.testEncodes(sourcePath)` uses the bounded retained Test Encode endpoint, and the dialog and LAB can share its React Query key. Test Encode rows carry output identity, range, execution/retention state, validation report, and any persisted Profile Qualification. The #157 and #142 contracts therefore require no backend change for this presentation.

## Goals / Non-Goals

**Goals:**

- Translate the approved header / parameter-panel / A-B / history composition into the current MediaForge shell and MUI theme.
- Keep the existing Video, Audio, and Tracks drafts as the actual parameter state and continue using current profile-selection and capability behavior.
- Make a selected retained Test Encode output the Result pane and keep source-reference identity reusable.
- Keep current profile Fidelity, preview, and Test Encode lifecycle details reachable without placing their full technical surface in the primary comparison layout.

**Non-Goals:**

- No Figma Make application shell, CSS framework, or generated dependency is imported.
- No duplicate Test Encode lifecycle, history store, profile-scope model, quality calculation, media scan, backend endpoint, or persisted snapshot inventory is added.
- No changes to #142 qualification semantics, #69/#70 evidence and sampling, Test Encode scheduler/runtime separation, cancellation, cleanup, Queue, validation, publishing, or production Library lifecycle.
- No promise of per-run image thumbnails, generic Test Encode preset export, one-click exact duplication, or result export when the current persisted contracts do not provide those artifacts/actions.

## Decisions

1. **Use the existing AppLayout and Assets V2 visual patterns.** LAB remains on its current route inside `AppLayout`; the issue screenshot does not justify replacing the app shell. Use compact outlined surfaces, theme dividers, existing spacing/radius, and responsive MUI grids. The Make export informs hierarchy and control grouping only.

2. **Move the current context and scope into the header without creating another model.** The header uses `selectedAsset`, the persisted selected snapshot, and `activeProfileScope`. Path/Asset continues to update the currently edited draft's existing scope. Its helper text clarifies that a Test Encode still operates on the selected asset. The single range control reuses the current LAB start/duration state and passes it as a custom Test Encode sample window; it is not repeated in the run dialog.

3. **Keep the full existing editor as the parameter authority.** The Video/Audio/Tracks editing surfaces and save-review flows are reused in the left parameter region. The video profile selector is presented once. Advanced profile controls remain reachable from this workspace, rather than adding a parallel simplified profile resolver. Existing Process Asset, FFmpeg diagnostic, fidelity, frame, and qualification tools remain available in secondary detail surfaces.

4. **Use canonical Test Encode records for the result and history.** The history reads the existing source-path-filtered Test Encode query with a bounded page and only a small visible strip. The run dialog keeps cancellation, Keep/Temporary, details, delete confirmation, and qualification ownership. Selecting a ready run changes only presentation state. The selected run's output path is consumed through the existing asset-preview route and its persisted output is never mutated.

5. **Compare the source at the selected sample window with the retained result.** Both panes use equal responsive boxes. Playback starts at the Test Encode start offset in the source and at zero in the result; shared play/pause and scrub behavior maps positions across that window. Existing compatible source-reference identities are reused when available. No new Original Test Encode or preview generation is triggered just to select history. Unsupported direct playback is reported as unavailable rather than rewritten into another retained artifact.

6. **Render metrics from available evidence only.** Source facts come from the existing asset/snapshot response; output size, effective encoder, validation, and qualification come from the selected Test Encode. Quality metrics are read from its canonical validation/qualification evidence. The UI does not calculate PSNR/SSIM/VMAF or turn a missing/stale qualification into a verdict. Values shown in the Figma screenshot are never fixture defaults.

   The retained source size is for the whole asset while the Test Encode output size is for the selected sample window. The comparison therefore labels both separately and leaves the delta unavailable until canonical sample-sized source evidence exists.

7. **Represent unsupported design actions as follow-up gaps.** Open Asset Info and existing edit/reset/profile-save workflows remain reachable. No exact duplicate Test Encode or result-export operation exists, and the Test Encode inventory has no persisted per-run still image. The workspace will not label current-profile reruns or source-derived artwork as exact duplication/export/snapshot behavior.

## Risks / Trade-offs

- **Risk:** The historic profile editor has a much larger control set than the screenshot's example panel → Keep all current editors reachable and capability-driven, with the concise experiment controls first.
- **Risk:** A Test Encode output codec may not play in the browser → Show a clear unavailable state and preserve the persisted output/evidence; do not silently transcode or re-encode on selection.
- **Risk:** Long filenames and narrow technical panes can force overflow → Use `minWidth: 0`, truncation with an accessible full-name label, and responsive stacking.
- **Risk:** Historical records can outnumber the visible strip → Use the backend's existing bounded page and show a link to the existing full Test Encode management view.
- **Visual limitation:** The screenshot shows an asset preview thumbnail, distinct still thumbnails in history, and populated sample metrics. The current inventory provides no persisted per-run still artifacts, many runs have no canonical quality metrics, and there is no sample-sized source byte count. Use a restrained media glyph in the header, a glyph-based Original reference/history strip, separately labeled source-file and result-sample sizes, and explicit unavailable labels instead of generating media or metrics at render time.

## Migration Plan

No data migration or backend rollout is required. Keep the existing route, query keys, APIs, and lifecycle dialog. Rollback is a frontend revert and does not alter retained Test Encodes, preview artifacts, source media, or Library assets.
