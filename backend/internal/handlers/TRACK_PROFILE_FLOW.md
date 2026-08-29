# Current Track Profile flow and migration boundary

Current persisted and execution flow:

```text
AppSetting `trackProfiles` JSON
  -> profile assignment and inheritance
  -> `resolveTrackProfileForAsset`
  -> `QueueJob.TrackProfileSnapshot`
  -> worker merges snapshot below explicit Asset overrides
  -> `MediaJobPlan.Override`
  -> `ResolvedStreamPlan`
  -> FFmpeg map/codec arguments
```

Path profiles persist semantic language/default/forced/commentary rules. Their
concrete indexes, metadata, and subtitle transforms are removed on storage and
resolved against each asset snapshot before Queue freezes them. Asset profiles
may persist exact video, audio, and subtitle selections, including an explicit
empty selection.

## Current split authority

- Track Profile and Asset overrides select embedded video/audio/subtitle indexes
  and describe subtitle sidecar transforms.
- Video `Profile.PreserveSubtitles` and `externalSubtitleFormat` still influence
  embedded subtitle mapping and conversion when no Track Profile is selected.
- Video `Profile.PreserveChapters` and the matching Asset override still decide
  chapter mapping.
- Video `WorkerConfig.removeAttachments` exists in profile/LAB configuration,
  while the current main stream plan otherwise copies attachments.
- `SubtitleTransforms.removeEmbedded` independently removes an embedded
  subtitle while requesting a sidecar, so subtitle survival currently has more
  than one source of truth.

`resolveTrackPlan` is now the canonical backend resolver. Queue stores its
per-asset result under `TrackProfileSnapshot.resolvedTrackPlan`, alongside the
legacy flattened fields required by existing jobs. Effective Test Encode uses
that Queue snapshot path, and LAB draft track profiles call the same resolver.
The worker restores the frozen plan into `MediaJobPlan.ResolvedTracks`.

FFmpeg rendering now consumes `ResolvedTracks` for embedded video, audio,
subtitle, attachment, and chapter disposition. It does not re-evaluate Track
Profile semantics. Jobs without a canonical snapshot keep the legacy selection
path for backward compatibility.

Canonical `extract` and `keep_and_extract` decisions create tracked
`SubtitleArtifacts` before the primary media command. Original ASS, SSA,
SubRip, and PGS representations are stream-copied into staging. Unsupported
codecs fail explicitly and preserve artifact status/error evidence.

Profiles without the new policies resolve compatibly to preserve selected
subtitles, attachments, and chapters. Explicit empty asset selections remain
distinct from absent selection. Jobs created before `resolvedTrackPlan` remain
readable and continue through the legacy fields.
