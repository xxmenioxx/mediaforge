import type { Asset } from '../api/types';

export function testEncodeEligibleAsset(
  asset: Pick<Asset, 'status' | 'publicationMode'>,
  mode: 'unprocessed' | 'library' | 'converted' | 'archive',
) {
  if (mode === 'converted' || mode === 'archive' || asset.status === 'converted' || asset.status === 'archive') {
    return false;
  }
  return asset.status !== 'published_as_is' && asset.publicationMode !== 'as_is';
}
