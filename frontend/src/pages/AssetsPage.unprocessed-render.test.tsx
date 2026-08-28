// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, AssetPath } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
    queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
    profileAssignments: vi.fn(), updateProfileAssignment: vi.fn(), assetScopeConfigurations: vi.fn(), updateAssetScopeConfiguration: vi.fn(), effectiveAssetConfiguration: vi.fn(), effectiveAssetConfigurations: vi.fn(),
    configureLogicalGroupsBatch: vi.fn(), queueSelectedAssets: vi.fn(), createQueueBatch: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

afterEach(cleanup);

function testAsset(id: number, path: string, missing = false): Asset {
  return { id, libraryId: 0, libraryName: 'Originals', path, relativePath: path.replace('/media/raw/', ''), groupPath: 'movies/Akira', fileName: path.split('/').pop() ?? '', extension: '.mkv', sizeBytes: 1024, modifiedAt: '2026-08-28T00:00:00Z', status: 'unprocessed', missing, review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' }, metadata: { categories: [], tags: [], updatedAt: '' }, conversion: {} };
}

function testPath(id: string, path: string, displayPath: string, root: boolean, assets: Asset[]): AssetPath {
  return { id, name: displayPath, path, relativePath: displayPath, displayPath, isLogicalGroupRoot: root, fileCount: assets.length, assetCount: assets.length, sizeBytes: assets.length * 1024, totalSizeBytes: assets.length * 1024, assets };
}

describe('Unprocessed Assets hierarchy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const rootAsset = testAsset(1, '/media/raw/movies/Akira/Akira.mkv');
    const childAsset = testAsset(2, '/media/raw/movies/Akira/extras/Trailer.mkv');
    const missingAsset = testAsset(3, '/media/raw/movies/Akira/extras/Missing.mkv', true);
    const rootPath = testPath('root', '/media/raw/movies/Akira', 'Root', true, [rootAsset]);
    const childPath = testPath('extras', '/media/raw/movies/Akira/extras', 'extras', false, [childAsset, missingAsset]);
    const inventory = {
      sourceGroups: [{ id: 1, name: 'Movies', relativePath: 'movies', sourcePath: '/media/raw/movies', fileCount: 3, assetCount: 3, titleCount: 1, pathCount: 2, sizeBytes: 3072, totalSizeBytes: 3072, logicalGroups: [{ id: '/media/raw/movies/Akira', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'Akira', fileCount: 3, assetCount: 3, pathCount: 2, sizeBytes: 3072, totalSizeBytes: 3072, assetPaths: [rootPath, childPath] }] }],
      unprocessed: [rootAsset, childAsset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], archive: [], missing: [missingAsset], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], archiveGroups: [], reports: {}, sync: { lastSyncedAt: '2026-08-28T00:00:00Z', totalRecords: 3, missingFiles: 1, missingActionable: 1, missingHistorical: 0 },
    } as unknown as AssetInventory;
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([]);
    vi.mocked(api.updateProfileAssignment).mockResolvedValue({ status: 'inherited' });
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.updateAssetScopeConfiguration).mockResolvedValue({} as never);
    vi.mocked(api.configureLogicalGroupsBatch).mockResolvedValue({ logicalGroupPaths: ['/media/raw/movies/Akira'], changedDimensions: ['video'] });
    vi.mocked(api.effectiveAssetConfiguration).mockImplementation(async (path) => ({ assetPath: path, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } }));
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((assetId) => [String(assetId), { assetPath: `/asset/${assetId}`, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } }])),
      missingAssetIds: [],
    }));
    vi.mocked(api.queueSelectedAssets).mockImplementation(async ({ assetIds, commit }) => ({
      summary: { selected: assetIds.length, eligible: assetIds.length, queued: commit ? assetIds.length : 0, skipped: 0, failed: 0, titleCount: 1, sizeBytes: assetIds.length * 1024 },
      results: assetIds.map((assetId) => ({ assetId, outcome: commit ? 'queued' as const : 'eligible' as const, batchId: 'selected-test-001', batchName: 'Akira', ...(commit ? { jobId: assetId + 100 } : {}) })),
      batches: commit ? [{ batchId: 'selected-test-001', batchName: 'Akira', jobCount: assetIds.length }] : [],
    }));
  });

  it('renders root assets directly and only child paths as nested accordions', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(await screen.findByRole('heading', { name: 'Akira' })).toBeTruthy();
    expect(screen.getByText('3 assets · 1 title · 2 paths · 3.00 KB')).toBeTruthy();
    expect(screen.queryByLabelText('Media area')).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Path' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Library' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Confidence' })).toBeNull();
    expect(api.effectiveAssetConfigurations).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Expand Akira' }));
    await waitFor(() => expect(api.effectiveAssetConfigurations).toHaveBeenCalledTimes(1));
    expect(api.effectiveAssetConfigurations).toHaveBeenCalledWith([1, 2]);
    expect(api.effectiveAssetConfiguration).not.toHaveBeenCalled();
    expect(await screen.findByText('Akira.mkv')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Queue Akira.mkv' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Toggle review for Akira.mkv' })).toBeTruthy();
    expect(screen.getByText('extras')).toBeTruthy();
    expect(screen.queryByRole('button', { name: /expand root/i })).toBeNull();
    expect(screen.getByRole('button', { name: /path actions extras/i })).toBeTruthy();
  });

  it('selects a collapsed title using only selectable descendants and keeps missing assets visible', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    const titleCheckbox = await screen.findByRole('checkbox', { name: 'Select title Akira' });
    await user.click(titleCheckbox);
    expect((titleCheckbox as HTMLInputElement).checked).toBe(true);
    expect(screen.getByText('1 title selected · 2 assets · 2.00 KB')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Expand Akira' }));
    await user.click(screen.getByRole('button', { name: 'Expand path extras' }));
    expect(await screen.findByText('Missing.mkv')).toBeTruthy();
    expect((screen.getByRole('checkbox', { name: 'Select Missing.mkv' }) as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByRole('checkbox', { name: 'Select path extras' }) as HTMLInputElement).checked).toBe(true);
  });

  it('loads persisted title values and saves only the explicitly changed dimension', async () => {
    vi.mocked(api.profileAssignments).mockResolvedValue([
      { id: 1, targetType: 'logical_group', targetPath: '/media/raw/movies/Akira', mediaType: 'video', selection: 'disabled', createdAt: '', updatedAt: '' },
      { id: 2, targetType: 'logical_group', targetPath: '/media/raw/movies/Akira', mediaType: 'audio', selection: 'disabled', createdAt: '', updatedAt: '' },
      { id: 3, targetType: 'logical_group', targetPath: '/media/raw/movies/Akira', mediaType: 'tracks', selection: 'disabled', createdAt: '', updatedAt: '' },
    ]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([{
      id: 1,
      scopeType: 'logical_group',
      scopeKey: '/media/raw/movies/Akira',
      categorySelection: 'disabled',
      category: '',
      destinationSelection: 'inherit',
      createdAt: '',
      updatedAt: '',
    }]);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Configure title' }));
    const videoInput = await screen.findByLabelText('Video profile');
    expect((videoInput as HTMLInputElement).value).toBe('Disabled');
    expect((screen.getByLabelText('Audio profile') as HTMLInputElement).value).toBe('Disabled');
    expect((screen.getByLabelText('Tracks profile') as HTMLInputElement).value).toBe('Disabled');
    expect(screen.getByLabelText('Category mode').textContent).toContain('Disabled');
    expect(screen.getByLabelText('Destination mode').textContent).toContain('Inherit');

    await user.click(screen.getByRole('checkbox', { name: 'Change video' }));
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await waitFor(() => expect(api.updateProfileAssignment).toHaveBeenCalledTimes(1));
    expect(api.updateProfileAssignment).toHaveBeenCalledWith(expect.objectContaining({
      targetType: 'logical_group',
      targetPath: '/media/raw/movies/Akira',
      mediaType: 'video',
      selection: 'disabled',
    }));
    expect(api.updateAssetScopeConfiguration).not.toHaveBeenCalled();
  });

  it('plans and commits Queue selected using only Asset IDs and exposes partial results', async () => {
    vi.mocked(api.queueSelectedAssets).mockImplementation(async ({ assetIds, commit }) => commit ? {
      summary: { selected: 2, eligible: 1, queued: 1, skipped: 1, failed: 0, titleCount: 1, sizeBytes: 2048 },
      results: [
        { assetId: 1, outcome: 'queued', batchId: 'selected-test-001', batchName: 'Akira', jobId: 101 },
        { assetId: 2, outcome: 'skipped', reason: 'already_queued', message: 'Asset already has an open Queue job' },
      ],
      batches: [{ batchId: 'selected-test-001', batchName: 'Akira', jobCount: 1 }],
    } : {
      summary: { selected: 2, eligible: 2, queued: 0, skipped: 0, failed: 0, titleCount: 1, sizeBytes: 2048 },
      results: assetIds.map((assetId) => ({ assetId, outcome: 'eligible', batchId: 'selected-test-001', batchName: 'Akira' })),
      batches: [],
    });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await waitFor(() => expect(api.queueSelectedAssets).toHaveBeenCalledWith({ assetIds: [1, 2], commit: false }));
    expect(api.effectiveAssetConfigurations).not.toHaveBeenCalled();
    expect(api.createQueueBatch).not.toHaveBeenCalled();
    expect(await screen.findByText('Will be queued: 2')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Queue 2 assets' }));
    await waitFor(() => expect(api.queueSelectedAssets).toHaveBeenCalledWith({ assetIds: [1, 2], commit: true }));
    expect(await screen.findByText('1 queued · 1 skipped · 0 failed')).toBeTruthy();
    expect(screen.getByText('Asset 2: Asset already has an open Queue job')).toBeTruthy();
    expect(api.createQueueBatch).not.toHaveBeenCalled();
  });

  it('configures only fully selected LogicalGroups in one transactional request', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Configure selected' }));
    await user.click(await screen.findByRole('checkbox', { name: 'Change Video' }));
    await user.click(screen.getByRole('button', { name: 'Apply to selected titles' }));

    await waitFor(() => expect(api.configureLogicalGroupsBatch).toHaveBeenCalledTimes(1));
    expect(api.configureLogicalGroupsBatch).toHaveBeenCalledWith({
      logicalGroupPaths: ['/media/raw/movies/Akira'],
      video: { mode: 'inherit' },
      audio: { mode: 'no_change' },
      tracks: { mode: 'no_change' },
      category: { mode: 'no_change' },
      destination: { mode: 'no_change' },
    });
    expect(api.updateProfileAssignment).not.toHaveBeenCalled();
    expect(api.updateAssetScopeConfiguration).not.toHaveBeenCalled();
  });
});
