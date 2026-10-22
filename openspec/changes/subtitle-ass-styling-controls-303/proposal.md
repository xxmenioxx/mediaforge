## Why

The canonical subtitle styling policy already persists bounded font size, auto-size, outline, and shadow values, but the Settings editor does not expose them or make the rendered effect understandable. Operators need to review the effective values and save style changes without changing existing sidecars or implying that the legacy auto-size fallback adapts to video resolution.

## What Changes

- Extend the existing ASS/SSA styling editor for built-in and custom presets with font size, auto font size, outline width, and shadow depth.
- Show persisted and draft values in ASS style units with PlayRes context, and update the existing preview for typography, colors, outline, shadow, and bold on light and dark backgrounds.
- Preserve the existing `subtitleStyling` AppSetting, draft/save/reset lifecycle, backend validation bounds, and native ASS/SSA preservation behavior.
- Document that auto font size currently resolves to a fixed 20-unit fallback, not resolution-aware sizing.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `subtitles`: expose the existing global ASS/SSA style policy in Settings and explain effective values and preview limits.

## Impact

- Frontend: existing Settings navigation and `SubtitleStylingCard`/tests.
- Backend: existing bounded style policy and ASS generation remain canonical; focused tests verify configured numeric values and fixed fallback serialization.
- No API schema, migration, new style store, media mutation, or automatic restyling of existing sidecars.
