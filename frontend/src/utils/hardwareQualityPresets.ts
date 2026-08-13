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

// Named presets are quality intent only. Encoder-specific values come from
// POST /assets/quality-recommendation and are never calculated in TypeScript.
export function applyHardwareQualityPreset(config: HardwareConfig, encoder: string, preset: string): HardwareConfig {
  const main10Preset = preset === 'recommended' || preset === 'best_quality';
  return {
    ...config,
    videoEncoder: encoder,
    hardwareQualityPreset: preset,
    hardwareQualityPresetScale: 2,
    ...(encoder === 'hevc_qsv' && preset !== 'custom' ? { qsvRateControl: 'icq' } : {}),
    ...(main10Preset && encoder === 'hevc_qsv' ? { pixFmt: 'p010le' } : {}),
    ...(main10Preset && encoder === 'hevc_videotoolbox' ? { pixFmt: 'p010le', videoToolboxProfile: 'main10' } : {}),
  };
}

export function markHardwareQualityCustom(config: HardwareConfig): HardwareConfig {
  const next: HardwareConfig = { ...config, hardwareQualityPreset: 'custom' };
  delete next.qsvRequestedGlobalQuality;
  delete next.qsvEffectiveGlobalQuality;
  delete next.qsvAssetQualityAdjustment;
  delete next.qsvAssetQualityReasons;
  return next;
}

export function qsvAssetQualitySummary(config: HardwareConfig): string | undefined {
  const requested = Number(config.qsvRequestedGlobalQuality);
  const effective = Number(config.qsvEffectiveGlobalQuality);
  if (!Number.isFinite(requested) || !Number.isFinite(effective) || requested === effective) return undefined;
  const reasons = Array.isArray(config.qsvAssetQualityReasons)
    ? config.qsvAssetQualityReasons.filter((reason): reason is string => typeof reason === 'string' && reason.length > 0)
    : [];
  return `Asset adjustment: ICQ ${requested} → ${effective}${reasons.length > 0 ? ` · ${reasons.join(' ')}` : ''}`;
}
