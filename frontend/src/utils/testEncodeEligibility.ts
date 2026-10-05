import type { Asset, EffectiveAssetConfigurationValue } from '../api/types';

export function hidePreConversionActionsForAsset(asset: Pick<Asset, 'lifecycle'>) {
  const lifecycle = asset.lifecycle;
  return lifecycle?.location === 'library' && (
    lifecycle.provenance === 'converted' ||
    lifecycle.provenance === 'published_as_is' ||
    lifecycle.currentPublication?.state === 'published'
  );
}

export function testEncodeEligibleAsset(
  asset: Pick<Asset, 'status' | 'publicationMode'>,
  mode: 'unprocessed' | 'library' | 'converted',
) {
  if (mode === 'converted' || asset.status === 'converted') {
    return false;
  }
  return asset.status !== 'published_as_is' && asset.publicationMode !== 'as_is';
}

// Mirrors the supported effective-asset inputs of Test Encode V1. Lifecycle
// capabilities remain separate and backend resolution is authoritative.
export function testEncodeConfigurationReason(video?: EffectiveAssetConfigurationValue): string {
  if (!video) return 'Effective video configuration is loading or unavailable.';
  if (video.selection === 'profile' && (video.videoProfileId ?? 0) > 0) return '';
  if (video.source === 'global' && video.selection === 'value' && Object.keys(video.profileData ?? {}).length > 0) return '';
  return `Test Encode V1 requires an effective video profile or supported global Custom configuration. Current selection: ${video.selection}.`;
}

export function effectiveVideoProfileLabel(video: EffectiveAssetConfigurationValue | undefined, profiles: { id: number; name: string }[]): string {
  if (!video) return 'Unavailable';
  if (video.selection === 'profile' && (video.videoProfileId ?? 0) > 0) return profiles.find((profile) => profile.id === video.videoProfileId)?.name ?? 'Unavailable profile';
  switch (video.selection) {
    case 'value': return video.source === 'global' && Object.keys(video.profileData ?? {}).length > 0 ? 'Custom' : 'Unavailable';
    case 'disabled': return 'Disabled';
    case 'override_only': return 'Overrides only';
    case 'audio_only': return 'Audio only';
    case 'inherit': return 'Inherit · no effective profile';
    default: return 'Unavailable';
  }
}
