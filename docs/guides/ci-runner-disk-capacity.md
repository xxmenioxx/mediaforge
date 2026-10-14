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
