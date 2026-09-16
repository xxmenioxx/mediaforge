## Context

Issue #23 introduces a cross-cutting fallback policy. Current resolution already
has separate profile, asset override, group, capability, subtitle, Queue, and
Worker boundaries. The living `openspec/specs/subtitles/spec.md` is the
authoritative existing subtitle contract; `docs/architecture/subtitle-pipeline.md`
is supporting context only. See proposal.md and `specs/asset-defaults/spec.md`
for the requested behavior.

## Goals / Non-Goals

**Goals:**

- Persist one independently selectable Custom/Profile source per Video, Audio, and Tracks domain.
- Insert Asset Defaults at the existing backend resolution boundary without duplicating domain resolvers.
- Preserve the existing `subtitles` capability as the sole owner of subtitle format selection, OCR, normalization, artifact identity, and validation.
- Produce one effective decision used by Preview, Test Encode, Queue, and Worker preparation, with immutable Queue snapshots.
- Preserve current non-destructive behavior for new and existing installations.

**Non-Goals:**

- Redesigning Video, Audio, Track Profiles, subtitle OCR, or subtitle artifact semantics.
- Adding new OCR modes, sidecar discovery rules, media storage behavior, publication behavior, translation, or unrelated Settings controls.
- Replacing explicit asset/path/logical-group/source-group assignments with global defaults.

## Decisions

1. **Use one typed Asset Defaults setting.** Store a versioned setting under the existing application-settings mechanism, with independent domain source state (`custom` or an existing profile reference) and only the Custom fields needed by that domain. This keeps configuration durable and lets the UI show source state without copying profile contents. A separate settings table was rejected because it would create a second persistence/migration path for a single project-wide policy.

2. **Resolve defaults at the backend domain boundary.** Extend the existing effective Video, Audio, and Track resolution inputs so each resolver receives the resolved Asset Defaults only after more-specific scopes have been evaluated. The resolver must retain explicit value, inherit, and disabled distinctions. A React-only fallback was rejected because Preview, Test Encode, Queue, and Worker would diverge.

3. **Keep profile references lazy.** A selected existing Video, Audio, or Track Profile is resolved through the current profile/assignment machinery at evaluation time, then its effective result is captured in Preview/Test Encode/Queue outputs. Profile fields are not copied into Asset Defaults. Missing or disabled references produce an explicit resolution warning and safe built-in fallback.

4. **Reuse the canonical subtitle planner unchanged in responsibility.** Tracks General supplies only the fallback disposition, requested output formats, attachment/font policy, and existing OCR configuration. The existing subtitle capability remains responsible for source classification, one logical extraction/OCR for `both`, cue reuse, SRT/ASS rendering, validation, naming, and artifact metadata. A new Asset Defaults subtitle/OCR path was rejected because it would violate issue #23's canonical-pipeline requirement.

5. **Represent `both` as one logical subtitle transform with two artifacts.** The planner records one source/transform identity and two requested render formats. Text normalization or bitmap OCR occurs once; SRT and ASS renderers consume the same normalized cue collection. Duplicate execution is prevented by the plan identity rather than by new sidecar matching semantics.

6. **Freeze effective defaults with the existing Queue snapshot lifecycle.** Queue capture stores the resolved domain decisions and canonical subtitle artifact plan; Worker preparation consumes that snapshot. Settings edits are visible to future resolution only. Existing snapshots lacking new fields remain authoritative and are not silently rewritten during retry or execution.

7. **Use runtime capability data for Video encoder choices.** The Settings response and UI consume the existing worker/runtime capability probe and include Copy as the non-encoding choice. The backend remains authoritative for availability and effective encoder fallback; the UI does not hard-code capabilities.

8. **Keep attachment and font policies separate.** Asset Defaults maps the two user-facing policies into the existing attachment/font dependency resolver. ASS/SSA warnings and validation remain the final authority, and global defaults do not infer font dependencies for converted neutral text artifacts.

## Risks / Trade-offs

- **Risk →** A selected profile may be edited after Asset Defaults selection. **Mitigation →** store only its reference for future jobs; freeze its effective result in each Queue snapshot, matching existing profile behavior.
- **Risk →** Defaults could accidentally override explicit disabled states. **Mitigation →** use the existing tri-state precedence model and add focused precedence regressions for every domain.
- **Risk →** `both` could accidentally run OCR twice. **Mitigation →** make the shared normalized-cue result an explicit planner output and test command/OCR invocation counts.
- **Risk →** Runtime encoder capabilities vary by worker. **Mitigation →** resolve capabilities at the existing worker/runtime boundary and report unsupported choices rather than inventing them.
- **Risk →** Existing installations could become destructive after migration. **Mitigation →** initialize to behavior-equivalent, non-destructive fallback values and require explicit user changes for removal policies.

## Migration Plan

1. Add the versioned Asset Defaults setting with compatibility-preserving defaults and additive API/UI representation.
2. Thread the resolved fallback through existing Video, Audio, and Track resolution paths, preserving explicit scope precedence.
3. Persist the effective result and canonical subtitle artifact plan in new Queue snapshots while leaving existing snapshots authoritative.
4. Add focused backend and frontend resolution/parity tests, then run `./scripts/verify.sh --auto`.
5. Rollback is code/configuration-only; do not rewrite media, sidecars, or existing Queue snapshots.
