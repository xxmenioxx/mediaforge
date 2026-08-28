import type { AssetPath } from '../api/types';

export function selectableAssetsForPaths(paths: AssetPath[]) {
  return paths.flatMap((path) => path.assets ?? []).filter((asset) => Boolean(asset.id));
}

export function hierarchicalSelectionState(assetIds: number[], selectedAssetIds: Set<number>) {
  const checked = assetIds.length > 0 && assetIds.every((id) => selectedAssetIds.has(id));
  const selectedCount = assetIds.filter((id) => selectedAssetIds.has(id)).length;
  return { checked, indeterminate: selectedCount > 0 && !checked, selectedCount };
}
