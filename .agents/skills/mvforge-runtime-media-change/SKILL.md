---
name: mvforge-runtime-media-change
description: Implement or review MVForge FFmpeg, ffprobe, hardware acceleration, runtime capability, scheduler-resource, or container media-stack changes. Use for QSV, VAAPI, VideoToolbox, NVENC, codec capability detection, runtime snapshots, FFmpeg builds, and backend image/runtime work.
license: MIT
metadata:
  project: MVForge
  version: "1.0"
---

# MVForge Runtime and Media Change

Use this skill for media-runtime and hardware-sensitive work.

## Grounding

Read applicable `AGENTS.md` first.

Use Serena MCP to locate:
- runtime/capability detector;
- requested vs effective configuration resolvers;
- command builders;
- scheduler/resource-policy consumers;
- directly relevant tests.

Use targeted text search only for FFmpeg option strings, package names, Dockerfile content, environment variables, or generated command fragments.

## Hardware capability rules

- Never assume an encoder option is supported because FFmpeg documents it generally.
- Capabilities come from the actual worker/runtime probe.
- Keep requested configuration separate from effective configuration.
- Record fallback/downgrade reasons.
- Do not silently switch codec, quality mode, pixel format, level/tier, or hardware backend.
- Do not treat CRF, ICQ, LA-ICQ, QSV global quality, NVENC CQ, and VideoToolbox quality as equivalent.

## FFmpeg / ffprobe changes

- Build commands using argument arrays, not shell concatenation.
- Preserve deterministic map/filter/codec ordering when it affects behavior.
- Reuse existing canonical command builders.
- Keep analysis/probing separate from policy decisions.
- Add focused tests around generated arguments and resolved effective behavior.

## Runtime snapshots / scheduling

- Preserve evidence needed to explain execution decisions.
- Do not turn SQLite into an unbounded telemetry store.
- Distinguish current telemetry from immutable execution evidence when applicable.
- Never prune runtime evidence still referenced by execution plans.

## Docker/runtime image work

Before removing packages or files:
- identify the runtime consumer;
- measure size contribution;
- verify feature coverage.

Protect:
- FFmpeg/ffprobe functionality;
- Intel QSV/VAAPI where supported;
- OCR/subtitle runtime dependencies when part of the image contract;
- backup/healthcheck workflows;
- licenses required for redistributed components.

Do not optimize image size by silently dropping supported capabilities.

## Resource-constrained NAS

Consider:
- CPU;
- RAM;
- GPU contention;
- temporary storage;
- parallelism;
- impact on Jellyfin/other colocated workloads.

Prefer bounded concurrency and reusable analysis/evidence.

## Validation

Run focused tests as needed, then:

```bash
./scripts/verify.sh --auto
```

Use `--full` for infrastructure/shared build configuration when required by `AGENTS.md`.

Follow repository commit/push rules after successful validation.

## Final check

Confirm:
- actual capability evidence matches behavior;
- no unsupported flag was invented;
- requested/effective config remain distinct;
- fallback is explicit;
- tests cover the changed runtime decision;
- no supported feature was lost unintentionally.
