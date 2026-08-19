import type { AppSetting, AssetConversionOverrideState, MVForgePreferences, MVForgeQualityGoal, ProfileInput } from './api/types';
import { applyHardwareQualityPreset } from './utils/hardwareQualityPresets';

export const defaultMVForgePreferences: MVForgePreferences = {
  qualityGoal: 'balanced',
  executionPreference: 'software',
  preferredVideoEncoder: 'auto',
  preferredLanguages: ['jpn', 'spa', 'eng'],
};

const qualityDefaults: Record<MVForgeQualityGoal, { crf: number; hardwarePreset: string }> = {
  maximum_savings: { crf: 26, hardwarePreset: 'compact' },
  balanced: { crf: 22, hardwarePreset: 'recommended' },
  conservative: { crf: 20, hardwarePreset: 'best_quality' },
  maximum_quality: { crf: 18, hardwarePreset: 'high_quality' },
  archive: { crf: 16, hardwarePreset: 'archive' },
};

export function getMVForgePreferences(settings?: AppSetting[]): MVForgePreferences {
  const raw = settings?.find((setting) => setting.key === 'mvforgePreferences')?.value ?? {};
  const qualityGoal = isQualityGoal(raw.qualityGoal) ? raw.qualityGoal : defaultMVForgePreferences.qualityGoal;
  const executionPreference = raw.executionPreference === 'hardware' ? 'hardware' : 'software';
  const preferredVideoEncoder = typeof raw.preferredVideoEncoder === 'string' && raw.preferredVideoEncoder.trim()
    ? raw.preferredVideoEncoder.trim()
    : 'auto';
  const preferredLanguages = Array.isArray(raw.preferredLanguages)
    ? uniqueStrings(raw.preferredLanguages.filter((value): value is string => typeof value === 'string'))
    : [];
  return {
    qualityGoal,
    executionPreference,
    preferredVideoEncoder,
    preferredLanguages: preferredLanguages.length ? preferredLanguages : defaultMVForgePreferences.preferredLanguages,
  };
}

export function applyMVForgeVideoPreferences(
  draft: ProfileInput,
  preferences: MVForgePreferences,
  availableEncoders: Iterable<string>,
): ProfileInput {
  const available = new Set(availableEncoders);
  const hardwareEncoder = preferredAvailableHardwareEncoder(preferences.preferredVideoEncoder, available);
  const hardware = preferences.executionPreference === 'hardware' && Boolean(hardwareEncoder);
  const defaults = qualityDefaults[preferences.qualityGoal];
  const baseConfig: Record<string, unknown> = {
    ...(draft.workerConfig ?? {}),
    preferredEncoder: hardware ? 'hardware' : 'software',
    useHardwareIfAvailable: hardware,
    videoEncoder: hardware ? hardwareEncoder : 'auto',
  };
  const workerConfig = hardware
    ? applyHardwareQualityPreset(baseConfig, hardwareEncoder, defaults.hardwarePreset)
    : baseConfig;
  return {
    ...draft,
    optimizationIntent: preferences.qualityGoal,
    qualityValue: draft.qualityMode === 'crf' ? defaults.crf : draft.qualityValue,
    workerConfig,
  };
}

export function assetOverridePreferenceDraft(
  preferences: MVForgePreferences,
  availableEncoders: Iterable<string>,
): AssetConversionOverrideState {
  const profile = applyMVForgeVideoPreferences({
    name: '', description: '', container: 'mkv', videoCodec: 'x265', audioCodec: 'copy', qualityMode: 'crf', qualityValue: 22,
    preserveHdr: true, preserveSubtitles: true, preserveChapters: true, workerConfig: {},
  }, preferences, availableEncoders);
  const preferredEncoder = String(
    profile.workerConfig?.videoEncoder ?? 'auto',
  );

  const videoCodec =
    preferredEncoder === 'auto'
      ? profile.videoCodec
      : videoCodecForPreferredEncoder(preferredEncoder);
  return {
    videoCodec,
    qualityMode: profile.qualityMode,
    qualityValue: profile.qualityValue,
    optimizationIntent: preferences.qualityGoal,
    preferredEncoder: profile.workerConfig?.preferredEncoder as AssetConversionOverrideState['preferredEncoder'],
    useHardwareIfAvailable: profile.workerConfig?.useHardwareIfAvailable === true,
    videoEncoder: String(profile.workerConfig?.videoEncoder ?? 'auto'),
    hardwareQualityPreset: String(profile.workerConfig?.hardwareQualityPreset ?? ''),
  };
}

export function preferredLanguages(settings?: AppSetting[]) {
  return getMVForgePreferences(settings).preferredLanguages;
}

function preferredAvailableHardwareEncoder(preferred: string, available: Set<string>) {
  if (preferred !== 'auto' && available.has(preferred) && isHardwareEncoder(preferred)) return preferred;
  return ['hevc_qsv', 'hevc_videotoolbox', 'hevc_nvenc', 'h264_qsv', 'h264_videotoolbox', 'h264_nvenc']
    .find((encoder) => available.has(encoder)) ?? '';
}

function isHardwareEncoder(value: string) {
  return /_(qsv|videotoolbox|nvenc|vaapi)$/.test(value);
}

function isQualityGoal(value: unknown): value is MVForgeQualityGoal {
  return ['maximum_savings', 'balanced', 'conservative', 'maximum_quality', 'archive'].includes(String(value));
}

function uniqueStrings(values: string[]) {
  return [...new Set(values.map((value) => value.trim().toLowerCase()).filter(Boolean))];
}

function videoCodecForPreferredEncoder(encoder: string) {
  const value = encoder.trim().toLowerCase();

  if (
    value === 'libx264' ||
    value.startsWith('h264_')
  ) {
    return 'x264';
  }

  if (
    value === 'libx265' ||
    value.startsWith('hevc_')
  ) {
    return 'x265';
  }

  return 'x265';
}