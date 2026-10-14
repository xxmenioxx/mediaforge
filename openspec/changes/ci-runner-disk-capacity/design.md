## Context

The standard Ubuntu GitHub-hosted runner has bounded SSD capacity. The
backend Dockerfile contains large Go, Intel media, FFmpeg/SVT-AV1, and subtitle
stages; `mode=max` exports intermediate layers to the GitHub Actions cache.
The existing build context excludes tests, local data, caches, and generated
frontend output, so broadening `.dockerignore` is not indicated. The failed run
did not retain disk metrics, therefore the precise peak remains to be measured
by a hosted run.

## Decisions

1. Keep real backend and production web image builds and NAS Compose validation.
   Preserve the backend `mode=max` cache because it avoids recompiling costly
   media stages; measure rather than guess at the cache's footprint.
2. Add a 10 GiB free-space admission check immediately before backend build.
   It fails with measured free space and an actionable recovery message.
3. On GitHub-hosted runners only, clean apt downloads and remove the exact
   unused Android, CodeQL, .NET, Haskell, and Boost toolchain directories. The
   script checks an explicit allowlist, rejects symlinks/path changes, and
   reports filesystem recovery. It does not prune Docker images.
4. After the backend action completes its `type=gha` cache export, prune only
   the currently selected setup-buildx-action builder. Its cache is temporary
   to this job; the web build uses another cache scope.
5. Record `df -h`, `df -i`, `docker system df`, available bytes, and
   `docker buildx du` at relevant milestones. Run the named issue branch on
   push so it can be validated without a PR.

## Recovery

If cleanup cannot reach the admission threshold or the backend still exhausts
storage, retain the failed diagnostics and evaluate a larger runner, a
maintained prebuilt media-dependency image, or a cache strategy change with
measured build-time and storage impact. Do not suppress the failure or remove
runtime dependencies.

## Compatibility

There are no Dockerfile, Compose, application, runtime, or NAS changes. The
existing PR and main-push triggers and all functional checks remain enabled;
the issue branch is also included in push validation so the final change can
be tested without a PR.
