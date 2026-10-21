## 1. Unified Publisher presentation

- [x] 1.1 Project eligibility and publication responses into one exact-identity list, preserving distinct publication IDs; verify matching, replaced-execution, and publication-only cases in focused UI tests.
- [x] 1.2 Add bounded state summaries, default active filter, explicit Published filter, and search; verify filters, cross-source operational state priority, deduplication, and bounded-window wording in focused UI tests.
- [x] 1.3 Move attempt, archive, validation, and audit details behind row expansion and link full History; verify evidence identities and History navigation.
- [x] 1.4 Preserve existing action gating, confirmation/reason flows, errors, and query invalidation; verify existing and added action tests.

## 2. Validation

- [x] 2.1 Run focused Publisher page tests and confirm required empty, error, mixed-state, and action behaviors.
- [x] 2.2 Run `python3 /Users/anuelvs/Documents/mediaforge/.agents/skills/dupla/scripts/dupla.py lock --scope shared-validation -- ./scripts/verify.sh --auto` and record the observed result.
