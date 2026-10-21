## Context

See proposal.md for motivation. Publisher currently polls publication summaries and eligibility separately every five seconds. Both are bounded read models; eligibility contains current job execution data and validation disposition, while publication summaries carry immutable publication IDs plus bounded evidence and audit arrays. The History route is already the complete audit view.

## Goals / Non-Goals

**Goals:** Present one operational list, avoid duplicate current job rows, preserve distinct publication IDs, and retain existing mutation behavior and bounded polling.

**Non-Goals:** No backend query/API changes, storage changes, attempt rewriting, per-row requests, or new global counts.

## Decisions

- Build a presentation projection from the existing two responses. Match an eligibility item only to a publication with the same `queueJobId` and `executionNumber`; when that exact current execution has multiple publication IDs, show each publication record and suppress the eligibility duplicate once. Keep publications without a matching current eligibility item as their own records, since they may be historical or out of the eligibility window. Never merge attempts or evidence across publication IDs.
- Use Publisher lifecycle state for filters and counts. Eligibility disposition (`ALLOWED`, `ALLOWED_BY_OVERRIDE`, `BLOCKED`) remains separately labeled evidence and does not become publication state. An unmatched eligible job appears as Ready unless blocked; blocked eligibility is attention-needed. A matching publication's persisted lifecycle state takes precedence for that exact execution.
- Sort the unified active list by operational priority across both read models: Ready, Publishing, Failed/Blocked, then Held. Keep Published behind its explicit filter and do not let the source response determine cross-state priority.
- Default to active operational work; provide explicit state filters including Published and a search across filename, job, and destination. Keep evidence collapsed per row and expose History as the full audit route.
- Keep the current five-second query cadence and 100-item eligibility limit. Label counts and publication/evidence summaries as bounded windows; no response-size claim is a global count.
- Reuse all existing mutation handlers and confirmation dialogs. UI state can disable actions for usability, while backend authorization and revalidation continue to decide outcomes.

## Risks / Trade-offs

- [Risk] Bounded endpoints omit older/current records outside their windows → label the windows and direct users to History for complete audit history.
- [Risk] Job identifiers alone can associate a replaced execution incorrectly → require exact job and execution identity; preserve all publication IDs independently.
- [Risk] An eligibility-only blocked row may have incomplete publication context → show backend eligibility reason and do not infer publication evidence.
- [Risk] A larger unified list could render many rows → source windows remain bounded at 100 and evidence is collapsed; no extra requests are introduced.

## Migration Plan

No data or API migration. Deploy the frontend change; rollback restores the prior layout without affecting Publisher records or History.
