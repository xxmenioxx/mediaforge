## 1. ASS/SSA styling controls and preview

- [x] 1.1 Expose numeric styling controls for Default, Top, Lyrics, Signs, and custom presets; verify stored values and bounds with frontend and backend regression tests.
- [x] 1.2 Update the existing preview for font size, auto-size fallback, outline, shadow, bold, color, opacity, and light/dark backgrounds; verify the visible SVG properties and PlayRes explanation in focused UI tests.
- [x] 1.3 Preserve local draft, explicit Save, Reset-to-default, and unrelated subtitle settings; verify Settings updates only the existing styling field and does not call media operations.
- [x] 1.4 Verify numeric settings round-trip through the existing canonical style policy and generated ASS serialization, including zero outline/shadow and fixed auto-size fallback.

## 2. Contract and repository validation

- [x] 2.1 Review existing native ASS/SSA preservation and explicit Restyle authorization contracts; keep the change within the existing AppSetting and writer boundaries.
- [x] 2.2 Run strict OpenSpec validation and `./scripts/verify.sh --auto`; confirm the `full` scope completes 7/7 checks with `ALL CHECKS PASSED` before publication.
