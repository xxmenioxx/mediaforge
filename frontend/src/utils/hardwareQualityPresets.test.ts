import { describe, expect, it } from 'vitest';
import { applyHardwareQualityPreset } from './hardwareQualityPresets';

describe('QSV hardware quality preset contextual feature defaults', () => {
  it('resets named QSV presets to Auto', () => {
    const result = applyHardwareQualityPreset({ qsvMBBRCMode: 'enabled', qsvRDOMode: 'enabled' }, 'hevc_qsv', 'best_quality');
    expect(result.qsvMBBRCMode).toBe('auto');
    expect(result.qsvRDOMode).toBe('auto');
  });

  it('preserves Custom MBBRC and RDO intent', () => {
    const result = applyHardwareQualityPreset({ qsvMBBRCMode: 'enabled', qsvRDOMode: 'disabled' }, 'hevc_qsv', 'custom');
    expect(result.qsvMBBRCMode).toBe('enabled');
    expect(result.qsvRDOMode).toBe('disabled');
  });
});
