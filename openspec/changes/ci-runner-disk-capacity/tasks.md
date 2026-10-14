## 1. Capacity controls and observability

- [x] 1.1 Add GitHub-hosted-only bounded cleanup and a 10 GiB pre-build headroom
  check with actionable failure output.
- [x] 1.2 Add filesystem, inode, Docker, and BuildKit diagnostics before/after
  the backend and web builds and after Compose validation.
- [x] 1.3 Preserve real Dockerfiles, the backend `mode=max` cache, and all
  functional image and Compose checks; release backend cache before web build.
- [x] 1.4 Document runner limits, cleanup targets, and recovery alternatives.
- [x] 1.5 Validate YAML, scripts, focused workflow checks, and
  `./scripts/verify.sh --auto`.
- [x] 1.6 Run the GitHub-hosted workflow twice and report measured cleanup
  recovery and post-build cache use; document that transient build peak was not
  sampled.
