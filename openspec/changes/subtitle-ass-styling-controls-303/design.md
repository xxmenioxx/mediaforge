## Context

The completed `subtitle-ass-styling` change owns one versioned `subtitleStyling` value inside the existing `subtitleSettings` AppSetting. Its backend validator already bounds font size to 0–200 and outline/shadow to 0–20, and canonical ASS writers consume the same style policy. Auto font size and zero font size use a fixed 20-unit fallback; no resolution-aware sizing algorithm exists. Native ASS/SSA remains source-owned unless the existing explicit override/Restyle contract authorizes a change.

## Goals / Non-Goals

**Goals:** expose the already persisted numeric policy in the existing editor, show its effective values honestly, and keep preview and Save behavior within the existing settings and subtitle pipelines.

**Non-Goals:** changing style resolution or validation, adding an AppSetting/schema field, implementing adaptive font sizing, invoking libass in the browser, or applying Settings changes to existing media.

## Decisions

- Keep one `SubtitleStylingCard` and one `subtitleStyling` AppSetting. Place the existing card in the Settings Subtitles section without implementing the broader #266 Settings redesign or creating a duplicate card.
- Treat the backend response and normalizer as authoritative for saved values and bounds. The UI keeps a local draft, clamps numeric edits for usability, and sends only the styling subobject through the existing settings update path so unrelated subtitle settings remain intact.
- Render the existing preview from the selected draft using a 384×288 PlayRes reference, light and dark backgrounds, and the currently selected font/color/weight/outline/shadow values. Label it as a browser approximation; it is not a substitute for libass/player rendering.
- Describe `autoFontSize` as a deterministic fixed 20 ASS-unit fallback. Keep the persisted boolean and font-size value intact; do not infer a resolution-dependent value or alter the canonical writer.
- Keep Reset local until explicit Save. Settings persistence does not invoke subtitle extraction, Restyle, publication, or filesystem operations.

## Risks / Trade-offs

- [Browser text and SVG stroke differ from libass and television renderers] → show the PlayRes reference and approximation caveat beside the preview.
- [A fixed fallback may be mistaken for adaptive sizing] → state the exact 20-unit behavior in the control and preview context.
- [Style updates could be confused with immediate media changes] → keep the existing explicit Save boundary and state that existing subtitle files are unchanged.

## Migration Plan

No database migration or rollout step is required. Existing installations continue to load the same versioned AppSetting; existing generated and native subtitle files are not rewritten. Rollback is a code rollback and does not require data cleanup.
