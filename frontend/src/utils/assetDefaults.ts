import type { AssetDefaultsDomain, AssetDefaultsValue } from '../api/types';

export type { AssetDefaultsDomain, AssetDefaultsSource, AssetDefaultsValue } from '../api/types';

const defaults: AssetDefaultsValue = {
  schemaVersion: 1,
  video: { source: 'custom', custom: { videoCodec: 'copy' }, removeNonPrimaryVideoTracks: false },
  audio: {
    source: 'custom',
    custom: {
      addCompatibleTrack: false,
      compatibleTrackQuality: 192,
      compatibleTrackChannels: 2,
      compatibleTrackSource: 'resolved',
      preferredDefaultLanguage: '',
    },
  },
  tracks: {
    source: 'custom',
    custom: {
      trackDispositionVersion: 1,
      audioMode: 'all',
      subtitleMode: 'all',
      subtitleDisposition: 'keep',
      subtitleSidecarFormats: ['srt'],
      attachmentPolicy: 'auto',
      fontAttachmentExportPolicy: 'none',
      chapterPolicy: 'keep',
      ocrMode: 'accurate',
      defaultAudioLanguage: '',
      defaultSubtitleLanguage: '',
    },
  },
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function merge(base: Record<string, unknown>, overlay: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = { ...base };
  Object.entries(overlay).forEach(([key, value]) => {
    result[key] = isRecord(result[key]) && isRecord(value) ? merge(result[key] as Record<string, unknown>, value) : value;
  });
  return result;
}

export function assetDefaultsValue(value: unknown): AssetDefaultsValue {
  return merge(defaults, isRecord(value) ? value : {}) as AssetDefaultsValue;
}

export function updateAssetDefaultsDomain(
  value: unknown,
  domain: 'video' | 'audio' | 'tracks',
  patch: Record<string, unknown>,
): AssetDefaultsValue {
  const current = assetDefaultsValue(value);
  const nextDomain = merge(current[domain] as Record<string, unknown>, patch);
  if (nextDomain.source === 'profile') {
    nextDomain.custom = {};
  } else if (nextDomain.source === 'custom') {
    delete nextDomain.profileId;
    delete nextDomain.profileKey;
  }
  return {
    ...current,
    [domain]: nextDomain,
  } as AssetDefaultsValue;
}

export function assetDefaultProfileReference(domain: AssetDefaultsDomain): number | string | undefined {
  return domain.profileId ?? domain.profileKey;
}
