import type { AssetLogicalGroup, AssetPath } from '../api/types';

export function selectableAssetsForPaths(paths: AssetPath[]) {
  return paths.flatMap((path) => path.assets ?? []).filter((asset) => Boolean(asset.id) && !asset.missing);
}

export function selectedSelectableAssetsForLogicalGroups(
  logicalGroups: AssetLogicalGroup[],
  selectedAssetIds: ReadonlySet<number>,
) {
  return logicalGroups
    .flatMap((group) => selectableAssetsForPaths(group.assetPaths))
    .filter((asset) => selectedAssetIds.has(asset.id as number));
}

export function hierarchicalSelectionState(assetIds: number[], selectedAssetIds: Set<number>) {
  const checked = assetIds.length > 0 && assetIds.every((id) => selectedAssetIds.has(id));
  const selectedCount = assetIds.filter((id) => selectedAssetIds.has(id)).length;
  return { checked, indeterminate: selectedCount > 0 && !checked, selectedCount };
}
