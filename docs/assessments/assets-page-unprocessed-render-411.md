# AssetsPage unprocessed render test reconciliation (#411)

The 23 failures reproduced in `AssetsPage.unprocessed-render.test.tsx` came
from four causes: tests using the earlier page layout, stale accessible names,
unscoped text lookups, and Batch Preflight fixtures that referenced inventory
records they had not loaded. One case exposed a real rendering omission: the
existing Legacy Analysis records component was not mounted on the RAW Assets view.

| Test | Root cause and decision | Focused fix |
| --- | --- | --- |
| `renders RAW title assets in the shared scan table and keeps path actions in context` | Test assumed an old title heading, summary, and `Configure` label. Current UI selects a tree item and shows path actions in a named toolbar. **Test-only.** | Select the RAW title, scope actions to its toolbar, assert the actual asset rows, and use `Configure Path`. |
| `shows one title-scoped Snapshots surface with root and nested assets` | Test did not select the title through the current tree before looking for title actions. **Test-only.** | Use the shared title-selection helper and named path-action region. |
| `runs Advisor for every current asset in the title using effective profiles` | Same outdated title-selection assumption; Advisor remains enabled by the existing effective-profile behavior. **Test-only.** | Select the title and find Advisor inside its path-action region. |
| `publishes a title as-is only when every asset resolves the same Destination` | Test used the old title action surface. The same-Destination requirement remains enforced. **Test-only.** | Select the title and scope the action lookup; keep the same-Destination assertion. |
| `blocks title Publish as-is for a missing Destination` | Old title action lookup hid the current action. Missing Destination must still block publishing. **Test-only.** | Use the current title path-action region and retain the disabled-action assertion. |
| `blocks title Publish as-is for different Destinations` | Old title action lookup hid the current action. Differing Destinations must still block publishing. **Test-only.** | Use the current title path-action region and retain the disabled-action assertion. |
| `blocks title Publish as-is for no Destinations` | Old title action lookup hid the current action. Unknown Destination must still block publishing. **Test-only.** | Use the current title path-action region and retain the disabled-action assertion. |
| `blocks title Publish as-is when an asset requires review` | Old title action lookup hid the current action. Review-required assets must remain blocked. **Test-only.** | Use the current title path-action region and retain the disabled-action assertion. |
| `keeps nested path configuration as an explicit inheritance-aware override` | The expected tree name and action label no longer match the accessible tree and toolbar. **Test-only.** | Use `Asset Library` and the current `Configure Path` action; preserve inheritance assertions. |
| `keeps legacy Analysis decisions and notes visible without path-only association` | The existing Legacy Analysis records component and settings query were not connected to the Assets page. This was a **product rendering defect**. | Mount `LegacyAnalysisRecords` with the page settings data and assert its current “Legacy Analysis Decisions” heading. |
| `does not observe asset scope configuration per AssetRow until Asset Info opens` | The query is correctly disabled until Asset Info opens; the test expected it to run once before that boundary. **Test-only.** | Assert zero calls on title selection and expansion, then one call after opening Asset Info. |
| `renames a published converted asset when only a different asset has an active Queue job` | Test searched for the former hierarchy accessible name. Active-job guards and rename behavior were correct. **Test-only.** | Use the current `Asset Library` tree name and retain the unrelated-job rename scenario. |
| `uses the latest stored snapshot after Analyze succeeds, closes, and reopens` | Matching codec text existed both behind and inside the open dialog; the complete dialog refresh flow also sat at the default five-second timeout. **Test-only.** | Scope the selected snapshot assertion to the dialog and allow 10 seconds for this flow, matching the adjacent refresh/reopen case. |
| `does not let a completed Refresh analysis result override a newer stored snapshot after reopen` | Matching codec text existed in multiple rendered regions. **Test-only.** | Scope the newer-snapshot assertion to the dialog. |
| `uses canonical snapshot cleanup when Rename closes Asset Info` | Matching codec text existed in multiple rendered regions. **Test-only.** | Scope the renamed snapshot assertion to the dialog; keep canonical cleanup checks. |
| `selects a RAW title and keeps missing descendants visible when browsing a path` | Test expected the former hierarchy accessible name. **Test-only.** | Use `Asset Library` and preserve checks for missing and present descendants. |
| `loads persisted title values and saves only the explicitly changed dimension` | Test used the former title heading/action path. **Test-only.** | Select the title in the hierarchy and use its current `Configure Path` action; retain the single-dimension persistence assertion. |
| `previews Queue selected and routes the same Asset IDs into Batch Preflight` | The candidate referenced a profile and Library absent from mocked inventory, so safe eligibility correctly disabled acceptance. **Fixture-only.** | Seed the referenced profile and Library; keep the eligibility and ID-routing assertions. |
| `Queue Remaining previews then opens Batch Preflight with the same complete group IDs` | Same missing profile/Library fixture references; disabled acceptance was correct. **Fixture-only.** | Seed the referenced profile and Library; keep complete group-ID assertions. |
| `keeps the RAW title selection in place after Batch Preflight accepts Queue selected` | Same missing inventory records, plus the acceptance mock returned a different candidate ID than the created candidate. **Fixture-only.** | Seed valid inventory references and have the mock preserve the requested candidate ID; retain selection and storage cleanup assertions. |
| `locks Queue and configuration mutations for selected assets with active jobs but keeps Asset Info available` | Test used the former title heading/action surface. The active-job guard was correct. **Test-only.** | Select the title through the tree and assert current Advisor and mutation controls while retaining the lock checks. |
| `keeps Library path actions singular and exposes persisted filters through Advanced` | Media area filters are inside the Filter assets popover, and Library path actions now live in the named path toolbar. Tree levels and the Destination library label also changed. With only the current Library in the fixture, Move path is correctly disabled. **Test-only.** | Open the filter popover before querying fields, reopen it after Escape closes it, use actual hierarchy levels and toolbar labels, and assert one safely disabled Move path action. |
| `retains the saved candidate reference after partial Accept & Queue and Close` | Candidate acceptance was disabled because its referenced profile and Library were missing from fixture inventory. **Fixture-only.** | Seed the referenced profile and Library; preserve the partial-candidate identity and resume assertions. |

The fixes preserve active-job locks, publish eligibility, effective profile
requirements, Queue asset identity, partial candidate persistence, and the
Asset Info loading boundary. Save Tracks cache and subtitle behavior are
outside this reconciliation and remain covered by their existing focused
tests. Full-suite reruns also showed that the existing `resets the dirty
baseline after Asset Info saves successfully` interaction can exceed Vitest's
default five-second timeout; it now has a 10-second per-test timeout. The
adjacent subtitle OCR routing tests also asserted the shortened text
`Translation paused`, while the UI reports the precise state
`Translation paused for source OCR review`; their assertion now matches that
current message without changing translation or OCR behavior.
