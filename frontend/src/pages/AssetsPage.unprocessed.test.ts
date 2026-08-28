import { describe, expect, it } from 'vitest';
import { hierarchicalSelectionState, selectableAssetsForPaths, selectedSelectableAssetsForLogicalGroups } from '../utils/unprocessedAssetSelection';
import type { Asset, AssetPath } from '../api/types';

function asset(id: number, missing = false): Asset {
  return {
    id,
    libraryId: 0,
    libraryName: 'Originals',
    path: `/raw/movies/Akira/${id}.mkv`,
    relativePath: `movies/Akira/${id}.mkv`,
    groupPath: 'movies/Akira',
    fileName: `${id}.mkv`,
    extension: '.mkv',
    sizeBytes: 100,
    modifiedAt: '',
    status: 'unprocessed',
    missing,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' },
    conversion: {},
  };
}

function path(id: string, root: boolean, assets: Asset[]): AssetPath {
  return { id, name: id, path: `/raw/movies/Akira/${id}`, relativePath: id, displayPath: id, isLogicalGroupRoot: root, fileCount: assets.length, assetCount: assets.length, sizeBytes: assets.length * 100, totalSizeBytes: assets.length * 100, assets };
}

describe('Unprocessed hierarchical selection', () => {
	it('expands a closed title from DTO asset IDs while excluding missing records', () => {
		expect(selectableAssetsForPaths([path('root', true, [asset(1), asset(2, true)]), path('extras', false, [asset(3)])]).map((item) => item.id)).toEqual([1, 3]);
	});

	it('treats a parent as checked when every selectable descendant is selected and another descendant is missing', () => {
		const paths = [path('root', true, [asset(1), asset(2), asset(3, true)])];
		const selectableIds = selectableAssetsForPaths(paths).map((item) => item.id as number);
		expect(hierarchicalSelectionState(selectableIds, new Set([1, 2]))).toEqual({ checked: true, indeterminate: false, selectedCount: 2 });
	});

	it('derives checked and indeterminate state from one Asset ID source of truth', () => {
		expect(hierarchicalSelectionState([1, 2, 3], new Set([1, 3]))).toEqual({ checked: false, indeterminate: true, selectedCount: 2 });
		expect(hierarchicalSelectionState([1, 2, 3], new Set([1, 2, 3]))).toEqual({ checked: true, indeterminate: false, selectedCount: 3 });
		expect(hierarchicalSelectionState([1, 2, 3], new Set())).toEqual({ checked: false, indeterminate: false, selectedCount: 0 });
	});

	it('excludes missing records from selected count and selected size', () => {
		const paths = [path('root', true, [asset(1), asset(2, true), asset(3)])];
		const selected = selectedSelectableAssetsForLogicalGroups([{
			id: 'akira', name: 'Akira', path: '/raw/movies/Akira', relativePath: 'Akira',
			fileCount: 3, assetCount: 3, pathCount: 1, sizeBytes: 300, totalSizeBytes: 300, assetPaths: paths,
		}], new Set([1, 2, 3]));
		expect(selected.map((item) => item.id)).toEqual([1, 3]);
		expect(selected.reduce((total, item) => total + item.sizeBytes, 0)).toBe(200);
	});
});
