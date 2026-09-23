import type { Asset, AssetGroup, AssetInventory, AssetMetadataState, AssetSourceGroup, AssetConversionOverrideState } from '../api/types';

export type AssetInventoryPatch = {
  metadata: AssetMetadataState;
  conversion: AssetConversionOverrideState;
};

export function patchAssetInventoryAsset(
  inventory: AssetInventory,
  path: string,
  patch: AssetInventoryPatch,
): AssetInventory {
  const normalizedPath = normalizePath(path);
  let changed = false;

  const patchFlat = (assets: Asset[]) => {
    const result = patchAssets(assets, normalizedPath, patch);
    changed ||= result.changed;
    return result.assets;
  };
  const patchGroups = (groups: AssetGroup[]) => {
    const result = groups.map((group) => {
      const assets = patchFlat(group.assets);
      return assets === group.assets ? group : { ...group, assets };
    });
    return result;
  };

  const flatKeys = ['unprocessed', 'library', 'converted', 'unverified', 'accepted', 'archive', 'missing'] as const;
  const next = { ...inventory };
  for (const key of flatKeys) {
    const assets = patchFlat(inventory[key]);
    if (assets !== inventory[key]) next[key] = assets;
  }
  const groupKeys = ['unprocessedGroups', 'libraryGroups', 'convertedGroups', 'unverifiedGroups', 'acceptedGroups', 'archiveGroups'] as const;
  for (const key of groupKeys) {
    const groups = patchGroups(inventory[key]);
    if (groups.some((group, index) => group !== inventory[key][index])) next[key] = groups;
  }

  const sourceGroups = inventory.sourceGroups.map((sourceGroup) => patchSourceGroup(sourceGroup, normalizedPath, patch, () => { changed = true; }));
  if (sourceGroups.some((sourceGroup, index) => sourceGroup !== inventory.sourceGroups[index])) next.sourceGroups = sourceGroups;

  return changed ? next : inventory;
}

function patchSourceGroup(
  sourceGroup: AssetSourceGroup,
  path: string,
  patch: AssetInventoryPatch,
  markChanged: () => void,
): AssetSourceGroup {
  let changed = false;
  const logicalGroups = sourceGroup.logicalGroups.map((logicalGroup) => {
    const assetPaths = logicalGroup.assetPaths.map((assetPath) => {
      const result = patchAssets(assetPath.assets, path, patch);
      if (!result.changed) return assetPath;
      changed = true;
      return { ...assetPath, assets: result.assets };
    });
    if (assetPaths.every((assetPath, index) => assetPath === logicalGroup.assetPaths[index])) return logicalGroup;
    return { ...logicalGroup, assetPaths };
  });
  if (!changed) return sourceGroup;
  markChanged();
  return { ...sourceGroup, logicalGroups };
}

function patchAssets(
  assets: Asset[],
  path: string,
  patch: AssetInventoryPatch,
): { assets: Asset[]; changed: boolean } {
  let changed = false;
  const next = assets.map((asset) => {
    if (normalizePath(asset.path) !== path) return asset;
    changed = true;
    return { ...asset, metadata: patch.metadata, conversion: patch.conversion };
  });
  return { assets: changed ? next : assets, changed };
}

function normalizePath(value: string): string {
  return value.replace(/\\/g, '/').replace(/\/+$/, '');
}
