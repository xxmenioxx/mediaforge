export type HardwareQualityPreset = 'compact' | 'medium' | 'recommended' | 'best_quality' | 'high_quality' | 'custom';

export const hardwareQualityPresetOptions: Array<{ value: HardwareQualityPreset; label: string }> = [
  { value: 'compact', label: 'Compact' },
  { value: 'medium', label: 'Medium' },
  { value: 'recommended', label: 'Recommended' },
  { value: 'best_quality', label: 'Best' },
  { value: 'high_quality', label: 'High Quality' },
  { value: 'custom', label: 'Custom' },
];

type HardwareConfig = Record<string, unknown>;

// These values are deliberately shared by Profiles, LAB, and Asset Overrides.
// The backend repeats the normalization as the execution authority.
export function applyHardwareQualityPreset(config: HardwareConfig, encoder: string, preset: string): HardwareConfig {
  const next: HardwareConfig = { ...config, hardwareQualityPreset: preset };
  if (preset === 'custom') return next;

  if (encoder === 'hevc_qsv') {
    const values: Record<string, HardwareConfig> = {
      compact: { globalQuality: 30, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      medium: { globalQuality: 27, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      recommended: { globalQuality: 25, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
      best_quality: { globalQuality: 22, qsvRateControl: 'la_icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
      high_quality: { globalQuality: 20, qsvRateControl: 'la_icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
    };
    return { ...next, ...(values[preset] ?? values.recommended) };
  }

  if (encoder === 'hevc_videotoolbox') {
    const values: Record<string, HardwareConfig> = {
      compact: { videoToolboxProfile: 'main', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'yuv420p' },
      medium: { videoToolboxProfile: 'main', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'yuv420p' },
      recommended: { videoToolboxProfile: 'main10', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'p010le' },
      best_quality: { videoToolboxProfile: 'main10', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'p010le' },
      high_quality: { videoToolboxProfile: 'main10', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'p010le' },
    };
    return { ...next, ...(values[preset] ?? values.recommended), videoToolboxQualityProfile: undefined };
  }
  return { ...next, globalQuality: ({ compact: 24, medium: 22, recommended: 20, best_quality: 18, high_quality: 16 }[preset] ?? 20) };
}

export function markHardwareQualityCustom(config: HardwareConfig): HardwareConfig {
  return { ...config, hardwareQualityPreset: 'custom' };
}
