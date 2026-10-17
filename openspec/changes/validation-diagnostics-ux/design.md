## Context

`ValidationPage` currently loads the review list and opens a run detail dialog. The existing `ValidationRunDetail` response already contains the immutable `run`, exact-run override events, the backend `effectiveHandoff`, and current-policy metadata. The `ValidationRun` includes its identity, outcome, score, findings, and report. Finding values and report evidence are intentionally flexible JSON, and historical runs can omit optional evidence.

## Goals / Non-Goals

**Goals:**

- Make the canonical Validation result understandable without reading nested JSON.
- Keep effective Publisher handoff and override audit state visibly separate from canonical run/finding state.
- Make the complete immutable run available to support and engineering on demand.
- Keep the change in the existing frontend page and API contract.

**Non-Goals:**

- Changing finding state, severity, scoring, thresholds, policy, run persistence, override authorization, or Publisher behavior.
- Adding a backend export endpoint, new API fields, media probes, filesystem reads, or dependencies.
- Redesigning the application shell or Validation list workflow.

## Decisions

1. **Keep the current Validation list and details entry point.** Selecting Details opens the existing responsive dialog. The dialog presents a `ValidationRunSummary`, then Diagnostics and Technical Evidence tabs. This preserves current navigation and list filters while separating the support view from the user-facing table. The dialog remains within the MVForge V2 shell and uses existing MUI tokens and controls.

2. **Keep data loading and presentation separate.** `ValidationPage` owns list queries, run detail loading, and existing override mutations. Isolated components own the summary, finding table, expanded details, and technical JSON actions. A typed rule presentation registry provides friendly names and evidence formatting for known rules; a generic fallback retains the original rule key and safely presents available explanation and primitive values for new or historical rules.

3. **Treat backend finding state and override events as separate inputs.** Finding status comes only from `ValidationFinding.state`; severity ordering uses the supplied `severity`. The default order places FAIL, WARNING, and UNVERIFIED findings before PASS, followed by other states. Filters and expansion can reveal passed findings. Matching override events add an audit marker and available actor, reason, and time, without changing the state shown for the finding or the run outcome. Effective handoff uses only `ValidationRunDetail.effectiveHandoff` and is labeled independently.

4. **Format evidence only when its existing shape is understood.** Known formatters may present supported expected/actual fields, units, track labels, timing offsets, tolerances, and provenance from the immutable finding/report data. They do not derive a new validation decision or invent an action. Missing evidence is shown as unavailable or UNVERIFIED, including in historical runs. Generic diagnostics do not embed JSON; the full structured values remain in Technical Evidence.

5. **Export the immutable `run` object from the existing detail response.** The technical view renders a readable JSON representation of the full `run`, including identity, policy snapshot fields, report, and findings. Copy and download operate on that same object; the download filename includes a sanitized run identity. Overrides and current effective handoff remain separately displayed because they are associated detail response data, not fields of the immutable run. Copy/download failures use an accessible fallback or a clear recoverable error. No export is sent to a server or written to application logs.

6. **Use responsive, keyboard-accessible controls.** Summary details wrap at narrow widths; long identifiers and track labels wrap within finding details. Text labels accompany status colors. Tabs, filters, expanders, skip actions, and copy/download controls remain keyboard reachable and expose accessible names.

**Design approval checkpoint:** Before implementation, review this dialog/tab information architecture, the canonical versus effective-handoff labels, the finding-state language, and the decision to export the immutable `run` object while keeping override audit events separate. No Figma mockup is included in this proposal.

## Risks / Trade-offs

- [Risk] A historical or newly introduced rule may not have a specialized formatter → Keep the raw rule key and canonical state, show available explanation and generic friendly values, and retain the full run in Technical Evidence.
- [Risk] Browser clipboard or download APIs may be unavailable → Keep JSON visible and provide a fallback or clear recoverable error; cover both paths in focused tests.
- [Risk] Flexible evidence may contain long values → Wrap or locally constrain values in details and keep the main diagnostics compact; do not place full JSON in table cells.
- [Risk] Technical data may contain paths already present in the run report → Export only the returned immutable run object on explicit user action, introduce no extra filesystem data or logging, and do not silently prune fields from the run export.

## Migration Plan

No migration or backend deployment step is required. The frontend continues to call the existing Validation list and detail APIs. Rollback is a frontend rollback; stored runs, reports, overrides, and Publisher state remain unchanged.
