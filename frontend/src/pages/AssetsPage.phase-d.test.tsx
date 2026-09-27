import { describe, expect, it } from 'vitest';
import type { Asset, AssetGroup, AssetInventory } from '../api/types';
import { filterAssetGroups } from '../utils/assetPageHelpers';
import { buildDashboardSummary } from '../utils/dashboardSummary';

function asset(id: number, lifecycle: Partial<NonNullable<Asset['lifecycle']>>): Asset {
  return {
    id,
    libraryId: 1,
    libraryName: 'Anime',
    path: `/media/library/anime/Show/E${id}.mkv`,
    relativePath: `Show/E${id}.mkv`,
    groupPath: 'Show',
    fileName: `E${id}.mkv`,
    extension: '.mkv',
    sizeBytes: 100,
    modifiedAt: '2026-09-25T00:00:00Z',
    status: 'mixed',
    missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' },
    conversion: {},
    lifecycle: {
      location: 'library', lifecycle: 'available', provenance: 'legacy_or_unknown', validation: 'unavailable',
      publication: 'unpublished', original: 'unknown', ...lifecycle,
    },
  };
}

function mixedGroup(): AssetGroup {
  const assets = [
    asset(1, { lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived' }),
    asset(2, { lifecycle: 'published', provenance: 'published_as_is', validation: 'passed_with_warnings', publication: 'published' }),
    asset(3, { lifecycle: 'queued', provenance: 'legacy_or_unknown', validation: 'pending' }),
    asset(4, { lifecycle: 'needs_review', validation: 'review_required', attention: ['needs_review'] }),
    asset(5, { lifecycle: 'failed', validation: 'failed', publication: 'failed' }),
  ];
  return {
    id: 'show', libraryId: 1, libraryName: 'Anime', path: '/media/library/anime/Show', relativePath: 'Show', status: 'mixed',
    fileCount: assets.length, sizeBytes: assets.length * 100, modifiedAt: '2026-09-25T00:00:00Z', assets,
    review: assets[3].review, pathReview: assets[3].review, metadata: assets[0].metadata, pathMetadata: assets[0].metadata,
    lifecycleSummary: {
      total: assets.length,
      lifecycle: [{ value: 'failed', count: 1 }, { value: 'needs_review', count: 1 }, { value: 'published', count: 2 }, { value: 'queued', count: 1 }],
      provenance: [{ value: 'converted', count: 1 }, { value: 'legacy_or_unknown', count: 2 }, { value: 'published_as_is', count: 1 }],
      validation: [{ value: 'failed', count: 1 }, { value: 'passed', count: 1 }, { value: 'passed_with_warnings', count: 1 }, { value: 'pending', count: 1 }, { value: 'review_required', count: 1 }],
      publication: [{ value: 'failed', count: 1 }, { value: 'published', count: 2 }, { value: 'unpublished', count: 2 }],
      attention: [{ value: 'needs_review', count: 1 }],
    },
  };
}

describe('Phase D Assets compatibility projections', () => {
  it('filters mixed Library groups by persisted lifecycle dimensions without group-state inference', () => {
    const group = mixedGroup();
    expect(filterAssetGroups([group], '', { provenance: 'converted' })[0].assets.map((item) => item.id)).toEqual([1]);
    expect(filterAssetGroups([group], '', { validation: 'failed' })[0].assets.map((item) => item.id)).toEqual([5]);
    expect(filterAssetGroups([group], '', { original: 'archived' })[0].assets.map((item) => item.id)).toEqual([1, 2, 3, 4, 5]);
    expect(filterAssetGroups([group], '', { workflow: 'queued' })[0].assets.map((item) => item.id)).toEqual([3]);
    expect(filterAssetGroups([group], '', { attention: 'needs_review' })[0].assets.map((item) => item.id)).toEqual([4]);
    expect(filterAssetGroups([group], '')[0].assets).toHaveLength(5);
    expect(group.status).toBe('mixed');
  });

  it('keeps lifecycle counts dimension-specific and does not count archive evidence as inventory', () => {
    const inventory = {
      raw: [asset(1, { location: 'raw' })],
      unprocessed: [asset(1, { location: 'raw' })],
      library: [asset(2, {}), asset(3, {})],
      converted: [asset(2, { provenance: 'converted' })],
      unverified: [asset(3, {})],
      accepted: [], missing: [],
      unprocessedGroups: [], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [],
      reports: {}, sync: { lastSyncedAt: '', totalRecords: 4, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
      counts: { raw: 1, library: 2, processing: 1, needsReview: 1, validationReview: 1, publicationBlocked: 1, converted: 1, originalsArchived: 1, missing: 0, historicalPaths: 1 },
    } as unknown as AssetInventory;
    const summary = buildDashboardSummary([], [], inventory);
    expect(summary.totalAssets).toBe(3);
    expect(summary.processingAssets).toBe(1);
    expect(summary.needsReviewAssets).toBe(1);
    expect(summary.validationReviewAssets).toBe(1);
    expect(summary.publicationBlockedAssets).toBe(1);
    expect(summary.originalsArchived).toBe(1);
  });

  it('ignores the retired Archived filter while retaining Library asset rows', () => {
    const libraryAssets = [
      asset(6, { provenance: 'converted', original: 'archived' }),
      asset(7, { provenance: 'converted', original: 'available' }),
    ];
    const group = { ...mixedGroup(), assets: libraryAssets };

    expect(filterAssetGroups([group], '', { original: 'archived' })[0].assets.map((item) => item.id)).toEqual([6, 7]);
    expect(libraryAssets.every((item) => item.lifecycle?.location === 'library')).toBe(true);
  });
});
