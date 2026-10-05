import { describe, expect, it } from 'vitest';
import type { EffectiveAssetConfigurationValue } from '../api/types';
import { effectiveVideoProfileLabel, testEncodeConfigurationReason } from './testEncodeEligibility';

function video(value: Partial<EffectiveAssetConfigurationValue>): EffectiveAssetConfigurationValue {
  return value as EffectiveAssetConfigurationValue;
}

describe('effective Test Encode video eligibility', () => {
  it('allows an effective persisted profile and shows its canonical label', () => {
    const configuration = video({ selection: 'profile', videoProfileId: 7, source: 'path' });
    expect(testEncodeConfigurationReason(configuration)).toBe('');
    expect(effectiveVideoProfileLabel(configuration, [{ id: 7, name: 'Path HEVC' }])).toBe('Path HEVC');
  });

  it('allows and labels a valid Global Asset Defaults Custom video configuration', () => {
    const configuration = video({ selection: 'value', source: 'global', profileData: { videoCodec: 'libx265' } });
    expect(testEncodeConfigurationReason(configuration)).toBe('');
    expect(effectiveVideoProfileLabel(configuration, [])).toBe('Custom');
  });

  it.each(['inherit', 'audio_only', 'override_only', 'disabled'])(
    'keeps %s unavailable even when a stale profile id is present',
    (selection) => {
      const configuration = video({ selection, videoProfileId: 7, source: 'asset' });
      expect(testEncodeConfigurationReason(configuration)).not.toBe('');
      expect(effectiveVideoProfileLabel(configuration, [{ id: 7, name: 'Stale profile' }])).not.toBe('Stale profile');
    },
  );

  it('does not mistake non-global Custom-like data for a supported Test Encode base', () => {
    const configuration = video({ selection: 'value', source: 'path', profileData: { videoCodec: 'libx265' } });
    expect(testEncodeConfigurationReason(configuration)).not.toBe('');
    expect(effectiveVideoProfileLabel(configuration, [])).toBe('Unavailable');
  });

  it('reports missing effective configuration as unavailable', () => {
    expect(testEncodeConfigurationReason(undefined)).toMatch(/loading or unavailable/i);
    expect(effectiveVideoProfileLabel(undefined, [])).toBe('Unavailable');
  });
});
