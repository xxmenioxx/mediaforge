// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, AssetPath } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
    queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
    profileAssignments: vi.fn(), assetScopeConfigurations: vi.fn(), effectiveAssetConfiguration: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

function testAsset(id: number, path: string): Asset {
  return { id, libraryId: 0, libraryName: 'Originals', path, relativePath: path.replace('/media/raw/', ''), groupPath: 'movies/Akira', fileName: path.split('/').pop() ?? '', extension: '.mkv', sizeBytes: 1024, modifiedAt: '2026-08-28T00:00:00Z', status: 'unprocessed', missing: false, review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' }, metadata: { categories: [], tags: [], updatedAt: '' }, conversion: {} };
}

function testPath(id: string, path: string, displayPath: string, root: boolean, assets: Asset[]): AssetPath {
  return { id, name: displayPath, path, relativePath: displayPath, displayPath, isLogicalGroupRoot: root, fileCount: assets.length, assetCount: assets.length, sizeBytes: assets.length * 1024, totalSizeBytes: assets.length * 1024, assets };
}

describe('Unprocessed Assets hierarchy', () => {
  beforeEach(() => {
    const rootAsset = testAsset(1, '/media/raw/movies/Akira/Akira.mkv');
    const childAsset = testAsset(2, '/media/raw/movies/Akira/extras/Trailer.mkv');
    const rootPath = testPath('root', '/media/raw/movies/Akira', 'Root', true, [rootAsset]);
    const childPath = testPath('extras', '/media/raw/movies/Akira/extras', 'extras', false, [childAsset]);
    const inventory = {
      sourceGroups: [{ id: 1, name: 'Movies', relativePath: 'movies', sourcePath: '/media/raw/movies', fileCount: 2, assetCount: 2, titleCount: 1, pathCount: 2, sizeBytes: 2048, totalSizeBytes: 2048, logicalGroups: [{ id: '/media/raw/movies/Akira', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'Akira', fileCount: 2, assetCount: 2, pathCount: 2, sizeBytes: 2048, totalSizeBytes: 2048, assetPaths: [rootPath, childPath] }] }],
      unprocessed: [rootAsset, childAsset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], archive: [], missing: [], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], archiveGroups: [], reports: {}, sync: { lastSyncedAt: '2026-08-28T00:00:00Z', totalRecords: 2, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
    } as unknown as AssetInventory;
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.effectiveAssetConfiguration).mockImplementation(async (path) => ({ assetPath: path, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } }));
  });

  it('renders root assets directly and only child paths as nested accordions', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(await screen.findByRole('heading', { name: 'Akira' })).toBeTruthy();
    expect(screen.getByText('2 assets · 1 title · 2 paths · 2.00 KB')).toBeTruthy();
    expect(screen.queryByLabelText('Media area')).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Path' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Library' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Confidence' })).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Expand Akira' }));
    expect(await screen.findByText('Akira.mkv')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Queue Akira.mkv' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Toggle review for Akira.mkv' })).toBeTruthy();
    expect(screen.getByText('extras')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /expand root/i })).toBeNull();
    expect(screen.getByRole('button', { name: /path actions extras/i })).toBeTruthy();
  });
});
