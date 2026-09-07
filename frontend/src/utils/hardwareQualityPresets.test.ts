import { describe, expect, it } from 'vitest';
import { applyHardwareQualityPreset } from './hardwareQualityPresets';

describe('QSV hardware quality preset MBBRC defaults', () => {
  it('resets named QSV presets to Auto', () => {
    expect(applyHardwareQualityPreset({ qsvMBBRCMode: 'enabled' }, 'hevc_qsv', 'best_quality').qsvMBBRCMode).toBe('auto');
  });

  it('preserves Custom MBBRC intent', () => {
    expect(applyHardwareQualityPreset({ qsvMBBRCMode: 'enabled' }, 'hevc_qsv', 'custom').qsvMBBRCMode).toBe('enabled');
  });
});
