// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, AssetPath, QueueJob, ScanResult, SnapshotOperation } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
    queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
    latestSnapshot: vi.fn(), startSnapshotOperation: vi.fn(), snapshotOperation: vi.fn(), externalAssetSubtitles: vi.fn(), subtitleExtractionOperations: vi.fn(),
    profileAssignments: vi.fn(), updateProfileAssignment: vi.fn(), assetScopeConfigurations: vi.fn(), updateAssetScopeConfiguration: vi.fn(), effectiveAssetConfiguration: vi.fn(), effectiveAssetConfigurations: vi.fn(),
    configureLogicalGroupsBatch: vi.fn(), queueSelectedAssets: vi.fn(), createQueueBatch: vi.fn(), renameAsset: vi.fn(), evaluateAdvisor: vi.fn(), publishAssetsAsIs: vi.fn(),
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

function testSnapshot(path = '/media/raw/movies/Akira/Akira.mkv', videoCodec = 'h264'): ScanResult {
  return {
    id: 1, path, fileName: 'Akira.mkv', container: 'matroska', sizeBytes: 1024, duration: 60, bitrate: 1000,
    videoCodec, width: 1920, height: 1080, hdr: false, audioTracks: 1, subtitleTracks: 0, chapters: 0,
    videoStreams: [], audioStreams: [], subtitleStreams: [], compatibilityAnalysis: {}, interlaceAnalysis: {}, rawProbe: {},
  } as unknown as ScanResult;
}

function completedSnapshotOperation(path = '/media/raw/movies/Akira/Akira.mkv', forceResult = testSnapshot(path)): SnapshotOperation {
  return {
    id: 'snapshot-operation-1', assetPath: path, status: 'completed', phase: 'complete', progress: 100,
    message: 'Complete', durationMs: 10, cacheHit: false, incrementalRefresh: false, result: forceResult,
    createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
  };
}

