import type { Asset } from '../api/types';

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
