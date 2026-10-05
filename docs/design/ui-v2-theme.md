# MVForge UI V2 theme tokens

Assets V2 is the current visual source of truth for MVForge UI V2. The canonical
tokens live on the existing MUI theme at `theme.mvforge`; use these meanings in
new shared UI rather than copying values from an Assets `sx` object or an
external template.

## Surface hierarchy

Use surfaces in this order, from the broadest area to the most active state:

| Role | Token | Use |
| --- | --- | --- |
| Canvas | `theme.mvforge.colors.canvas` | App or page background |
| Workspace | `theme.mvforge.colors.workspace` | A bounded work area inside the canvas |
| Surface | `theme.mvforge.colors.surface` | Common panels, cards, and controls |
| Raised surface | `theme.mvforge.colors.surfaceRaised` | Menus, overlays, and elements above a panel |
| Interactive surface | `theme.mvforge.colors.surfaceInteractive` | Resting or hovered interactive regions |
| Selected surface | `theme.mvforge.colors.surfaceSelected` | Selected-state tint; use `theme.mvforge.interaction.selectedBackground` when the existing treatment calls for its gradient |

Use `theme.palette.background.default` and `theme.palette.background.paper` for
the MUI canvas and common surface mappings. Use the explicit `mvforge` tokens
when a component needs a more specific role.

## Text, borders, accents, and status

`theme.mvforge.colors.text` defines primary, secondary, muted, and disabled
text. MUI `text.primary`, `text.secondary`, and `text.disabled` map to those
common roles. Borders use `subtle`, `default`, and `strong`; MUI `divider` maps
to the subtle border.

Use the blue primary accent for primary actions and information, and the violet
secondary accent for secondary emphasis. The `status` tokens provide restrained
success, warning, error, info, and neutral/unavailable colors. Status surface
tokens pair with their corresponding foreground when a tinted background is
needed. MUI's `success`, `warning`, `error`, and `info` palette colors map to
these status values.

The `interaction` tokens cover hover, selected, and focus states. Preserve the
visible keyboard focus indicator when using them.

## Typography and density

The theme keeps the existing Inter-first font stack. UI V2 semantic sizes are:

| Role | Token | Size |
| --- | --- | --- |
| Body/value | `theme.mvforge.typography.body` | 0.8rem |
| Compact body | `theme.mvforge.typography.compactBody` | 0.78rem |
| Supporting text | `theme.mvforge.typography.supporting` | 0.75rem |
| Tertiary text | `theme.mvforge.typography.tertiary` | 0.72rem |
| Section heading | `theme.mvforge.typography.sectionHeading` | 0.8rem |
| Page heading | `theme.mvforge.typography.pageHeading` | 1.25rem |

Use tertiary sizing only for genuinely secondary metadata. Avoid shrinking
content to fit; exceptional artwork or dense technical labels may retain their
existing local treatment. Existing global MUI heading variants are retained
for compatibility and are not migrated page-by-page in this foundation issue.

Density is opt-in through `theme.mvforge.density`: compact controls are 32px,
normal controls are 36px, and the compact row reference is 42px. The global MUI
button default remains unchanged. Shared-control adoption belongs to the
follow-up component migration.

## Usage

Prefer semantic tokens for a repeated visual role:

```tsx
<Box sx={{ bgcolor: (theme) => theme.mvforge.colors.surface }} />
```

Avoid repeating a value that already has a semantic role:

```tsx
<Box sx={{ bgcolor: '#091a2c' }} />
```

Page-local values remain appropriate for genuinely unique artwork, data
visualizations, or a one-off effect that has no shared meaning. Do not copy
random template colors, branding, logos, or demo artwork into MVForge pages.
External code, assets, and dependencies must follow the provenance and license
guardrails in #219; this token foundation introduces none.
