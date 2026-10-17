## 1. Design Approval

- [ ] 1.1 Obtain approval of the diagnostics/evidence tab layout, canonical versus effective-handoff language, finding status language, and immutable-run export boundary in `design.md` before implementation.

## 2. Finding Presentation

- [ ] 2.1 Add typed known-rule presentation formatters and a generic fallback that preserves the rule key, canonical state, explanation, and readable primitive values; verify with focused formatter tests for unknown rules and missing evidence.
- [ ] 2.2 Add finding status labels, severity-first ordering, and expandable/filterable PASS rows; verify tests cover PASS, WARNING, FAIL, UNVERIFIED, REVIEW_REQUIRED, NOT_APPLICABLE, and distinct SKIPPED audit markers.
- [ ] 2.3 Add friendly expected/observed and provenance details for supported structured video and per-track A/V evidence; verify focused tests cover timing offsets, audio track labels, units/tolerances where present, historical omissions, and long values.

## 3. Run Summary and Diagnostics Integration

- [ ] 3.1 Present asset/job/run identity, canonical outcome, score, findings by severity, and backend effective handoff as separate summary values while keeping ValidationPage focused on queries and orchestration; verify tests assert the supplied handoff is displayed without inference.
- [ ] 3.2 Integrate diagnostics and expanded finding views in the responsive details dialog with loading, empty, and error states; verify focused page tests cover historical runs and narrow-layout/accessibility semantics.

## 4. Technical Evidence and Overrides

- [ ] 4.1 Add the separate technical evidence view for the complete immutable run JSON with run-identified copy and JSON download actions; verify tests cover complete report/findings values plus clipboard and download fallbacks.
- [ ] 4.2 Preserve exact-run per-rule Skip and Skip all override flows and display matching actor/reason/time audit details separately from canonical findings; verify existing override tests and added state-distinction tests pass.

## 5. Validation

- [ ] 5.1 Run the focused Validation frontend tests and frontend lint/build checks required by the issue; record the actual command and result.
- [ ] 5.2 Run `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto` and confirm `ALL CHECKS PASSED` or preserve the failing log for the parent.
