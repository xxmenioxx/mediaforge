# CI FinOps baseline and routing

## Baseline

The baseline was sampled from completed GitHub Actions runs on 2026-10-18. Job
durations below are elapsed time from each job's `startedAt` to `completedAt`;
parallel jobs overlap, so they do not add up to workflow wall time.

| Run | Job elapsed | Workflow elapsed |
| --- | --- | --- |
| [PR #360 CI, run 39142247411](https://github.com/xxmenioxx/mediaforge/actions/runs/39142247411) | backend 5:10; containers 3:48; frontend 1:11; compliance 0:59 | 5:14 |
| [PR #360 Pipeline Smoke, run 39142247436](https://github.com/xxmenioxx/mediaforge/actions/runs/39142247436) | smoke 2:48 | 2:52 |
| [PR #283 CI, run 39081552924](https://github.com/xxmenioxx/mediaforge/actions/runs/39081552924) | backend 4:15; containers 2:36; frontend 1:42; compliance 0:49 | 4:18 |
| [PR #283 Pipeline Smoke, run 39081552959](https://github.com/xxmenioxx/mediaforge/actions/runs/39081552959) | smoke 1:11 | 1:14 |

For PR #360, the five concurrent jobs consumed 13:56 of aggregate runner
elapsed time. A rough whole-minute ceiling is 16 runner-minutes: 6 backend + 4
containers + 2 frontend + 1 compliance + 3 smoke. This is a billing proxy, not
an observed invoice value. GitHub's billed minutes depend on repository
visibility, runner type, account plan, and billing rules; the Actions run data
does not expose the account's billed-minute total.

The redundant work was directly observable: a backend PR built the backend
Docker image in `CI / containers` and independently built it again in
`Pipeline Smoke / pipeline-smoke (happy-path)`. CI also built the web image on
every change, and ran both full backend/race tests and frontend install/build
for unrelated path changes. The release image workflow is tag-triggered and
keeps its compliance, license/source-evidence, attestation, and publish gates.
This change does not edit that workflow.

## Routing matrix

| Changed paths | CI checks | Image work | Pipeline Smoke |
| --- | --- | --- | --- |
| `docs/**`, `README.md`, `CONTRIBUTING.md` | Selector only | None | Selector only; smoke job skipped |
| `backend/**` except dependency/license/shared files | Backend unit, race, vet; Compose validation | Smoke builds backend image once; CI skips duplicate backend image build | Full synthetic smoke |
| `frontend/**` except dependency/license/shared files | Frontend install/lint/build; Compose validation | Web image only | Smoke job skipped |
| `scripts/e2e/**` | Selector only | Smoke builds backend image | Full synthetic smoke |
| Docker context (`backend/**` or `frontend/**`) | Backend or frontend checks above; Compose | Build only affected image in CI when smoke is not exercising the backend image | Backend context runs smoke; web context skips smoke |
| Dependencies, legal files, workflows, `scripts/ci/**`, verification, deployment, root/shared config | All existing CI checks | Backend and web images; Compose | Full synthetic smoke |
| Empty/ambiguous diff, unknown event or unclassified path | All existing CI checks | Backend and web images; Compose | Full synthetic smoke |
| Manual `workflow_dispatch` | All existing CI checks | Backend and web images; Compose | Selected scenario runs |

The four existing CI job IDs and check names (`compliance`, `backend`,
`frontend`, `containers`) remain stable. Workflow-level path filters are not
used, so GitHub still creates each required check; irrelevant jobs complete as
skipped after the selector. Selector failure or an unknown path enables the
full suite. Only newer runs for the same pull request can cancel an older PR
run. Main pushes and manual runs use unique concurrency groups and cannot
cancel another PR or main run.

## Savings estimate and limits

For a docs-only PR, the sampled 13:56 of CI-plus-smoke runner elapsed is avoided
apart from two short selector jobs. For a frontend-only PR, the sampled backend
job (5:10), compliance job (0:59), and smoke job (2:48) are avoided; the
frontend and web-image checks remain. For a backend-only PR, backend tests,
Compose validation, and the end-to-end smoke remain, while the unrelated
frontend/compliance jobs and duplicate CI backend build are removed. Thus
estimated savings are workload-dependent; the routing tests measure which
checks are selected, not billed minutes.

These estimates use one recent PR's observed per-job elapsed times. They are
not a guarantee for future runs: cache state, runner queue/setup, code size,
and smoke scenario affect elapsed time. Compare billed minutes with the
repository's Actions billing export after several post-change runs before
claiming realized cost savings. Manual full runs and high-risk changes retain
the complete validation cost by design.
