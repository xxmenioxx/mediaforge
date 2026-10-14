# Container CI runner storage

The `containers` job builds the production backend and web Dockerfiles on a
standard GitHub-hosted Ubuntu runner. GitHub documents 14 GB of SSD storage for
standard Ubuntu runners, so the job requires 10 GiB of free space before the
backend build. This is an admission guardrail, not a claim that every backend
build peaks below that amount; CI records actual free space, inode use, Docker
usage, and BuildKit cache use at each build boundary.

Before building, CI cleans the apt download cache and removes only these
allowlisted, unused runner toolchains: `/opt/hostedtoolcache/CodeQL`,
`/opt/hostedtoolcache/dotnet`, `/usr/local/lib/android`, `/usr/local/.ghcup`,
`/usr/local/share/boost`, and `/usr/share/dotnet`. Cleanup runs only on
GitHub-hosted runners, skips symlinks, refuses changed canonical paths, and
reports recovered filesystem space. It does not remove Docker images,
arbitrary host directories, or components from either Dockerfile.

The backend's `type=gha,mode=max` cache remains enabled because it preserves
expensive FFmpeg and Intel media build stages for later jobs. Once that export
completes, CI prunes the current job's Buildx cache before starting the web
build. The production web build and NAS Compose validation remain required.

If the pre-build check reports less than 10 GiB free, or a later build still
runs out of disk, use the recorded `df`, `docker system df`, and `docker buildx
du` output to identify the pressure point. Next options are a larger runner, a
maintained prebuilt media-dependency image, or a measured cache strategy
change. Do not disable `containers` or replace a production Dockerfile with a
reduced test image.

## Issue #336 measurements

Two successful executions on GitHub-hosted runners (run `38506374332`,
attempts 1 and 2, commit `f68b08a`) reported:

| Measurement | Attempt 1 | Attempt 2 |
| --- | ---: | ---: |
| Free space after checkout | 12.43 GiB | 12.43 GiB |
| Net space recovered by cleanup | 21.75 GiB | 21.75 GiB |
| Free space before backend build | 33.95 GiB | 33.95 GiB |
| BuildKit cache after backend build | 13.78 GB | 1.85 GB |
| Free space after backend build | 21.11 GiB | 32.22 GiB |
| Free space after web build and Compose | 33.19 GiB | 33.28 GiB |

The warm second attempt reused the exported GitHub Actions cache and reduced
local backend cache by about 11.93 GB. The selected builder reported `0B` after
the post-backend prune in both attempts. `docker system df` showed 1.852 GB of
preinstalled images initially and 2.101 GB after Buildx setup; these were left
untouched. The build output is not loaded as a Docker-daemon image, so the
workflow does not record a separate final-image size. These are milestone
snapshots; they do not measure the lowest free space during compilation.
