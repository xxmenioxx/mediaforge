import { describe, expect, it } from 'vitest';
import type { AppSetting, AssetConversionOverrideState } from './api/types';
import { emptyTrackProfile, getTrackProfiles, materializeAssetTrackSelection, trackProfileWithConversion } from './trackProfiles';

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
});
