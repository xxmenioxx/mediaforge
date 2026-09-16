import { describe, expect, it } from 'vitest';
import {
  assetDefaultProfileReference,
  assetDefaultsValue,
  updateAssetDefaultsDomain,
} from './assetDefaults';

describe('Asset Defaults settings mapping', () => {
  it('shows independent custom source states with compatibility defaults', () => {
    const value = assetDefaultsValue({});

    expect(value.video.source).toBe('custom');
    expect(value.audio.source).toBe('custom');
    expect(value.tracks.source).toBe('custom');
    expect(value.tracks.custom.subtitleSidecarFormats).toEqual(['srt']);
  });

  it('persists a profile reference without copying custom fields', () => {
    const value = updateAssetDefaultsDomain({
      video: { source: 'custom', custom: { videoCodec: 'copy' } },
    }, 'video', { source: 'profile', profileId: 17, custom: {} });

    expect(assetDefaultProfileReference(value.video)).toBe(17);
    expect(value.video.custom).toEqual({});
  });

  it('updates one domain without changing the other domain policies', () => {
    const original = assetDefaultsValue({
      audio: { source: 'profile', profileKey: 'dialogue' },
    });
    const updated = updateAssetDefaultsDomain(original, 'tracks', {
      custom: { subtitleSidecarFormats: ['both'] },
    });

    expect(updated.audio.source).toBe('profile');
    expect(updated.audio.profileKey).toBe('dialogue');
    expect(updated.tracks.custom.subtitleSidecarFormats).toEqual(['both']);
  });
});
