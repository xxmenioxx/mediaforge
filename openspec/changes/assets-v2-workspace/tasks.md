## 1. Compact Header and Navigation

- [x] 1.1 Add compact Total Assets, Processing, Completed, and Needs Review summaries from existing inventory counts; verify the rendered values against the mocked canonical inventory response.
- [x] 1.2 Preserve the existing collapsible desktop rail and mobile drawer, expose existing Reports and Logs destinations where applicable, and verify active Assets navigation plus collapsed/mobile reachability.

## 2. Library Hierarchy and Asset List

- [x] 2.1 Add a lightweight, keyboard-accessible Library/path hierarchy derived from existing group summaries; verify selecting a nested path filters its assets without disturbing URL filters.
- [x] 2.2 Present a paginated scan-first table with Name, Status, Profile, Subtitles, Duration, and Modified columns, no row thumbnails, readable unavailable values, and truncated long names; verify row selection and the default column set.
- [x] 2.3 Keep selected-path Advisor directly discoverable and expose existing Queue, configuration, migration, and secondary asset actions through compact contextual or overflow surfaces; verify each entry still invokes its existing workflow.

## 3. Selected Asset Details

- [x] 3.1 Add a compact selected-asset strip with available status, profile, subtitle, duration, modified, container, size, video, resolution, audio, and path summaries; verify deeper persisted detail is requested only for the selected asset.
- [x] 3.2 Wire the integrated preview Play control, Assign Profile, Test Encode, and secondary actions to the existing AssetRow flows; verify no duplicate Preview/Play CTA and retain canonical Test Encode behavior.
- [x] 3.3 Render canonical descriptive metadata and NFO-derived information only when existing data is present; verify the absent-metadata case creates no empty section.

## 4. Regression and Validation

- [x] 4.1 Add focused workspace and navigation regressions for table scanning, path filtering/actions, selection, detail behavior, accessibility, and responsive layouts; run the focused frontend tests.
- [x] 4.2 Run `openspec validate assets-v2-workspace --strict`, `npm run lint`, `npm run build`, and `./scripts/verify.sh --auto`; record exact results and resolve only failures introduced by this change.
