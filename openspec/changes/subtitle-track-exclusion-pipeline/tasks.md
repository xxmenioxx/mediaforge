## 1. Freeze explicit subtitle selection

- [x] 1.1 Make the existing canonical track resolver retain explicit asset exclusions ahead of automatic subtitle rules; verified with `TestResolveTrackPlanAssetSubtitleSelectionOutranksCanonicalRulesAndOperations` and `TestFreezeEffectiveTrackPlanPreservesAssetOverridePrecedence`.
- [x] 1.2 Preserve explicit selection during Video Profile externalization and map construction; verified `[2,3,4]` / `[2,4]`, SRT/ASS, empty selection, and embedded FFmpeg mappings in focused `media_command_test.go` regressions.

## 2. Stop excluded work before subtitle operations

- [x] 2.1 Filter excluded indexes before legacy transform execution and canonical sidecar execution; `TestExcludedLegacySubtitleStreamsNeverInvokeExtractionOCROrConversion`, `TestExcludedCanonicalSubtitleSidecarNeverInvokesSubtitleTools`, and `TestEmptyAssetSubtitleSelectionNeverInvokesSubtitleOperations` assert fake FFmpeg/ffprobe/converter/OCR tools are not invoked for excluded ASS, SRT, and PGS streams.
- [x] 2.2 Keep `SubtitleArtifacts`, progress, and Worker stages limited to authorized subtitle work; `TestRemovedResolvedStreamHasNoPlannedSubtitleArtifactOrStageWork` and the empty-selection tests verify no sidecar or subtitle stage is scheduled, while independent font work retains its stage.
- [x] 2.3 Verify selected sidecars, final FFmpeg mappings, Validation inputs, and Publisher outputs agree; `TestProfileExternalizesOnlySelectedSubtitleStreams`, `TestExcludedSameLanguageSubtitleNeverReachesFFmpeg`, `TestFFmpegCommandBuilderMapsOnlySelectedEmbeddedSubtitleIndexes`, and `TestValidationAndPublisherIgnoreArtifactsOutsideTheFrozenTrackPlan` cover SRT/ASS, duplicate languages, excluded mapping, and preserved video/audio/chapter/attachment maps.

## 3. Preserve Queue freeze and legacy behavior

- [x] 3.1 Cover Queue Preflight and the accepted frozen execution for assets with and without a Track Profile; `TestBatchPreflightAcceptsTheExactReviewedFrozenPlan` verifies the no-Track-Profile selection survives acceptance, and `TestQueuePreflightFreezesAssetSubtitleExclusionAheadOfTrackProfileRules` plus resolver freeze tests verify canonical dispositions with a Track Profile.
- [x] 3.2 `TestFrozenAssetSubtitleSelectionSurvivesSelectionChangesOnRetryAndRestart` proves retries/restarts keep the captured `[2,4]` after the asset changes to `[3]`, while a new job sees `[3]`; `TestAssetTracksPatchLeavesHistoricalQueueSnapshotsUnchanged` and `TestSubtitleArtifactSelectionPreservesNilEmptyAndLegacySnapshotSemantics` cover immutable historical snapshots and legacy artifact behavior.

## 4. Validate and record evidence

- [x] 4.1 `git diff --check` passed; `openspec validate subtitle-track-exclusion-pipeline --strict` reported the change valid; final `./scripts/verify.sh --auto` passed all 6/6 checks with `ALL CHECKS PASSED`. The frontend build emitted only its bundle-size warning after dependencies were installed from the lockfile.
