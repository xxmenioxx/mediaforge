## Why

Issue #336 records the `containers` job exhausting the GitHub-hosted runner
filesystem while building the backend image. That build compiles FFmpeg,
SVT-AV1, and Intel media components and exports intermediate BuildKit layers;
the failed run did not retain enough logs to measure the peak.

## What Changes

- Add disk and inode diagnostics at runner start, before and after each image
  build, after backend BuildKit cleanup, and after NAS Compose validation.
- Reclaim only explicitly allowlisted preinstalled toolchains on GitHub-hosted
  runners and require 10 GiB free before the backend build.
- Release the temporary job builder's backend cache after its `gha` export so
  the web build does not overlap its local cache footprint.
- Run CI on pushes to the issue branch so it can be validated in Actions
  without opening a pull request.
- Document the storage limit, cleanup boundaries, and capacity escalation
  options.

## Scope Boundaries

Keep both production Dockerfiles, FFmpeg/SVT-AV1, QSV/VAAPI, OCR and subtitle
dependencies, functional checks, web build, and NAS Compose validation intact.
Do not change #334 application code, NAS state, release publishing, or Docker
runtime behavior.

## Capabilities

### New Capabilities

- `ci-runner-disk-capacity`: Observable and bounded storage handling for the
  GitHub Actions container build job.

### Modified Capabilities

None.