function activeQueueJob(id: number, mediaPath: string): QueueJob {
  return {
    id, batchId: 'active-batch', batchName: 'Active batch', batchPosition: id, mediaPath,
    publishMode: 'standard', libraryId: 1, profileId: 1, profileVersion: 1, profileSnapshot: {},
    audioProfileKey: '', trackProfileKey: '', processingMode: 'full_encode', priority: 5,
    queuePosition: id, status: 'queued', stage: 'queued', stageHistory: [], progress: 0,
    workerName: '', outputPath: '', plannedPublishedPath: '', errorMessage: '', notes: '',
    validationStatus: 'pending', validationScore: 0, validationReport: {}, publishedPath: '',
    replacementTargetPath: '', originalArchivedPath: '', createdAt: '2026-08-28T00:00:00Z',
    updatedAt: '2026-08-28T00:00:00Z',
  } as QueueJob;
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
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation());
    vi.mocked(api.snapshotOperation).mockResolvedValue(completedSnapshotOperation());
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([]);
    vi.mocked(api.subtitleExtractionOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.renameAsset).mockResolvedValue({ oldPath: '/media/raw/movies/Akira/Akira.mkv', path: '/media/raw/movies/Akira/Akira Renamed.mkv', fileName: 'Akira Renamed.mkv' });
    vi.mocked(api.evaluateAdvisor).mockImplementation(async ({ mediaPath }) => ({ recommendation: 'worth_it', score: 90, summary: `Ready: ${mediaPath}`, reasons: [], warnings: [] } as never));
    vi.mocked(api.publishAssetsAsIs).mockResolvedValue({ message: 'Published as-is', published: 2 } as never);
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
    expect(screen.getByRole('button', { name: 'Configure title' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Snapshots' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Run Advisor' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Publish as-is' })).toBeTruthy();
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
    expect(screen.queryByRole('button', { name: 'Root path settings' })).toBeNull();
    expect(screen.queryByRole('button', { name: /path actions/i })).toBeNull();
    // One SourceGroup Configure plus one nested path Configure; Root contributes none.
    expect(screen.getAllByRole('button', { name: 'Configure' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Snapshots' })).toHaveLength(1);
  });

  it('shows one title-scoped Snapshots surface with root and nested assets', async () => {
    vi.mocked(api.latestSnapshot).mockImplementation(async (path) => path.endsWith('/Akira.mkv')
      ? { found: true, snapshot: testSnapshot(path), status: 'current', requiresAnalysis: false, staleComponents: [] }
      : { found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    const snapshots = await screen.findAllByRole('button', { name: 'Snapshots' });
    expect(snapshots).toHaveLength(1);
    await user.click(snapshots[0]);
    expect(await screen.findByRole('heading', { name: 'Snapshots · Akira' })).toBeTruthy();
    expect(screen.getByText('Akira.mkv')).toBeTruthy();
    expect(screen.getByText('Trailer.mkv')).toBeTruthy();
    expect(screen.getByRole('cell', { name: 'Root' })).toBeTruthy();
    expect(screen.getByRole('cell', { name: 'extras' })).toBeTruthy();
    expect(await screen.findByRole('button', { name: 'Regenerate' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Generate' })).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Regenerate' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledWith({ path: '/media/raw/movies/Akira/Akira.mkv', force: true, analysisSeconds: 20 }));
  });

  it('runs Advisor for every current asset in the title using effective profiles', async () => {
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((assetId) => [String(assetId), { assetPath: `/asset/${assetId}`, video: { selection: 'profile', videoProfileId: assetId + 10 }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } }])),
      missingAssetIds: [],
    }));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Run Advisor' }));
    await waitFor(() => expect(api.evaluateAdvisor).toHaveBeenCalledTimes(2));
    expect(api.evaluateAdvisor).toHaveBeenCalledWith({ mediaPath: '/media/raw/movies/Akira/Akira.mkv', profileId: 11 });
    expect(api.evaluateAdvisor).toHaveBeenCalledWith({ mediaPath: '/media/raw/movies/Akira/extras/Trailer.mkv', profileId: 12 });
    expect(await screen.findByText('2 assets evaluated · 2 comply · 0 review · 0 failed')).toBeTruthy();
  });

  it('publishes a title as-is only when every asset resolves the same Destination', async () => {
    vi.mocked(api.libraries).mockResolvedValue([{ id: 5, name: 'Movies', sourcePath: '/media/raw', destinationPath: '/media/library/movies', type: 'movies', validationRules: {}, createdAt: '', updatedAt: '' }]);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((assetId) => [String(assetId), { assetPath: `/asset/${assetId}`, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'value', destinationLibraryId: 5 } }])),
      missingAssetIds: [],
    }));
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Publish as-is' }));
    await waitFor(() => expect(api.publishAssetsAsIs).toHaveBeenCalledWith({ sourcePath: '/media/raw/movies/Akira', destinationLibraryId: 5 }));
  });

  it.each([
    ['a missing Destination', [5, 0]],
    ['different Destinations', [5, 7]],
    ['no Destinations', [0, 0]],
  ])('blocks title Publish as-is for %s', async (_label, destinations) => {
    vi.mocked(api.libraries).mockResolvedValue([{ id: 5, name: 'Movies', sourcePath: '/media/raw', destinationPath: '/media/library/movies', type: 'movies', validationRules: {}, createdAt: '', updatedAt: '' }]);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((assetId, index) => [String(assetId), { assetPath: `/asset/${assetId}`, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: destinations[index] ? 'value' : 'inherit', destinationLibraryId: destinations[index] } }])),
      missingAssetIds: [],
    }));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Publish as-is' }));
    expect(await screen.findByText('Configure the same effective Destination for every asset in this title before publishing as-is.')).toBeTruthy();
    expect(api.publishAssetsAsIs).not.toHaveBeenCalled();
  });

  it('blocks title Publish as-is when an asset requires review', async () => {
    const inventory = await api.assets();
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[1].assets[0].review!.requiresReview = true;
    vi.mocked(api.assets).mockResolvedValue(inventory);
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect((await screen.findByRole('button', { name: 'Publish as-is' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('keeps nested path configuration as an explicit inheritance-aware override', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(screen.getAllByRole('button', { name: 'Configure' })[1]);
    expect(await screen.findByRole('heading', { name: 'Configure' })).toBeTruthy();
    expect((screen.getByLabelText('Video profile') as HTMLInputElement).value).toContain('Inherit');
    expect((screen.getByLabelText('Audio profile') as HTMLInputElement).value).toContain('Inherit');
    expect((screen.getByLabelText('Tracks profile') as HTMLInputElement).value).toContain('Inherit');
    expect(screen.getByLabelText('Destination mode').textContent).toContain('Inherit');
    expect(screen.queryByRole('button', { name: /Snapshots for/ })).toBeNull();
  });

  it.each([
    ['current stored snapshot', { found: true, snapshot: testSnapshot(), status: 'current' as const, requiresAnalysis: false, staleComponents: [] }],
    ['stale stored snapshot', { found: true, snapshot: testSnapshot(), status: 'stale' as const, requiresAnalysis: true, staleComponents: ['interlace'] }],
    ['no stored snapshot', { found: false, snapshot: null, status: 'missing' as const, requiresAnalysis: true, staleComponents: [] }],
  ])('keeps Asset Info read-only for %s', async (_label, snapshotState) => {
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByRole('heading', { name: 'Asset Snapshot' })).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledWith('/media/raw/movies/Akira/Akira.mkv'));
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
    if (_label === 'no stored snapshot') {
      expect(screen.getByText('No snapshot available')).toBeTruthy();
      expect(screen.getByRole('button', { name: 'Analyze asset' })).toBeTruthy();
      expect(screen.queryByRole('tab', { name: 'Asset Information' })).toBeNull();
      expect(screen.queryByRole('button', { name: 'Rescan' })).toBeNull();
    } else {
      expect(screen.getByRole('tab', { name: 'Asset Information' })).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Analyze asset' })).toBeNull();
      if (_label === 'stale stored snapshot') {
        expect(screen.getByText('Snapshot needs refresh')).toBeTruthy();
      } else {
        expect(screen.queryByText('Snapshot needs refresh')).toBeNull();
      }
      expect(screen.getByRole('button', { name: 'Rescan' })).toBeTruthy();
    }
  });

  it('starts one snapshot operation only after Analyze asset is clicked', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(api.startSnapshotOperation).toHaveBeenCalledWith({ path: '/media/raw/movies/Akira/Akira.mkv' });
  });

  it('starts one forced snapshot operation only after Rescan is clicked', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByRole('heading', { name: 'Asset Snapshot' })).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Rescan' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(api.startSnapshotOperation).toHaveBeenCalledWith({ path: '/media/raw/movies/Akira/Akira.mkv', force: true });
  });

  it('does not re-arm Analyze after success, snapshot refetch, or rerenders', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    const view = render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));

    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    await queryClient.invalidateQueries({ queryKey: ['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'] });
    await waitFor(() => expect(screen.getByText('Snapshot needs refresh')).toBeTruthy());
    view.rerender(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    view.rerender(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 10000);

  it('does not re-arm Rescan after success and snapshot refetch', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Rescan' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    await queryClient.invalidateQueries({ queryKey: ['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'] });

    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('keeps close and reopen read-only', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const user = userEvent.setup();
    const view = render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('Snapshot needs refresh')).toBeTruthy();
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('Snapshot needs refresh')).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();

    view.unmount();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('Snapshot needs refresh')).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
  });

  it('uses the latest stored snapshot after Analyze succeeds, closes, and reopens', async () => {
    const analyzedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'analyzed-codec');
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(analyzedSnapshot.path, analyzedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('analyzed-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Rescan' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'newer-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('newer-stored-codec')).toBeTruthy();
    expect(screen.queryByText('analyzed-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('does not let a completed Rescan result override a newer stored snapshot after reopen', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'initial-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });
    const rescannedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rescanned-codec');
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(rescannedSnapshot.path, rescannedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Rescan' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('rescanned-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Rescan' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'newest-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('newest-stored-codec')).toBeTruthy();
    expect(screen.queryByText('rescanned-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('clears a previous snapshot mutation error when Asset Info closes', async () => {
    vi.mocked(api.startSnapshotOperation).mockRejectedValue(new Error('snapshot test failure'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    expect(await screen.findByText(/snapshot test failure/)).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));

    expect(await screen.findByText('No snapshot available')).toBeTruthy();
    expect(screen.queryByText(/snapshot test failure/)).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('uses canonical snapshot cleanup when Rename closes Asset Info', async () => {
    const analyzedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rename-analyzed-codec');
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(analyzedSnapshot.path, analyzedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    expect(await screen.findByText('rename-analyzed-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Rescan' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rename-newer-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    const renameInput = screen.getByLabelText('Rename file');
    fireEvent.change(renameInput, { target: { value: 'Akira Renamed.mkv' } });
    await user.click(screen.getByRole('button', { name: 'Rename' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    expect(vi.mocked(api.renameAsset).mock.calls[0]?.[0]).toEqual({ path: '/media/raw/movies/Akira/Akira.mkv', fileName: 'Akira Renamed.mkv' });

    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByText('rename-newer-stored-codec')).toBeTruthy();
    expect(screen.queryByText('rename-analyzed-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 10000);

  it('does not reopen a snapshot error after Rename closes Asset Info', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.startSnapshotOperation).mockRejectedValue(new Error('rename snapshot failure'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await user.click(await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }));
    await user.click(await screen.findByRole('button', { name: 'Rescan' }));
    expect(await screen.findByText(/rename snapshot failure/)).toBeTruthy();
    const renameInput = screen.getByLabelText('Rename file');
    fireEvent.change(renameInput, { target: { value: 'Akira Renamed.mkv' } });
    await user.click(screen.getByRole('button', { name: 'Rename' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());

    await user.click(screen.getByRole('button', { name: 'Asset Info Akira.mkv' }));
    expect(await screen.findByRole('tab', { name: 'Asset Information' })).toBeTruthy();
    expect(screen.queryByText(/rename snapshot failure/)).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 10000);

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
      summary: { selected: 1, eligible: 0, queued: 0, skipped: 1, failed: 0, titleCount: 1, sizeBytes: 1024 },
      results: [
        { assetId: 1, outcome: 'skipped', reason: 'already_queued', message: 'Asset already has an open Queue job' },
      ],
      batches: [],
    } : {
      summary: { selected: 2, eligible: 1, queued: 0, skipped: 1, failed: 0, titleCount: 1, sizeBytes: 2048 },
      results: [
        { assetId: assetIds[0], outcome: 'eligible', batchId: 'selected-test-001', batchName: 'Akira' },
        { assetId: assetIds[1], outcome: 'skipped', reason: 'missing', message: 'Asset file is missing' },
      ],
      batches: [],
    });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await waitFor(() => expect(api.queueSelectedAssets).toHaveBeenCalledWith({ assetIds: [1, 2], commit: false }));
    expect(api.effectiveAssetConfigurations).not.toHaveBeenCalled();
    expect(api.createQueueBatch).not.toHaveBeenCalled();
    expect(await screen.findByText('Will be queued: 1')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Queue 1 assets' }));
    await waitFor(() => expect(api.queueSelectedAssets).toHaveBeenCalledWith({ assetIds: [1], commit: true }));
    expect(await screen.findByText('0 queued · 1 skipped · 0 failed')).toBeTruthy();
    expect(screen.getByText('Asset 1: Asset already has an open Queue job')).toBeTruthy();
    expect(api.createQueueBatch).not.toHaveBeenCalled();
  });

  it('keeps the Assets selection and result in place after a successful Queue selected commit', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/assets?tab=unprocessed']}><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await user.click(await screen.findByRole('button', { name: 'Queue 2 assets' }));

    expect(await screen.findByText('2 queued · 0 skipped · 0 failed')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Queue selected titles' })).toBeNull());
    expect(screen.getByRole('checkbox', { name: 'Select title Akira' })).toHaveProperty('checked', true);
    expect(screen.getByRole('heading', { name: 'Akira' })).toBeTruthy();
  });

  it('keeps focus and restores Queue selected actions after a commit failure', async () => {
    vi.mocked(api.queueSelectedAssets).mockImplementation(async ({ assetIds, commit }) => {
      if (commit) throw new Error('Queue service unavailable');
      return {
        summary: { selected: assetIds.length, eligible: assetIds.length, queued: 0, skipped: 0, failed: 0, titleCount: 1, sizeBytes: assetIds.length * 1024 },
        results: assetIds.map((assetId) => ({ assetId, outcome: 'eligible' as const, batchId: 'selected-test-001', batchName: 'Akira' })),
        batches: [],
      };
    });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await user.click(await screen.findByRole('button', { name: 'Queue 2 assets' }));

    expect(await screen.findByText('Queue service unavailable')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Queue 2 assets' }) as HTMLButtonElement).disabled).toBe(false);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Queue selected titles' })).toBeNull());
    expect(screen.getByRole('checkbox', { name: 'Select title Akira' })).toHaveProperty('checked', true);
  });

  it('locks Queue and configuration mutations for selected assets with active jobs but keeps Asset Info available', async () => {
    vi.mocked(api.queueJobs).mockResolvedValue([
      activeQueueJob(101, '/media/raw/movies/Akira/Akira.mkv'),
      activeQueueJob(102, '/media/raw/movies/Akira/extras/Trailer.mkv'),
    ]);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    expect((screen.getByRole('button', { name: 'Queue selected' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Configure selected' }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText('2 assets already locked by an active Queue job. Read-only details remain available.')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Publish as-is' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Run Advisor' }) as HTMLButtonElement).disabled).toBe(false);

    await user.click(screen.getByRole('button', { name: 'Configure title' }));
    expect(await screen.findByText('This scope has an active Queue job. Configuration is read-only until the job finishes.')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Apply' }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Configure title' })).toBeNull());

    await user.click(screen.getByRole('button', { name: 'Expand Akira' }));
    expect((await screen.findByRole('button', { name: 'Asset Info Akira.mkv' }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole('button', { name: 'Queue Akira.mkv' }) as HTMLButtonElement).disabled).toBe(true);
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
