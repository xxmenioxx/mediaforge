## Context

The current Library screenshot shows rows such as:

```text
Root
/media/library/anime/ARBEGAS

Season1
/media/library/anime/Attack on Titan/Season1
```

while the column header remains `Asset group`.

The behavior entered during `feature: Assets page refactor part1`
(`c1ce531`, 2026-08-27). Before that change the row used
`groupTitle(group)` and `groupSubpath(group)`; afterward the row used
`pathLabelForCollection(group)` and `cleanPathLabel(group.path)`.

The path-oriented helpers remain valid for path configuration, Queue notes,
snapshot dialogs, and Unprocessed path hierarchy. The regression is using that
operational identity as the main inventory identity in Library/Converted.

## Goals / Non-Goals

**Goals:**

- Make Library and Converted rows identify the logical media/title/group first.
- Keep physical path information discoverable for diagnostics and operations.
- Preserve path-based search.
- Preserve Unprocessed's logical group and child-path hierarchy.
- Add regression tests that distinguish logical identity from path scope.

**Non-Goals:**

- Rebuild the Assets inventory architecture.
- Change `AssetGroup`, `AssetRecord`, logical-group persistence, or grouping algorithms.
- Change path configuration scopes or Profile assignment precedence.
- Rename files/directories.
- Remove path information from Asset Info, expanded details, dialogs, tooltips, or search.
- Implement issue #21's entire future Assets architecture in this fix.

## Decisions

1. **Logical identity is primary in Library/Converted.**
   The first visible label SHALL represent the title/group identity a user recognizes,
   not `Root`, `Season1`, or an absolute filesystem path.

2. **Path identity remains operational metadata.**
   Path-oriented helpers such as `pathLabelForCollection()` remain valid where the
   UI is explicitly operating on a path. This change SHALL NOT globally redefine
   those helpers if doing so would break Unprocessed, snapshots, configuration, or
   Queue semantics.

3. **Do not hide useful structure.**
   When a logical title contains a meaningful child path, a compact secondary label
   MAY expose a relative path such as `Season1`. It SHOULD NOT default to the full
   absolute path in the collapsed inventory row.

   Preferred examples:

   ```text
   Attack on Titan
   Season1 · Anime
   ```

   or, when no child-path qualifier is useful:

   ```text
   Berserk
   Anime
   ```

4. **Absolute paths remain available on demand.**
   Full paths MAY remain in expanded details, Asset Info, tooltips, accessible labels,
   or other operational surfaces. Search SHALL continue matching them.

5. **Use existing canonical data before adding API fields.**
   Prefer current `AssetGroup`, first asset, `logicalGroupPath`, relative path,
   library name, and existing display helpers. Add backend/API data only if a focused
   implementation proves the logical title cannot be reconstructed reliably.

6. **Do not alter Unprocessed hierarchy.**
   Unprocessed intentionally distinguishes logical titles from `Root`, `Season1`,
   `Specials`, etc. Those path labels are useful because the user configures and
   selects path scopes there.

7. **Converted follows Library identity semantics.**
   Converted represents provenance/lifecycle, not a new path identity. Its collapsed
   row should therefore use the same logical-title-first presentation principle.

## Risks / Trade-offs

- **Risk →** Two titles with similar names may become visually ambiguous.
  **Mitigation →** keep Library/media-area/relative-path context as secondary metadata
  and retain full path in details/search.

- **Risk →** Reusing old `groupTitle()` behavior blindly may restore earlier grouping
  assumptions that no longer match the refactored data.
  **Mitigation →** define a dedicated display resolver or adapt existing helpers using
  current logical-group evidence; test root and nested-season cases.

- **Risk →** Changing a shared helper may accidentally alter Unprocessed path labels.
  **Mitigation →** scope the display resolver by inventory mode and add tests for both
  Library/Converted and Unprocessed.

- **Risk →** Search could regress if the rendered path disappears.
  **Mitigation →** keep `filterAssetGroups()` path matching independent from visible
  row labels.

## Migration Plan

1. Add focused display-resolution tests that reproduce current `Root/full path` and
   `Season1/full path` Library behavior.
2. Implement a mode-aware logical inventory display resolver in
   `frontend/src/pages/AssetsPage.tsx`.
3. Keep absolute path evidence in an operational/detail surface and verify path search.
4. Verify Unprocessed logical title/path hierarchy and Converted rows remain correct.
5. Run focused frontend tests, then `./scripts/verify.sh --auto`.
6. No data migration or rollback operation is required; rollback is frontend-only.
