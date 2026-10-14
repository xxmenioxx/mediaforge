## ADDED Requirements

### Requirement: Container builds observe runner storage

The `containers` workflow MUST report filesystem capacity and inode use with
`df -h` and `df -i`, and Docker usage with `docker system df`, at job start,
before and after image builds, and after NAS Compose validation. BuildKit
capacity MUST be reported when the Buildx builder is available.

#### Scenario: Backend build has insufficient headroom

- **WHEN** available filesystem space immediately before the backend image
  build is below 10 GiB
- **THEN** the job fails before invoking the build action
- **AND** the failure reports available space and capacity recovery options

### Requirement: Hosted runner cleanup is bounded

The workflow MUST limit pre-build cleanup to documented unused toolchain and
package-download targets on GitHub-hosted runners. Cleanup MUST reject
unallowlisted paths and MUST NOT follow or delete symlinked targets. It MUST
NOT remove Docker images or required image-build dependencies.

#### Scenario: Job uses a self-hosted runner

- **WHEN** the runner environment is not `github-hosted`
- **THEN** hosted-runner cleanup and post-backend cache pruning do not run

### Requirement: Production container checks remain representative

The workflow MUST build the real backend and production web images, preserve
runtime dependencies and cache provenance, and validate NAS Compose.

#### Scenario: Backend cache is released before web build

- **WHEN** the backend build and its remote cache export complete successfully
- **THEN** CI may prune the selected job-local Buildx cache before the web build
- **AND** the backend image build, web image build, and NAS Compose check remain
  required workflow steps
