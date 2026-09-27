import { describe, expect, it } from 'vitest';
import type { Asset, AssetGroup, AssetInventory, AssetSourceGroup } from '../api/types';
import { patchAssetInventoryAsset } from './assetInventoryPatch';

const asset = (path: string, category: string): Asset => ({
  path,
  metadata: { categories: [category], tags: [], updatedAt: '' },
  conversion: { keepAudioStreams: [], keepSubtitleStreams: [], keepVideoStreams: [] },
  libraryId: 1,
  libraryName: 'Test',
  relativePath: path,
  groupPath: '/media/raw',
  fileName: path.split('/').at(-1) ?? path,
  extension: '.mkv',
  sizeBytes: 1,
  modifiedAt: '',
  status: 'unprocessed',
  missing: false,
  review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
});

const inventory = (target: Asset, unrelated: Asset): AssetInventory => {
  const targetGroup: AssetGroup = { id: 'group', libraryId: 1, libraryName: 'Test', path: '/media/raw', relativePath: '', status: 'unprocessed', fileCount: 1, sizeBytes: 1, modifiedAt: '', assets: [target], review: target.review, pathReview: target.review, metadata: target.metadata, pathMetadata: target.metadata };
  const sourceGroup: AssetSourceGroup = { id: 1, name: 'Source', relativePath: '', sourcePath: '/media/raw', fileCount: 1, assetCount: 1, titleCount: 1, pathCount: 1, sizeBytes: 1, totalSizeBytes: 1, logicalGroups: [{ id: 'logical', name: 'Logical', path: '/media/raw', relativePath: '', fileCount: 1, assetCount: 1, pathCount: 1, sizeBytes: 1, totalSizeBytes: 1, assetPaths: [{ id: 'path', name: 'Path', path: '/media/raw', relativePath: '', displayPath: '', isLogicalGroupRoot: true, fileCount: 1, assetCount: 1, sizeBytes: 1, totalSizeBytes: 1, assets: [target] }] }] };
  return {
    sourceGroups: [sourceGroup],
    unprocessed: [target, unrelated],
    library: [], converted: [], unverified: [], accepted: [], missing: [],
    unprocessedGroups: [targetGroup], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [],
    reports: {} as AssetInventory['reports'],
    sync: {} as AssetInventory['sync'],
  };
};

describe('patchAssetInventoryAsset', () => {
  it('patches flat, grouped, and nested source-group representations', () => {
    const target = asset('/media/raw/movie.mkv', 'old');
    const unrelated = asset('/media/raw/other.mkv', 'other');
    const nextMetadata = { ...target.metadata, categories: ['new'] };
    const nextConversion = { ...target.conversion, keepAudioStreams: [2] };
    const current = inventory(target, unrelated);
    const result = patchAssetInventoryAsset(current, '/media/raw/movie.mkv/', { metadata: nextMetadata, conversion: nextConversion });

    expect(result).not.toBe(current);
    expect(result.unprocessed[0].metadata).toBe(nextMetadata);
    expect(result.unprocessedGroups[0].assets[0].conversion).toBe(nextConversion);
    expect(result.sourceGroups[0].logicalGroups[0].assetPaths[0].assets[0].metadata).toBe(nextMetadata);
    expect(result.unprocessed[1]).toBe(unrelated);
    expect(result.library).toBe(current.library);
    expect(patchAssetInventoryAsset(current, '/media/raw/missing.mkv', { metadata: nextMetadata, conversion: nextConversion })).toBe(current);
  });
});
