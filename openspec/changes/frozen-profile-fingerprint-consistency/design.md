## Context

See `proposal.md` for the reported mismatch. Today `CreateTestEncode` stores `ResolvedProfileFingerprint` before `buildMediaJobPlanWithOverride` finalizes `plan.Profile`; Qualification decodes `effectiveConfiguration.profile` after that plan and hashes it with the same helper. That helper hashes all of `Profile.Definition().WorkerConfig`, including `qsvAssetAnalysisPath`. Queue Preflight independently restores its frozen profile snapshot and hashes that reconstructed value. Snapshot schema v1 also omits `TemplateKey` and `TemplateVersion`, although the current fingerprint includes them. Finally, `ConfigurationHash` hashes a broader runtime record containing executor and runtime snapshot identifiers, and #69 uses that value as the candidate's configuration identity.

Queue profile capture already resolves asset overrides and freezes persisted cadence, interlace, frame structure, color, upscale, and restoration inputs from `ScanResult`. Test Encode then builds a media plan that applies additional automatic video decisions before FFmpeg. Qualification currently has no persisted identity that distinguishes the reusable profile from that final plan.

## Goals / Non-Goals

**Goals:**

- Give the reusable profile definition and the effective video profile used by FFmpeg separate, versioned fingerprints.
- Make Queue Preflight and Test Encode capture comparable effective profile fingerprints through existing resolution stages and frozen snapshots.
- Keep `ConfigurationHash` as the full candidate configuration identity consumed by #69, and bind it to the effective profile fingerprint without including run-only metadata.
- Keep source, Test Encode generation, SamplingPlan, evidence/reference, and policy identities independently checked.
- Preserve legacy snapshots and qualification rows without rewriting them or requiring a re-encode just to inspect them.

**Non-Goals:**

- Implement #306 or any profile candidate-search flow.
- Change profile assignment precedence, Quality policy thresholds, or final Validation behavior.
- Add media scans or FFmpeg executions solely to display or re-evaluate old evidence.

## Decisions

1. **Use three identity layers with one canonical owner per layer.**
   - `ResolvedProfileFingerprint` becomes the versioned reusable definition identity. Compute it from the chosen stored profile before assignment overlays, Asset Overrides, or per-asset analysis context are applied. It includes the definition and profile/template version; it excludes database ID, display metadata, assignment scope/source, and asset/runtime paths. A LAB draft uses the same canonical definition shape with explicit draft provenance.
   - `EffectiveProfileFingerprint` identifies the resolved video profile after the existing automatic resolvers have applied the effective encoder, quality mode/value, HEVC level, GOP/B-frames, cadence, color, restoration, and other output-affecting settings. Compute it from the exact frozen `plan.Profile` used to build FFmpeg arguments. Exclude output paths, analysis-source paths, capture timestamps, executor labels, and runtime snapshot IDs; preserve the underlying output-affecting decisions.
   - `ConfigurationHash` remains the candidate-level identity for the whole Test Encode configuration, including the effective profile fingerprint and the existing effective override/stream plan. Keep runtime/executor details in `EffectiveConfiguration` for diagnostics but outside this identity. #69 continues to bind `CandidateIdentity.ConfigurationIdentity` to this value and also retains Test Encode ID/generation, source revision, segment, and reference identities.

2. **Persist the definition identity before mutating the profile, then preserve it through freeze.**
   Capture the definition identity at the existing profile snapshot boundary before `applyAssetDefaultsAudioToVideoProfile` or `applyAssetConversionOverrideToProfile` changes the profile. Carry it as explicit versioned metadata in the Queue `ProfileSnapshot`; the final Queue freeze copies this metadata while replacing the resolved profile payload. Test Encode copies the same identity into its requested snapshot. Snapshot restore may continue to rebuild the executable `Profile`; identity metadata is read separately and is never inferred from a partially reconstructed profile.

3. **Use one effective-profile identity helper and make Preflight resolve the same profile stage.**
   Test Encode calculates the effective profile identity after `buildMediaJobPlanWithOverride` has finished resolving `plan.Profile`, then persists the profile, both fingerprints, and `ConfigurationHash` in the same conditional database update before starting FFmpeg. Queue Preflight computes the corresponding identity from its frozen resolved profile, selected encoder, and current persisted source-analysis facts using the same existing video resolvers; it fingerprints the restored snapshot representation so later readers hash the same canonical data. It does not launch FFmpeg or refresh media analysis for identity calculation. Missing or stale facts leave that identity unavailable or stale and prevent a current Qualification from being attached. The helper excludes per-asset analysis paths but retains resolved decisions, so equal settings yield equal identities and real effective changes remain distinguishable.

4. **Treat Qualification and evidence as immutable references to the frozen Test Encode.**
   Add the effective-profile fingerprint to `ProfileQualification` alongside the existing definition fingerprint. Evaluation reads only the persisted Test Encode request/effective snapshots and #69 evidence, verifies the profile-definition identity, effective-profile identity, full `ConfigurationHash`, source/candidate generation, SamplingPlan, evidence, reference, and policy identities, then emits its existing rule results. The evidence matcher reports the first failed identity with its stage and expected/observed provenance before returning. It does not reload or hash the mutable current Profile.

5. **Fail closed for legacy identity gaps without a data rewrite.**
   New identity strings carry their scheme version. Existing Test Encode/Queue snapshots and Qualification rows are not backfilled. When required versioned fields are absent or have legacy semantics, expose a specific stale/review cause and leave the historical outcome unchanged. Qualification remains advisory in Preflight; final Validation is untouched.

Alternatives considered: hashing one mutable `Profile` at every stage recreates the pre-plan/post-plan mismatch and leaks asset paths into reusable identity; comparing only profile ID/version or definition identity can reuse evidence across changed execution settings; re-resolving the current Profile while viewing evidence violates frozen-snapshot semantics; rebuilding old Test Encodes would alter history and spend NAS resources without repairing their original provenance.

## Risks / Trade-offs

- **Preflight's persisted ScanResult may not contain current facts required for the effective profile identity** → Keep identity unavailable with an actionable reason and do not show a current Qualification; do not run a new probe just to attach evidence.
- **Canonicalization may omit a setting that changes FFmpeg output** → Build the identity payload from the existing resolved profile and explicit effective decisions, and add sensitivity regressions for QSV, HEVC, GOP/B-frames, cadence, color, restoration, and overrides.
- **Older records cannot prove the new identity relationship** → Preserve their rows and snapshots; mark them stale/review-required without synthetic outcomes or automatic re-encoding.
- **Additive model fields are deployed on existing SQLite databases** → Use the existing idempotent GORM AutoMigrate path and test opening a database with legacy Test Encode/Qualification rows.

## Migration Plan

1. Add the versioned definition/effective profile identity fields through existing AutoMigrate behavior; do not backfill historical values.
2. Write new versioned metadata into newly captured Queue and Test Encode snapshots, and persist the final Test Encode identities with the effective plan before FFmpeg starts.
3. Continue restoring legacy ProfileSnapshot schema v1. A missing identity envelope is a review condition, not a restore error or a reason to rewrite the snapshot.
4. Re-evaluate only when an operator requests it. Append a new Qualification only when all frozen identities can be proven; otherwise preserve history and return the specific stale/review cause.
5. Rollback is code-only: older binaries can ignore additive JSON metadata/columns; no media, Queue history, or old snapshot needs rewriting.
