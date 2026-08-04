import type { ScanResult } from '../api/types';

export type HardwareQualityPreset = 'compact' | 'medium' | 'recommended' | 'best_quality' | 'high_quality' | 'archive' | 'master' | 'custom';

export const hardwareQualityPresetOptions: Array<{ value: HardwareQualityPreset; label: string }> = [
  { value: 'compact', label: 'Compact' },
  { value: 'medium', label: 'Medium' },
  { value: 'recommended', label: 'Recommended' },
  { value: 'best_quality', label: 'Best' },
  { value: 'high_quality', label: 'High Quality' },
  { value: 'archive', label: 'Archive' },
  { value: 'master', label: 'Master' },
  { value: 'custom', label: 'Custom' },
];

type HardwareConfig = Record<string, unknown>;

// These values are deliberately shared by Profiles, LAB, and Asset Overrides.
// The backend repeats the normalization as the execution authority.
export function applyHardwareQualityPreset(config: HardwareConfig, encoder: string, preset: string, scan?: ScanResult): HardwareConfig {
  const next: HardwareConfig = { ...config, hardwareQualityPreset: preset, hardwareQualityPresetScale: 2 };
  if (preset === 'custom') return next;

  if (encoder === 'hevc_qsv') {
    const values: Record<string, HardwareConfig> = {
      compact: { globalQuality: 36, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      medium: { globalQuality: 33, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      recommended: { globalQuality: 30, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      best_quality: { globalQuality: 27, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'nv12' },
      high_quality: { globalQuality: 25, qsvRateControl: 'icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
      archive: { globalQuality: 22, qsvRateControl: 'la_icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
      master: { globalQuality: 20, qsvRateControl: 'la_icq', qsvLookAheadDepth: 40, qsvExtendedBRC: false, qsvAdaptiveI: false, qsvAdaptiveB: false, pixFmt: 'p010le' },
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
      archive: { videoToolboxProfile: 'main10', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'p010le' },
      master: { videoToolboxProfile: 'main10', videoToolboxGop: 120, videoToolboxAllowFrameReordering: false, videoToolboxPowerEfficiency: true, pixFmt: 'p010le' },
    };
    const configured = { ...next, ...(values[preset] ?? values.recommended), videoToolboxQualityProfile: undefined };
    const rates = adaptiveVideoToolboxPresetKbps(configured, scan);
    return rates ? {
      ...configured,
      videoToolboxBitrateMbps: Math.ceil(rates.target / 1000),
      videoToolboxMaxrateMbps: Math.ceil(rates.maxrate / 1000),
      videoToolboxBufferMbps: Math.ceil(rates.buffer / 1000),
    } : configured;
  }
  return { ...next, globalQuality: ({ compact: 28, medium: 26, recommended: 24, best_quality: 22, high_quality: 20, archive: 18, master: 16 }[preset] ?? 24) };
}

export function adaptiveVideoToolboxPresetKbps(config: HardwareConfig, scan?: ScanResult) {
  if (!scan) return undefined;
  const preset = typeof config.hardwareQualityPreset === 'string' ? config.hardwareQualityPreset : 'custom';
  const multiplier = ({ compact: 0.25, medium: 0.33, recommended: 0.40, best_quality: 0.52, high_quality: 0.65, archive: 0.80, master: 0.95 } as Record<string, number>)[preset];
  const source = scan.videoStreams[0]?.bitrate || scan.bitrate;
  if (!multiplier || source <= 0) return undefined;
  const floors: Record<string, number[]> = {
    compact: [900, 1400, 2000, 4000], medium: [1200, 1800, 2500, 5000], recommended: [1500, 2200, 3000, 6000], best_quality: [2000, 3000, 4000, 8000], high_quality: [2500, 4000, 5000, 10000], archive: [3200, 5000, 6000, 12000], master: [4000, 6500, 7000, 14000],
  };
  const sourceHeight = scan.height || scan.videoStreams[0]?.height || 1080;
  const filters = typeof config.videoFilters === 'string' ? config.videoFilters : '';
  const crop = filters.match(/(?:^|,)crop=\d+:(\d+)/);
  const height = crop?.[1] ? Number(crop[1]) : sourceHeight;
  const index = height <= 576 ? 0 : height <= 720 ? 1 : height <= 1080 ? 2 : 3;
  let target = Math.max(floors[preset][index], Math.ceil(source * multiplier * (height <= 576 ? 1.075 : 1) / 1000));
  const sdCeilings: Record<string, number> = { compact: 1700, medium: 2200, recommended: 2500, best_quality: 3200, high_quality: 4000, archive: 5000, master: 6000 };
  if (height <= 576) target = Math.min(target, sdCeilings[preset]);
  return { target, maxrate: Math.ceil(target * 1.5), buffer: Math.ceil(target * 2.5) };
}

export function markHardwareQualityCustom(config: HardwareConfig): HardwareConfig {
  return { ...config, hardwareQualityPreset: 'custom' };
}
