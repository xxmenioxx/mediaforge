import { describe, expect, it } from 'vitest';
import type { AppSetting, AssetConversionOverrideState } from './api/types';
import { emptyTrackProfile, getTrackProfiles, materializeAssetTrackSelection, migrateTrackDisposition, trackProfileWithConversion } from './trackProfiles';

const conversion: AssetConversionOverrideState = {
  keepVideoStreams: [0],
  keepAudioStreams: [4, 1, 4],
  keepSubtitleStreams: [],
  audioMetadata: { '1': { language: 'jpn' } },
  subtitleTransforms: [{
    streamIndex: 5,
    format: 'srt',
    removeEmbedded: true,
    makeDefault: false,
    language: 'spa',
  }],
};

describe('trackProfileWithConversion', () => {
  it('persists the exact retained indexes, including remove-all, for an asset profile', () => {
    const saved = trackProfileWithConversion({ ...emptyTrackProfile, key: 'asset-tracks', name: 'Asset tracks', scope: 'asset' }, conversion);
    const settings = JSON.parse(JSON.stringify([{ key: 'trackProfiles', value: { profiles: [saved] } }])) as AppSetting[];
    const [reloaded] = getTrackProfiles(settings);

    expect(reloaded.keepVideoStreams).toEqual([0]);
    expect(reloaded.keepAudioStreams).toEqual([1, 4]);
    expect(reloaded.keepSubtitleStreams).toEqual([]);
    expect(reloaded.audioMetadata).toEqual({ '1': { language: 'jpn' } });
    expect(reloaded.subtitleTransforms).toEqual([expect.objectContaining({ streamIndex: 5, removeEmbedded: true })]);
  });

  it('omits asset indexes and per-stream actions from a path profile', () => {
    const saved = trackProfileWithConversion({ ...emptyTrackProfile, key: 'path-tracks', name: 'Path tracks', scope: 'path' }, conversion);

    expect(saved.keepVideoStreams).toBeUndefined();
    expect(saved.keepAudioStreams).toBeUndefined();
    expect(saved.keepSubtitleStreams).toBeUndefined();
    expect(saved.audioMetadata).toBeUndefined();
    expect(saved.subtitleTransforms).toBeUndefined();
  });

  it('materializes all current indexes for an asset instead of falling back to semantic rules', () => {
    const exact = materializeAssetTrackSelection({ keepSubtitleStreams: [] }, {
      videoStreams: [{ index: 0 }, { index: 6 }],
      audioStreams: [{ index: 1 }, { index: 3 }],
      subtitleStreams: [{ index: 5 }],
    });

    expect(exact.keepVideoStreams).toEqual([0, 6]);
    expect(exact.keepAudioStreams).toEqual([1, 3]);
    expect(exact.keepSubtitleStreams).toEqual([]);
  });

  it('keeps display fallbacks separate from the explicit migration marker', () => {
    const settings = [{ key: 'trackProfiles', value: { profiles: [{ key: 'legacy', name: 'Legacy' }] } }] as unknown as AppSetting[];
    const [legacy] = getTrackProfiles(settings);

    expect(legacy.subtitleDisposition).toBe('keep');
    expect(legacy.attachmentPolicy).toBe('auto');
    expect(legacy.chapterPolicy).toBe('keep');
    expect(legacy.trackDispositionVersion).toBeUndefined();
    expect(JSON.parse(JSON.stringify(legacy)).trackDispositionVersion).toBeUndefined();
  });

  it('migrates only the profile whose disposition is intentionally edited', () => {
    const settings = [{ key: 'trackProfiles', value: { profiles: [
      { key: 'legacy', name: 'Legacy' },
      { ...emptyTrackProfile, key: 'canonical', name: 'Canonical' },
    ] } }] as unknown as AppSetting[];
    const [legacy, canonical] = getTrackProfiles(settings);
    const savedCollection = JSON.parse(JSON.stringify([legacy, migrateTrackDisposition(canonical, { chapterPolicy: 'remove' })]));

    expect(savedCollection[0].trackDispositionVersion).toBeUndefined();
    expect(savedCollection[1].trackDispositionVersion).toBe(1);
    expect(savedCollection[1].chapterPolicy).toBe('remove');
    expect(migrateTrackDisposition(legacy, { attachmentPolicy: 'keep' }).trackDispositionVersion).toBe(1);
  });

  it('preserves explicit und and selectorless legacy rules without treating either as all', () => {
    const settings = [{ key: 'trackProfiles', value: { profiles: [{
      key: 'rules', name: 'Rules', subtitleRules: [
        { language: 'und', action: 'remove' },
        { language: '', action: 'keep' },
      ],
    }] } }] as unknown as AppSetting[];
    const [profile] = getTrackProfiles(settings);

    expect(profile.subtitleRules[0]).toEqual({ language: 'und', streamIndex: undefined, action: 'remove' });
    expect(profile.subtitleRules[1]).toEqual({ language: undefined, streamIndex: undefined, action: 'keep' });
  });

  it('round-trips semantic and stream-specific subtitle sidecar formats', () => {
    const settings = [{ key: 'trackProfiles', value: { profiles: [{
      ...emptyTrackProfile,
      key: 'compatibility', name: 'Compatibility', subtitleSidecarFormats: ['original', 'srt'],
      subtitleRules: [{ streamIndex: 4, action: 'keep_and_extract', sidecarFormats: ['srt'] }],
    }] } }] as unknown as AppSetting[];
    const [profile] = getTrackProfiles(settings);

    expect(profile.subtitleSidecarFormats).toEqual(['original', 'srt']);
    expect(profile.subtitleRules[0].sidecarFormats).toEqual(['srt']);
  });
});
