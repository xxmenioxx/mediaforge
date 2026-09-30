// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetGroup, AssetInventory, AssetPath, ExternalSubtitle, ProfileSuggestion, QueueJob, ScanResult, SnapshotOperation } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
    queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
    latestSnapshot: vi.fn(), startSnapshotOperation: vi.fn(), snapshotOperation: vi.fn(), externalAssetSubtitles: vi.fn(), extractAssetSubtitles: vi.fn(), assetMetadataSidecars: vi.fn(), trackMaintenanceInventory: vi.fn(), subtitleEligibility: vi.fn(), subtitleExtractionOperations: vi.fn(),
    suggestProfile: vi.fn(), runtimeState: vi.fn(), workerNodes: vi.fn(), recommendEncoderQuality: vi.fn(),
    profileAssignments: vi.fn(), updateProfileAssignment: vi.fn(), assetScopeConfigurations: vi.fn(), updateAssetScopeConfiguration: vi.fn(), effectiveAssetConfiguration: vi.fn(), effectiveAssetConfigurations: vi.fn(),
    configureLogicalGroupsBatch: vi.fn(), queueSelectedAssets: vi.fn(), queueRemainingPreview: vi.fn(), queueRemainingCommit: vi.fn(), createQueueBatch: vi.fn(), preflightSelectedAssets: vi.fn(), acceptQueueBatchPreflight: vi.fn(), renameAsset: vi.fn(), evaluateAdvisor: vi.fn(), publishAssetsAsIs: vi.fn(),
    updateAssetConversion: vi.fn(), updateAssetMetadata: vi.fn(), updateAssetEdit: vi.fn(), testEncodes: vi.fn(),
    jobArtifacts: vi.fn(), executionPlans: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function testAsset(id: number, path: string, missing = false): Asset {
  return { id, libraryId: 0, libraryName: 'Originals', path, relativePath: path.replace('/media/raw/', ''), groupPath: 'movies/Akira', fileName: path.split('/').pop() ?? '', extension: '.mkv', sizeBytes: 1024, modifiedAt: '2026-08-28T00:00:00Z', status: 'unprocessed', missing, review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' }, metadata: { categories: [], tags: [], updatedAt: '' }, conversion: {} };
}

function testPath(id: string, path: string, displayPath: string, root: boolean, assets: Asset[]): AssetPath {
  return { id, name: displayPath, path, relativePath: displayPath, displayPath, isLogicalGroupRoot: root, fileCount: assets.length, assetCount: assets.length, sizeBytes: assets.length * 1024, totalSizeBytes: assets.length * 1024, assets };
}

function singleAssetInventory(): AssetInventory {
  const asset = testAsset(1, '/media/raw/movies/Akira/Akira.mkv');
  const rootPath = testPath('root', '/media/raw/movies/Akira', 'Root', true, [asset]);
  return {
    sourceGroups: [{ id: 1, name: 'Movies', relativePath: 'movies', sourcePath: '/media/raw/movies', fileCount: 1, assetCount: 1, titleCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024, logicalGroups: [{ id: '/media/raw/movies/Akira', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'Akira', fileCount: 1, assetCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024, assetPaths: [rootPath] }] }],
    unprocessed: [asset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], missing: [], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], reports: {}, sync: { lastSyncedAt: '2026-08-28T00:00:00Z', totalRecords: 1, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
  } as unknown as AssetInventory;
}

function testSnapshot(path = '/media/raw/movies/Akira/Akira.mkv', videoCodec = 'h264'): ScanResult {
  return {
    id: 1, path, fileName: 'Akira.mkv', container: 'matroska', sizeBytes: 1024, duration: 60, bitrate: 1000,
    videoCodec, width: 1920, height: 1080, hdr: false, audioTracks: 1, subtitleTracks: 0, chapters: 0,
    videoStreams: [], audioStreams: [], subtitleStreams: [], attachmentStreams: [], compatibilityAnalysis: {}, interlaceAnalysis: {}, rawProbe: {},
    createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
  } as unknown as ScanResult;
}

function testProfileSuggestion(summary = 'Existing snapshot suggestion'): ProfileSuggestion {
  return {
    matchType: 'create',
    summary,
    scan: testSnapshot(),
    candidates: [],
    proposedProfile: {
      name: 'Suggested profile', scope: 'asset', description: '', container: 'mkv', videoCodec: 'x265', audioCodec: 'copy',
      qualityMode: 'crf', qualityValue: 20, preserveHdr: true, preserveSubtitles: true, preserveChapters: true, workerConfig: {},
    },
    insights: {
      recommendedCrf: 20, estimatedMinBytes: 800, estimatedMaxBytes: 900,
      estimatedSavingsLow: 10, estimatedSavingsHigh: 20, recommendations: [],
    },
    findings: [],
  } as unknown as ProfileSuggestion;
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

function testBatchPreflightCandidate(assetIds: number[], readyIds = assetIds, queuedIds: number[] = []) {
  return {
    id: 'candidate-raw-test', revision: queuedIds.length ? 2 : 1, resolved: true, batchId: 'batch-raw-test', batchName: 'Akira',
    assets: assetIds.map((assetId) => {
      const mediaPath = assetId === 1 ? '/media/raw/movies/Akira/Akira.mkv' : '/media/raw/movies/Akira/extras/Trailer.mkv';
      return {
        id: `asset-${assetId}`, assetRecordId: assetId, mediaPath, batchId: 'batch-raw-test', batchName: 'Akira',
        input: { mediaPath, libraryId: 1, profileId: 1, priority: 5 },
        status: queuedIds.includes(assetId) ? 'queued' as const : readyIds.includes(assetId) ? 'ready' as const : 'blocked' as const,
        blockers: readyIds.includes(assetId) || queuedIds.includes(assetId) ? [] : ['Asset requires review'], warnings: [],
        ...(queuedIds.includes(assetId) ? { queuedJobId: assetId + 100 } : {}),
      };
    }),
  };
}

function LocationSearch() {
  return <output data-testid="location-search">{useLocation().search}</output>;
}

function renderAssetsPage(queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);
}

async function openAssetInfo(user: ReturnType<typeof userEvent.setup>) {
  const details = await screen.findByTestId('selected-asset-detail');
  await user.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
  return screen.findByRole('dialog');
}

async function openTitleActions(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('button', { name: 'More title actions for Akira' }));
}

function openAssetInfoWithFireEvent() {
  const details = screen.getByTestId('selected-asset-detail');
  fireEvent.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Asset Info' }));
}

async function openQuickAssetOverrides(dialog: HTMLElement, user: ReturnType<typeof userEvent.setup>) {
  await user.click(within(dialog).getByRole('tab', { name: 'Quick Asset Overrides' }));
  const quality = await within(dialog).findByRole('combobox', { name: 'Quality / storage intent' });
  await waitFor(() => expect(quality.textContent).toContain('Balanced'));
  return quality;
}

async function chooseSelectOption(select: HTMLElement, option: string, user: ReturnType<typeof userEvent.setup>) {
  await user.click(select);
  await user.click(await screen.findByRole('option', { name: option }));
  await waitFor(() => expect(select.textContent).toContain(option));
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
      unprocessed: [rootAsset, childAsset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], missing: [missingAsset], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], reports: {}, sync: { lastSyncedAt: '2026-08-28T00:00:00Z', totalRecords: 3, missingFiles: 1, missingActionable: 1, missingHistorical: 0 },
    } as unknown as AssetInventory;
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    vi.mocked(api.assetMetadataSidecars).mockResolvedValue([]);
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({ path: '', fingerprint: '', streams: [], chapters: 0, durationSeconds: 0, maintenanceAllowed: true });
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation());
    vi.mocked(api.snapshotOperation).mockResolvedValue(completedSnapshotOperation());
    vi.mocked(api.suggestProfile).mockResolvedValue(testProfileSuggestion());
    vi.mocked(api.runtimeState).mockResolvedValue({ encoders: {} } as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.recommendEncoderQuality).mockResolvedValue(undefined as never);
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([]);
    vi.mocked(api.subtitleEligibility).mockResolvedValue({
      embedded: [], external: [], composition: {
        policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test',
      },
    } as never);
    vi.mocked(api.subtitleExtractionOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.renameAsset).mockResolvedValue({ oldPath: '/media/raw/movies/Akira/Akira.mkv', path: '/media/raw/movies/Akira/Akira Renamed.mkv', fileName: 'Akira Renamed.mkv' });
    vi.mocked(api.evaluateAdvisor).mockImplementation(async ({ mediaPath }) => ({ recommendation: 'worth_it', score: 90, summary: `Ready: ${mediaPath}`, reasons: [], warnings: [] } as never));
    vi.mocked(api.publishAssetsAsIs).mockResolvedValue({ message: 'Published as-is', published: 2 } as never);
    vi.mocked(api.updateAssetConversion).mockResolvedValue({} as never);
    vi.mocked(api.updateAssetEdit).mockImplementation(async ({ path, metadata, conversion }) => ({ path, metadata: { ...metadata, updatedAt: '2026-08-28T00:00:00Z' }, conversion }) as never);
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jobArtifacts).mockResolvedValue({ resultPath: '/reports/job-101.json', asIsPath: '' } as never);
    vi.mocked(api.executionPlans).mockResolvedValue([]);
    vi.mocked(api.preflightSelectedAssets).mockImplementation(async (assetIds) => testBatchPreflightCandidate(assetIds) as never);
    vi.mocked(api.acceptQueueBatchPreflight).mockImplementation(async (_candidateId, _revision, selectedAssetIds) => {
      const queuedIds = selectedAssetIds.map((id) => Number(id.replace('asset-', '')));
      return testBatchPreflightCandidate(queuedIds, [], queuedIds) as never;
    });
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
    vi.mocked(api.queueRemainingPreview).mockImplementation(async (assetIds) => ({
      summary: { selected: assetIds.length, eligible: assetIds.length, queued: 0, skipped: 0, failed: 0, titleCount: 1, sizeBytes: assetIds.length * 1024 },
      results: assetIds.map((assetId) => ({ assetId, outcome: 'eligible' as const, batchId: 'remaining-test-001', batchName: 'Akira' })),
      batches: [],
    }));
    vi.mocked(api.queueRemainingCommit).mockImplementation(async (assetIds) => ({
      summary: { selected: assetIds.length, eligible: 0, queued: assetIds.length, skipped: 0, failed: 0, titleCount: 1, sizeBytes: assetIds.length * 1024 },
      results: assetIds.map((assetId) => ({ assetId, outcome: 'queued' as const, batchId: 'remaining-test-001', batchName: 'Akira', jobId: assetId + 100 })),
      batches: [{ batchId: 'remaining-test-001', batchName: 'Akira', jobCount: assetIds.length }],
    }));
  });

  it('renders RAW title assets in the shared scan table and keeps path actions in context', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(await screen.findByRole('heading', { name: 'Akira' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Configure title' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Run Advisor' })).toBeTruthy();
    await openTitleActions(user);
    expect(screen.getByRole('menuitem', { name: 'Snapshots' })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: 'Publish as-is' })).toBeTruthy();
    await user.keyboard('{Escape}');
    expect(screen.getByText('3 assets · 1 title · 2 paths · 3.00 KB')).toBeTruthy();
    expect(screen.queryByLabelText('Media area')).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Path' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Library' })).toBeNull();
    expect(screen.queryByRole('columnheader', { name: 'Confidence' })).toBeNull();
    await waitFor(() => expect(api.effectiveAssetConfigurations).toHaveBeenCalledWith([1, 2]));

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await waitFor(() => expect(api.effectiveAssetConfigurations).toHaveBeenCalledTimes(1));
    expect(api.effectiveAssetConfigurations).toHaveBeenCalledWith([1, 2]);
    expect(api.effectiveAssetConfiguration).not.toHaveBeenCalled();
    const rawTable = screen.getByRole('table', { name: 'RAW assets' });
    expect(await within(rawTable).findByText('Akira.mkv')).toBeTruthy();
    await user.click(within(rawTable).getByRole('button', { name: 'More actions for Akira.mkv' }));
    expect(screen.getByRole('menuitem', { name: 'Queue asset' })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: 'Mark for review' })).toBeTruthy();
    await user.keyboard('{Escape}');
    const rawNavigation = screen.getByRole('navigation', { name: 'RAW source hierarchy' });
    const rawTree = within(rawNavigation).getByRole('tree', { name: 'RAW sources' });
    expect(within(rawTree).getByRole('treeitem', { name: 'extras' })).toBeTruthy();
    await user.click(within(rawTree).getByRole('treeitem', { name: 'extras' }));
    expect(screen.getAllByRole('button', { name: 'Configure' }).length).toBeGreaterThan(0);
  });

  it('opens and generates Test Encode without persisting unsaved Asset Overrides', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.profiles).mockResolvedValue([{ id: 20, name: 'Persisted profile' }] as never);
    vi.mocked(api.libraries).mockResolvedValue([{ id: 7, name: 'Anime', sourcePath: '/media/raw', destinationPath: '/media/library/anime' }] as never);
    vi.mocked(api.effectiveAssetConfiguration).mockImplementation(async (path) => ({
      assetPath: path,
      video: { selection: 'profile', videoProfileId: 20 },
      audio: { selection: 'inherit' },
      tracks: { selection: 'inherit' },
      category: { selection: 'inherit' },
      destination: { selection: 'inherit' },
    }));
    vi.mocked(api.effectiveAssetConfigurations).mockResolvedValue({
      configurations: {
        '1': { assetPath: '/media/raw/movies/Akira/Akira.mkv', video: { selection: 'profile', videoProfileId: 20 }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } },
        '2': { assetPath: '/media/raw/movies/Akira/extras/Trailer.mkv', video: { selection: 'profile', videoProfileId: 20 }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } },
      },
      missingAssetIds: [],
    });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    expect(await screen.findByRole('tab', { name: 'MVForge Suggestions' })).toBeTruthy();
    await user.click(await screen.findByRole('tab', { name: 'Test Encode' }));
    await user.click(await screen.findByRole('button', { name: 'Generate Test Encode' }));

    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    expect(api.updateAssetConversion).not.toHaveBeenCalled();
  }, 10000);

  it('renders each attachment once with its metadata under Tracks and keeps NFO sidecars there', async () => {
    const path = '/media/raw/movies/Akira/Akira.mkv';
    const snapshot = {
      ...testSnapshot(path),
      attachmentStreams: [
        { index: 4, type: 'attachment' as const, codec: 'ttf', filename: 'SubtitleFont.ttf', mimeType: 'font/ttf', title: 'Subtitle font', attachmentKind: 'FONT', fontFormat: 'TTF' },
        { index: 5, type: 'attachment' as const, codec: 'mjpeg', filename: 'Cover.jpg', mimeType: 'image/jpeg', attachmentKind: 'IMAGE', fontFormat: '' },
        { index: 6, type: 'attachment' as const, codec: 'bin_data', filename: 'Extra.bin', mimeType: 'application/octet-stream', attachmentKind: 'ATTACHMENT', fontFormat: '' },
      ],
    };
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({
      path, fingerprint: 'track-fingerprint', streams: [
        { index: 4, type: 'attachment', codec: 'ttf', fileName: 'SubtitleFont.ttf', mimeType: 'font/ttf', title: 'Subtitle font', attachmentKind: 'FONT', fontFormat: 'TTF', default: false, forced: false, attachedPic: false, stillImage: false },
        { index: 5, type: 'attachment', codec: 'mjpeg', fileName: 'Cover.jpg', mimeType: 'image/jpeg', attachmentKind: 'IMAGE', default: false, forced: false, attachedPic: false, stillImage: false },
        { index: 6, type: 'attachment', codec: 'bin_data', fileName: 'Extra.bin', mimeType: 'application/octet-stream', attachmentKind: 'ATTACHMENT', default: false, forced: false, attachedPic: false, stillImage: false },
      ], chapters: 0, durationSeconds: 60, maintenanceAllowed: true,
    });
    vi.mocked(api.assetMetadataSidecars).mockResolvedValue([{
      path: '/media/raw/movies/Akira/Akira.nfo', fileName: 'Akira.nfo', format: 'nfo', sizeBytes: 128, modifiedAt: '2026-08-28T00:00:00Z',
    }]);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).queryByRole('tab', { name: 'Attachments' })).toBeNull();
    expect(within(dialog).queryByRole('tab', { name: 'Subtitles' })).toBeNull();
    await user.click(within(dialog).getByRole('tab', { name: 'Tracks' }));

    const attachmentRows = await within(dialog).findAllByTestId('asset-track-attachment');
    expect(attachmentRows.length).toBe(3);
    expect(within(dialog).getAllByText(/SubtitleFont\.ttf/).length).toBe(1);
    expect(within(dialog).getAllByText(/Cover\.jpg/).length).toBe(1);
    expect(within(dialog).getAllByText(/Extra\.bin/).length).toBe(1);
    expect(within(dialog).getByText(/Fonts/)).toBeTruthy();
    expect(within(dialog).getByText(/Images/)).toBeTruthy();
    expect(within(dialog).getByText(/Other/)).toBeTruthy();
    expect(within(dialog).getByText(/font\/ttf · TTF/)).toBeTruthy();
    expect(within(dialog).getByText(/SubtitleFont\.ttf · Subtitle font/)).toBeTruthy();
    expect(within(dialog).getByText(/image\/jpeg/)).toBeTruthy();
    expect(within(dialog).getByText(/application\/octet-stream/)).toBeTruthy();
    expect((await within(dialog).findByTestId('asset-nfo-sidecar')).textContent).toContain('Akira.nfo');
    expect(within(dialog).queryByRole('button', { name: /Akira\.nfo/ })).toBeNull();
    expect(api.assetMetadataSidecars).toHaveBeenCalledWith(path);
  });

  it('closes Asset Info immediately without asking to save', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(confirm).not.toHaveBeenCalled();
  });

  it('stays clean after Asset Info preference and editable form hydration completes', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    await openQuickAssetOverrides(dialog, user);
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(confirm).not.toHaveBeenCalled();
  });

  it('prompts after a persisted Asset Info value changes', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    const quality = await openQuickAssetOverrides(dialog, user);
    await chooseSelectOption(quality, 'Maximum space saving', user);
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    expect(confirm).toHaveBeenCalledWith('Discard unsaved Asset Info changes?');
    expect(screen.getByRole('dialog')).toBeTruthy();
  });

  it('returns to clean when a persisted Asset Info value is reverted', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    const quality = await openQuickAssetOverrides(dialog, user);
    await chooseSelectOption(quality, 'Maximum space saving', user);
    await chooseSelectOption(quality, 'Balanced', user);
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(confirm).not.toHaveBeenCalled();
  });

  it('resets the dirty baseline after Asset Info saves successfully', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    const quality = await openQuickAssetOverrides(dialog, user);
    await chooseSelectOption(quality, 'Maximum space saving', user);
    await user.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.updateAssetEdit).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(confirm).not.toHaveBeenCalled();
  });

  it('refreshes new sidecars in Tracks without making a clean Asset Info form dirty', async () => {
    const path = '/media/raw/movies/Akira/Akira.mkv';
    const sidecar: ExternalSubtitle = {
      path: '/media/raw/movies/Akira/Akira.eng.srt', fileName: 'Akira.eng.srt', format: 'srt',
      language: 'eng', default: false, forced: false, sizeBytes: 100, modifiedAt: '',
    };
    const snapshot = { ...testSnapshot(path), subtitleTracks: 1, subtitleStreams: [{ index: 3, codec: 'ass', language: 'eng' }] } as unknown as ScanResult;
    let sidecars: ExternalSubtitle[] = [];
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.externalAssetSubtitles).mockImplementation(async () => sidecars);
    vi.mocked(api.subtitleEligibility).mockImplementation(async () => ({
      embedded: [], external: sidecars,
      composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' },
    } as never));
    vi.mocked(api.extractAssetSubtitles).mockImplementation(async () => {
      sidecars = [sidecar];
      return { created: [sidecar.path], existing: [], unsupported: [] };
    });
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    await user.click(within(dialog).getByRole('tab', { name: 'Tracks' }));
    await user.click(await within(dialog).findByRole('button', { name: 'Generate SRT' }));
    expect(await within(dialog).findByText(sidecar.fileName)).toBeTruthy();
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(confirm).not.toHaveBeenCalled();
  }, 15000);

  it('preserves an unsaved Asset Info edit while Tracks sidecars refresh', async () => {
    const path = '/media/raw/movies/Akira/Akira.mkv';
    const sidecar: ExternalSubtitle = {
      path: '/media/raw/movies/Akira/Akira.eng.srt', fileName: 'Akira.eng.srt', format: 'srt',
      language: 'eng', default: false, forced: false, sizeBytes: 100, modifiedAt: '',
    };
    const snapshot = { ...testSnapshot(path), subtitleTracks: 1, subtitleStreams: [{ index: 3, codec: 'ass', language: 'eng' }] } as unknown as ScanResult;
    let sidecars: ExternalSubtitle[] = [];
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.externalAssetSubtitles).mockImplementation(async () => sidecars);
    vi.mocked(api.subtitleEligibility).mockImplementation(async () => ({
      embedded: [], external: sidecars,
      composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' },
    } as never));
    vi.mocked(api.extractAssetSubtitles).mockImplementation(async () => {
      sidecars = [sidecar];
      return { created: [sidecar.path], existing: [], unsupported: [] };
    });
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    renderAssetsPage();

    const dialog = await openAssetInfo(user);
    const quality = await openQuickAssetOverrides(dialog, user);
    await chooseSelectOption(quality, 'Maximum space saving', user);
    await user.click(within(dialog).getByRole('tab', { name: 'Tracks' }));
    await user.click(await within(dialog).findByRole('button', { name: 'Generate SRT' }));
    expect(await within(dialog).findByText(sidecar.fileName)).toBeTruthy();
    await user.click(within(dialog).getByRole('tab', { name: 'Overview' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Quick Asset Overrides' }));
    const refreshedQuality = await within(dialog).findByRole('combobox', { name: 'Quality / storage intent' });
    expect(refreshedQuality.textContent).toContain('Maximum space saving');
    fireEvent.keyDown(dialog, { key: 'Escape', code: 'Escape' });

    expect(confirm).toHaveBeenCalledWith('Discard unsaved Asset Info changes?');
    expect(screen.getByRole('dialog')).toBeTruthy();
  }, 15000);

  it('does not show backend action-capability diagnostics in Asset Info', async () => {
    const inventory = singleAssetInventory();
    inventory.unprocessed[0].lifecycle = {
      location: 'raw', lifecycle: 'available', provenance: 'legacy_or_unknown', validation: 'unavailable', publication: 'unpublished', original: 'unknown',
      currentPublication: { state: 'unpublished' },
      actions: {
        rename: { allowed: false, reason: 'unsupported' },
        queue: { allowed: false, reason: 'not_raw' },
        lab: { allowed: false, reason: 'unsupported' },
      },
    };
    vi.mocked(api.assets).mockResolvedValue(inventory);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByTestId('asset-info-overview');

    expect(within(dialog).queryByText('Backend action capabilities')).toBeNull();
    expect(dialog.textContent).not.toContain('not_raw');
    expect(dialog.textContent).not.toContain('unsupported');
  });

  it('shows one title-scoped Snapshots surface with root and nested assets', async () => {
    vi.mocked(api.latestSnapshot).mockImplementation(async (path) => path.endsWith('/Akira.mkv')
      ? { found: true, snapshot: testSnapshot(path), status: 'current', requiresAnalysis: false, staleComponents: [] }
      : { found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await openTitleActions(user);
    const snapshots = screen.getByRole('menuitem', { name: 'Snapshots' });
    expect(snapshots).toBeTruthy();
    await user.click(snapshots);
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('heading', { name: 'Snapshots · Akira' })).toBeTruthy();
    expect(within(dialog).getByText('Akira.mkv')).toBeTruthy();
    expect(within(dialog).getByText('Trailer.mkv')).toBeTruthy();
    expect(within(dialog).getByRole('cell', { name: 'Root' })).toBeTruthy();
    expect(within(dialog).getByRole('cell', { name: 'extras' })).toBeTruthy();
    expect(await within(dialog).findByRole('button', { name: 'Regenerate' })).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Generate' })).toBeTruthy();

    await user.click(within(dialog).getByRole('button', { name: 'Regenerate' }));
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

    await openTitleActions(user);
    await user.click(screen.getByRole('menuitem', { name: 'Publish as-is' }));
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

    await openTitleActions(user);
    await user.click(screen.getByRole('menuitem', { name: 'Publish as-is' }));
    expect(await screen.findByText('Configure the same effective Destination for every asset in this title before publishing as-is.')).toBeTruthy();
    expect(api.publishAssetsAsIs).not.toHaveBeenCalled();
  });

  it('blocks title Publish as-is when an asset requires review', async () => {
    const inventory = await api.assets();
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[1].assets[0].review!.requiresReview = true;
    vi.mocked(api.assets).mockResolvedValue(inventory);
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await openTitleActions(userEvent.setup());
    expect((screen.getByRole('menuitem', { name: 'Publish as-is' }) as HTMLLIElement).getAttribute('aria-disabled')).toBe('true');
  });

  it('keeps nested path configuration as an explicit inheritance-aware override', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    const rawTree = screen.getByRole('tree', { name: 'RAW sources' });
    await user.click(within(rawTree).getByRole('treeitem', { name: 'extras' }));
    const pathActions = screen.getByRole('table', { name: 'Actions for /media/raw/movies/Akira/extras' });
    await user.click(within(pathActions).getByRole('button', { name: 'Configure' }));
    expect(await screen.findByRole('heading', { name: 'Configure' })).toBeTruthy();
    expect((screen.getByLabelText('Video profile') as HTMLInputElement).value).toContain('Inherit');
    expect((screen.getByLabelText('Audio profile') as HTMLInputElement).value).toContain('Inherit');
    expect((screen.getByLabelText('Tracks profile') as HTMLInputElement).value).toContain('Inherit');
    expect(screen.getByLabelText('Destination mode').textContent).toContain('Inherit');
    expect(screen.queryByRole('button', { name: /Snapshots for/ })).toBeNull();
  });

  it.each([
    ['current stored snapshot', { found: true, snapshot: testSnapshot(), status: 'current' as const, requiresAnalysis: false, staleComponents: [], analysisEvidence: { status: 'ready' as const, sourceIdentity: { path: '/media/raw/movies/Akira/Akira.mkv', sizeBytes: 100, modifiedAtUnixNano: 1, snapshotId: 1, matchesCurrentSourceRevision: true } } }],
    ['stale stored snapshot', { found: true, snapshot: testSnapshot(), status: 'stale' as const, requiresAnalysis: true, staleComponents: ['interlace'], analysisEvidence: { status: 'stale' as const, staleComponents: ['interlace'] } }],
    ['no stored snapshot', { found: false, snapshot: null, status: 'missing' as const, requiresAnalysis: true, staleComponents: [], analysisEvidence: { status: 'missing' as const } }],
    ['unavailable stored snapshot', { found: false, snapshot: null, status: 'unavailable' as const, requiresAnalysis: false, staleComponents: [], analysisEvidence: { status: 'unavailable' as const, unavailableReason: 'source_unavailable' } }],
  ])('keeps Asset Info read-only for %s', async (_label, snapshotState) => {
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByRole('heading', { name: 'Asset Snapshot' })).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledWith('/media/raw/movies/Akira/Akira.mkv'));
    const statusLabel = _label === 'current stored snapshot' ? 'Ready' : _label === 'stale stored snapshot' ? 'Stale' : _label === 'no stored snapshot' ? 'Missing' : 'Unavailable';
    await waitFor(() => expect(within(screen.getByRole('dialog')).getAllByText(statusLabel).length).toBeGreaterThan(0));
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
    if (_label === 'no stored snapshot') {
      expect(screen.getByText(/No snapshot available/)).toBeTruthy();
      expect(screen.getByRole('button', { name: 'Analyze asset' })).toBeTruthy();
      expect(screen.getByRole('tab', { name: 'Overview' })).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Refresh analysis' })).toBeNull();
    } else if (_label === 'unavailable stored snapshot') {
      expect(screen.getByText(/Analysis evidence is unavailable: source_unavailable/)).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Refresh analysis' })).toBeNull();
    } else {
      expect(screen.getByRole('tab', { name: 'Overview' })).toBeTruthy();
      expect(screen.queryByRole('button', { name: 'Analyze asset' })).toBeNull();
      if (_label === 'stale stored snapshot') {
        expect(screen.getByText(/Snapshot evidence is stale/)).toBeTruthy();
      } else {
        expect(screen.queryByText(/Snapshot evidence is stale/)).toBeNull();
      }
      expect(screen.getByRole('button', { name: 'Refresh analysis' })).toBeTruthy();
    }
  }, 15000);

  it('loads Suggestions from an existing snapshot without Refresh analysis', async () => {
    const snapshotState = { found: true, snapshot: testSnapshot(), status: 'current' as const, requiresAnalysis: false, staleComponents: [] };
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.suggestProfile).mockResolvedValue(testProfileSuggestion('Suggestion from the existing snapshot'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'MVForge Suggestions' }));
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledWith('/media/raw/movies/Akira/Akira.mkv'));
    await waitFor(() => expect(api.suggestProfile).toHaveBeenCalledWith('/media/raw/movies/Akira/Akira.mkv'));
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();

    expect(await screen.findByText('Suggestion from the existing snapshot')).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
  });

  it('does not request Suggestions when no snapshot exists', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText('No snapshot available for the current source revision.')).toBeTruthy();
    expect(api.suggestProfile).not.toHaveBeenCalled();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
  });

  it('opens persisted lifecycle Overview without heavy work and loads evidence/history lazily', async () => {
    const inventory = singleAssetInventory();
    const asset = inventory.unprocessed[0];
    const currentPath = '/media/library/anime/Akira/Akira.mkv';
    const lifecycle = {
      location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { state: 'published', authority: 'queue_job', id: 101, publishedPath: currentPath, publishedAt: '2026-08-27T00:00:00Z' },
      evidenceSummary: { queueJobIds: [100, 101], directPublicationIds: [55], hasAnalysis: true, analysis: { status: 'ready' } },
      actions: { archive: { allowed: false, reason: 'active Queue operation' } },
    };
    const enrichedAsset = { ...asset, libraryId: 0, libraryName: 'Anime', lifecycle } as typeof asset;
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[0].assets[0] = enrichedAsset;
    vi.mocked(api.assets).mockResolvedValue({
      ...inventory,
      unprocessed: [enrichedAsset],
      library: [enrichedAsset],
    } as never);
    vi.mocked(api.libraries).mockResolvedValue([{ id: 1, name: 'Anime', sourcePath: '/media/raw/anime', destinationPath: '/media/library/anime', type: 'anime', validationRules: {}, createdAt: '', updatedAt: '' }]);
    vi.mocked(api.queueJobs).mockResolvedValue([
      { ...activeQueueJob(100, asset.path), status: 'completed', stage: 'completed', publishedPath: `${currentPath}.old`, publicationRetiredAt: '2026-08-27T00:00:00Z' },
      {
        ...activeQueueJob(101, asset.path), status: 'completed', stage: 'completed', publishedPath: currentPath,
        validationStatus: 'failed', validationScore: 97, validationRunId: 'run-53-101',
        validationOutcome: 'PASS_WITH_WARNINGS', validationOverridePresent: true, validationHasActionableWarnings: true,
        sourceAssetRecordId: asset.id, activeExecutionPlanId: 202,
        profileSnapshot: { sourceAnalysisEvidence: {
          status: 'ready', sourceAssetRecordId: asset.id,
          sourceIdentity: { path: asset.path, snapshotId: 77, matchesCurrentSourceRevision: true },
        } },
      },
    ]);

    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);

    expect(await screen.findByTestId('asset-info-overview')).toBeTruthy();
    const dialog = screen.getByRole('dialog');
    const mainNavigation = within(dialog).getByRole('tablist', { name: 'Asset Info sections' });
    expect(within(mainNavigation).getAllByRole('tab').map((tab) => tab.textContent)).toEqual(['Overview', 'Tracks', 'Job Information']);
    expect(within(within(dialog).getByTestId('asset-info-overview')).getByText('Published', { selector: 'p' })).toBeTruthy();
    const overview = within(dialog).getByTestId('asset-info-overview');
    expect(within(overview).queryByText('queue_job')).toBeNull();
    expect(within(overview).queryByText(/Queue job #101|run-53-101/)).toBeNull();
    expect(within(dialog).getByText('Anime')).toBeTruthy();
    expect(within(dialog).getByText('Converted by MVForge')).toBeTruthy();
    expect(within(dialog).queryByText(currentPath)).toBeNull();
    expect(within(dialog).queryByText('Integration evidence')).toBeNull();
    expect(within(dialog).queryByText(/active Queue operation/)).toBeNull();
    expect(within(dialog).queryByText('Backend action capabilities')).toBeNull();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledWith(asset.path));
    expect(api.latestSnapshot).toHaveBeenCalledTimes(1);
    expect(api.externalAssetSubtitles).not.toHaveBeenCalled();
    expect(api.subtitleEligibility).not.toHaveBeenCalled();
    expect(api.suggestProfile).not.toHaveBeenCalled();
    expect(api.jobArtifacts).not.toHaveBeenCalled();

    await user.click(within(mainNavigation).getByRole('tab', { name: 'Job Information' }));
    const jobNavigation = within(dialog).getByRole('tablist', { name: 'Job Information sections' });
    expect(within(jobNavigation).getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'Execution / Result', 'Validation', 'Original', 'Evidence', 'History', 'Current publication', 'Integration evidence',
    ]);
    expect(within(dialog).queryByText('Validation: passed 97/100')).toBeNull();
    await user.click(within(jobNavigation).getByRole('tab', { name: 'Validation' }));
    expect(within(dialog).getByText('Outcome · Passed with warnings')).toBeTruthy();
    expect(within(dialog).getByText('Validation run · run-53-101')).toBeTruthy();
    expect(within(dialog).getByText('Override present · Yes')).toBeTruthy();
    expect(within(dialog).getByText('Actionable warnings · Yes')).toBeTruthy();
    expect(within(dialog).getByText('Latest score · 97/100')).toBeTruthy();
    await user.click(within(jobNavigation).getByRole('tab', { name: 'Current publication' }));
    expect(within(dialog).getByText('Authority · queue_job')).toBeTruthy();
    expect(within(dialog).getByText('Identity · Queue job #101')).toBeTruthy();
    expect(within(dialog).getByText(currentPath)).toBeTruthy();
    expect(within(dialog).getByText(/^Published /)).toBeTruthy();

    await user.click(within(jobNavigation).getByRole('tab', { name: 'Integration evidence' }));
    expect(within(dialog).getByText('Analysis · Ready')).toBeTruthy();
    expect(within(dialog).getByText('Validation · Passed with warnings · run run-53-101')).toBeTruthy();
    expect(within(dialog).getByText('Publication · Published')).toBeTruthy();
    expect(within(dialog).getByText('2 Queue jobs')).toBeTruthy();
    expect(within(dialog).getByText('1 direct publications')).toBeTruthy();

    await user.click(within(jobNavigation).getByRole('tab', { name: 'Evidence' }));
    expect(within(dialog).getByText('Queue job #100 · completed · historical')).toBeTruthy();
    expect(within(dialog).getByText('Queue job #101 · completed')).toBeTruthy();
    expect(api.jobArtifacts).not.toHaveBeenCalled();
    expect(api.executionPlans).not.toHaveBeenCalled();
    await user.click(within(dialog).getAllByRole('button', { name: 'Load report summary' })[0]);
    await waitFor(() => expect(api.jobArtifacts).toHaveBeenCalledWith(101));
    await waitFor(() => expect(api.executionPlans).toHaveBeenCalledWith(101));

    await user.click(within(jobNavigation).getByRole('tab', { name: 'History' }));
    expect(within(dialog).getByText(new RegExp(`Queue result #101 .* current`))).toBeTruthy();
    expect(within(dialog).getByText(new RegExp(`Queue result #100 .* historical`))).toBeTruthy();
    expect(within(dialog).getByText(new RegExp(`Source evidence snapshot #77 · Ready .* frozen plan #202 → Queue result #101`))).toBeTruthy();
    expect(within(dialog).getByText('Source evidence not linked.')).toBeTruthy();
    expect(within(dialog).getByText(/Direct publication #55/)).toBeTruthy();
  });

  it('labels validation without a #53 run as legacy and unbound', async () => {
    const inventory = singleAssetInventory();
    const asset = inventory.unprocessed[0];
    const publishedPath = '/media/library/anime/Akira/Akira.mkv';
    const enrichedAsset = {
      ...asset,
      lifecycle: {
        location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
        currentPublication: { state: 'published', authority: 'queue_job', id: 401, publishedPath },
        evidenceSummary: { queueJobIds: [401], directPublicationIds: [], hasAnalysis: true, analysis: { status: 'ready' } },
      },
    } as typeof asset;
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[0].assets[0] = enrichedAsset;
    vi.mocked(api.assets).mockResolvedValue({ ...inventory, unprocessed: [enrichedAsset], library: [enrichedAsset] } as never);
    vi.mocked(api.queueJobs).mockResolvedValue([
      { ...activeQueueJob(401, asset.path), status: 'completed', stage: 'completed', publishedPath, validationStatus: 'passed', validationScore: 88 },
    ]);

    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Job Information' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Validation' }));

    expect(within(dialog).getByText('Legacy/unbound validation status · Passed')).toBeTruthy();
    expect(within(dialog).getByText('Latest score · 88/100')).toBeTruthy();
    expect(within(dialog).queryByText(/Validation run/)).toBeNull();

    await user.click(within(dialog).getByRole('tab', { name: 'Integration evidence' }));
    expect(within(dialog).getByText('Analysis · Ready')).toBeTruthy();
    expect(within(dialog).getByText('Validation · Legacy/unbound · Passed')).toBeTruthy();
    expect(within(dialog).getByText('Publication · Published')).toBeTruthy();
  });

  it('opens the requested RAW asset Overview from a legacy Analysis deep link without media work', async () => {
    const path = '/media/raw/movies/Akira/Akira.mkv';
    render(<MemoryRouter initialEntries={[`/assets?location=raw&asset=${encodeURIComponent(path)}`]}><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    expect(await screen.findByTestId('asset-info-overview')).toBeTruthy();
    expect(screen.getByRole('tab', { name: 'Overview' }).getAttribute('aria-selected')).toBe('true');
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledWith(path));
    expect(api.latestSnapshot).toHaveBeenCalledTimes(1);
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
    expect(api.evaluateAdvisor).not.toHaveBeenCalled();
  });

  it('shows a useful empty state when an asset has no linked MVForge processing history', async () => {
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Job Information' }));
    expect(within(dialog).getByText('No MVForge processing history is linked to this asset.')).toBeTruthy();
    expect(within(dialog).queryByText('No snapshot available')).toBeNull();
    await user.click(within(dialog).getByRole('tab', { name: 'History' }));
    expect(within(dialog).getByText('No MVForge processing history is linked to this asset.')).toBeTruthy();
  });

  it('does not bind a historical job to an asset by path alone', async () => {
    const inventory = singleAssetInventory();
    const asset = inventory.unprocessed[0];
    const lifecycle = {
      location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { state: 'published', authority: 'publisher', id: 55, publishedPath: asset.path },
      evidenceSummary: { queueJobIds: [], directPublicationIds: [], hasAnalysis: false },
    };
    const enrichedAsset = { ...asset, lifecycle } as typeof asset;
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[0].assets[0] = enrichedAsset;
    vi.mocked(api.assets).mockResolvedValue({ ...inventory, unprocessed: [enrichedAsset], library: [enrichedAsset] } as never);
    vi.mocked(api.queueJobs).mockResolvedValue([
      { ...activeQueueJob(301, asset.path), status: 'completed', stage: 'completed', publishedPath: asset.path, publicationRetiredAt: '2026-08-27T00:00:00Z' },
    ]);

    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Job Information' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Current publication' }));
    expect(within(dialog).getByText('Authority · publisher')).toBeTruthy();
    expect(within(dialog).getByText('Identity · Publication #55')).toBeTruthy();
    expect(within(dialog).queryByText(/Queue job #55/)).toBeNull();
    await user.click(within(dialog).getByRole('tab', { name: 'History' }));

    expect(within(dialog).queryByText(/Queue result #301/)).toBeNull();
    expect(within(dialog).getByText('No MVForge processing history is linked to this asset.')).toBeTruthy();
  });

  it('keeps publication IDs distinct from Queue job IDs in History', async () => {
    const inventory = singleAssetInventory();
    const asset = inventory.unprocessed[0];
    const lifecycle = {
      location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { state: 'published', authority: 'publisher', id: 301, publishedPath: asset.path },
      evidenceSummary: { queueJobIds: [301], directPublicationIds: [301], hasAnalysis: false },
    };
    const enrichedAsset = { ...asset, lifecycle } as typeof asset;
    inventory.sourceGroups[0].logicalGroups[0].assetPaths[0].assets[0] = enrichedAsset;
    vi.mocked(api.assets).mockResolvedValue({ ...inventory, unprocessed: [enrichedAsset], library: [enrichedAsset] } as never);
    vi.mocked(api.queueJobs).mockResolvedValue([
      { ...activeQueueJob(301, asset.path), status: 'completed', stage: 'completed', publishedPath: asset.path },
    ]);

    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Job Information' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Current publication' }));
    expect(within(dialog).getByText('Authority · publisher')).toBeTruthy();
    expect(within(dialog).getByText('Identity · Publication #301')).toBeTruthy();
    expect(within(dialog).queryByText(/Queue job #301/)).toBeNull();
    await user.click(within(dialog).getByRole('tab', { name: 'History' }));

    expect(within(dialog).getByText(new RegExp(`Queue result #301 .* historical`))).toBeTruthy();
    expect(within(dialog).getByText(/Direct publication #301/)).toBeTruthy();
  });

  it('keeps legacy Analysis decisions and notes visible without path-only association', async () => {
    vi.mocked(api.settings).mockResolvedValue([{ key: 'analysisRecords', value: { records: [{ id: 'legacy-1', assetName: 'Akira.mkv', assetPath: '/old/Akira.mkv', decision: 'Keep source', notes: 'Manual review note', createdAt: '2025-01-01' }] } }] as never);
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Legacy Analysis decisions (1)' }));
    expect(screen.getByText(/Keep source/)).toBeTruthy();
    expect(screen.getByText('Manual review note')).toBeTruthy();
    expect(screen.getByText(/Legacy record · \/old\/Akira.mkv/)).toBeTruthy();
    expect(screen.getByText(/path equality alone is never sufficient/)).toBeTruthy();
  });

  it('refreshes Suggestions when Refresh analysis changes the snapshot identity', async () => {
    const initialSnapshot = { ...testSnapshot(), updatedAt: '2026-08-28T00:00:01Z' } as ScanResult;
    const rescannedSnapshot = { ...testSnapshot(), updatedAt: '2026-08-28T00:05:00Z' } as ScanResult;
    vi.mocked(api.latestSnapshot)
      .mockResolvedValueOnce({ found: true, snapshot: initialSnapshot, status: 'current', requiresAnalysis: false, staleComponents: [] })
      .mockResolvedValue({ found: true, snapshot: rescannedSnapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(rescannedSnapshot.path, rescannedSnapshot));
    vi.mocked(api.suggestProfile)
      .mockResolvedValueOnce(testProfileSuggestion('Initial snapshot suggestion'))
      .mockResolvedValueOnce(testProfileSuggestion('Refreshed snapshot suggestion'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'MVForge Suggestions' }));
    await waitFor(() => expect(api.suggestProfile).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('tab', { name: 'MVForge Suggestions' }));
    expect(await screen.findByText('Initial snapshot suggestion')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Refresh analysis' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(api.suggestProfile).toHaveBeenCalledTimes(2));
    expect(await screen.findByText('Refreshed snapshot suggestion')).toBeTruthy();
  });

  it('uses backend GOP Recommendation v2 in Quick Asset Overrides', async () => {
    const gopSnapshot = {
      ...testSnapshot(),
      frameStructureAnalysis: {
        framesAnalyzed: 120, averageGopLength: 14.9, medianGopLength: 15, maxConsecutiveBFrames: 2, confidence: 'high',
      },
      frameStructureRecommendation: {
        version: 2, fps: 24000 / 1001, sourceFps: 24000 / 1001, sourceAnchorFrames: 15,
        sourceAnchorSeconds: 15 / (24000 / 1001), autoStrategy: 'balanced', analysisDriven: true,
        recommendedMaxBFrames: 2,
        byMode: {
          compatible: { targetGopFrames: 15, targetGopSeconds: 15 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
          balanced: { targetGopFrames: 19, targetGopSeconds: 19 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
          maximum_compression: { targetGopFrames: 26, targetGopSeconds: 26 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
        },
      },
    } as unknown as ScanResult;
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: gopSnapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(await screen.findByRole('tab', { name: 'Quick Asset Overrides' }));

    expect(await screen.findByText(/Effective GOP: 19 frames/)).toBeTruthy();
    expect(screen.getByText(/Candidates: Compatible 15 · Balanced 19 · Maximum Compression 26/)).toBeTruthy();
    expect(screen.getByText('Analysis-based GOP recommendation')).toBeTruthy();
  });

  it('starts one snapshot operation only after Analyze asset is clicked', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(api.startSnapshotOperation).toHaveBeenCalledWith({ path: '/media/raw/movies/Akira/Akira.mkv', force: true });
  });

  it('starts one forced snapshot operation only after Refresh analysis is clicked', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByRole('heading', { name: 'Asset Snapshot' })).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Refresh analysis' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(api.startSnapshotOperation).toHaveBeenCalledWith({ path: '/media/raw/movies/Akira/Akira.mkv', force: true });
  });

  it('does not re-arm Analyze after success, snapshot refetch, or rerenders', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    const user = userEvent.setup();
    const view = render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));

    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    await queryClient.invalidateQueries({ queryKey: ['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'] });
    await waitFor(() => expect(screen.getByText(/Snapshot evidence is stale/)).toBeTruthy());
    view.rerender(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    view.rerender(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 10000);

  it('does not observe asset scope configuration per AssetRow until Asset Info opens', async () => {
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    expect(await screen.findByRole('heading', { name: 'Akira' })).toBeTruthy();
    await waitFor(() => expect(api.assetScopeConfigurations).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', { name: 'Expand Akira' }));
    expect(within(await screen.findByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for Akira.mkv' })).toBeTruthy();
    expect(api.assetScopeConfigurations).toHaveBeenCalledTimes(1);

    openAssetInfoWithFireEvent();
    await waitFor(() => expect(api.assetScopeConfigurations).toHaveBeenCalledTimes(2));
  });

  it('hydrates an explicit asset Destination override in Asset Info', async () => {
    const snapshotState: Awaited<ReturnType<typeof api.latestSnapshot>> = { found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] };
    const assetConfigurations = [{
      id: 5, scopeType: 'asset', scopeKey: '/media/raw/movies/Akira/Akira.mkv',
      categorySelection: 'inherit', category: '', destinationSelection: 'value', destinationLibraryId: 5,
      createdAt: '', updatedAt: '',
    }];
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.assets).mockResolvedValue(singleAssetInventory());
    vi.mocked(api.libraries).mockResolvedValue([{ id: 5, name: 'Movies', sourcePath: '/media/raw', destinationPath: '/media/library/movies', type: 'movies', validationRules: {}, createdAt: '', updatedAt: '' }]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue(assetConfigurations as never);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'], snapshotState);
    queryClient.setQueryData(['assetScopeConfigurations'], assetConfigurations);
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    fireEvent.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    expect(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for Akira.mkv' })).toHaveProperty('disabled', false);
    openAssetInfoWithFireEvent();

    const dialog = await screen.findByRole('dialog');
    const destinationMode = await within(dialog).findByLabelText('Destination mode');
    const destination = await within(dialog).findByLabelText('Destination');
    await waitFor(() => expect(destinationMode.textContent).toContain('Override'));
    await waitFor(() => expect((destination as HTMLInputElement).value).toBe('Movies'));
  }, 10000);

  it('edits and resets the Asset Info semantic role override with snapshot evidence', async () => {
    const snapshot = {
      ...testSnapshot(),
      subtitleTracks: 1,
      subtitleStreams: [{ index: 3, codec: 'subrip', language: 'spa', role: 'signs', roleProvenance: 'heuristic', roleConfidence: 'medium' }],
    } as unknown as ScanResult;
    const snapshotState: Awaited<ReturnType<typeof api.latestSnapshot>> = { found: true, snapshot, status: 'current', requiresAnalysis: false, staleComponents: [] };
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.assets).mockResolvedValue(singleAssetInventory());
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    queryClient.setQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'], snapshotState);
    queryClient.setQueryData(['assetScopeConfigurations'], []);
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    fireEvent.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    openAssetInfoWithFireEvent();
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('tab', { name: 'Analysis' }));
    expect(queryClient.getQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'])).toEqual(snapshotState);
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByText('Video format');
    fireEvent.click(within(dialog).getByRole('tab', { name: 'Tracks' }));
    const user = userEvent.setup();
    const role = (await within(dialog).findAllByLabelText('#3 · SPA · SUBRIP')).at(-1)!;
    expect(role.textContent).toContain('Automatic');

    await user.click(role);
    await user.click(await screen.findByRole('option', { name: 'commentary' }));
    expect(api.updateAssetConversion).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole('tab', { name: 'Overview' }));
    fireEvent.click(within(dialog).getByRole('tab', { name: 'Quick Asset Overrides' }));
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.updateAssetEdit).toHaveBeenCalledTimes(1));
    expect(api.updateAssetEdit).toHaveBeenCalledWith(expect.objectContaining({
      path: '/media/raw/movies/Akira/Akira.mkv',
      conversion: expect.objectContaining({
        subtitleRoleOverrides: expect.objectContaining({ '3': expect.objectContaining({ role: 'commentary' }) }),
      }),
    }));

    fireEvent.click(within(dialog).getByRole('tab', { name: 'Tracks' }));
    const refreshedRole = within(dialog).getAllByLabelText('#3 · SPA · SUBRIP').at(-1)!;
    await user.click(refreshedRole);
    await user.click(await screen.findByRole('option', { name: /^Automatic/ }));
    expect(api.updateAssetEdit).toHaveBeenCalledTimes(1);
  }, 20000);

  it('shows the inherited effective Destination in Asset Info', async () => {
    const snapshotState: Awaited<ReturnType<typeof api.latestSnapshot>> = { found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] };
    const assetConfigurations = [{
      id: 5, scopeType: 'asset', scopeKey: '/media/raw/movies/Akira/Akira.mkv',
      categorySelection: 'inherit', category: '', destinationSelection: 'inherit', destinationLibraryId: 0,
      createdAt: '', updatedAt: '',
    }];
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.assets).mockResolvedValue(singleAssetInventory());
    vi.mocked(api.libraries).mockResolvedValue([{ id: 5, name: 'Movies', sourcePath: '/media/raw', destinationPath: '/media/library/movies', type: 'movies', validationRules: {}, createdAt: '', updatedAt: '' }]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue(assetConfigurations as never);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((assetId) => [String(assetId), { assetPath: `/asset/${assetId}`, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'value', destinationLibraryId: 5 } }])),
      missingAssetIds: [],
    }));
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'], snapshotState);
    queryClient.setQueryData(['assetScopeConfigurations'], assetConfigurations);
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    fireEvent.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await waitFor(() => expect(api.effectiveAssetConfigurations).toHaveBeenCalled());
    expect(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for Akira.mkv' })).toHaveProperty('disabled', false);
    openAssetInfoWithFireEvent();

    const dialog = await screen.findByRole('dialog');
    const destinationMode = await within(dialog).findByLabelText('Destination mode');
    await waitFor(() => expect(destinationMode.textContent).toContain('Inherit'));
    expect(within(dialog).getByText('Destination applied · inherited').parentElement?.textContent).toContain('Movies');
  });

  it('rehydrates persisted Destination after reopening Asset Info', async () => {
    const snapshotState: Awaited<ReturnType<typeof api.latestSnapshot>> = { found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] };
    const assetConfigurations = [{
      id: 5, scopeType: 'asset', scopeKey: '/media/raw/movies/Akira/Akira.mkv',
      categorySelection: 'inherit', category: '', destinationSelection: 'inherit', destinationLibraryId: 0,
      createdAt: '', updatedAt: '',
    }];
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.assets).mockResolvedValue(singleAssetInventory());
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue(assetConfigurations as never);
    const inventory = singleAssetInventory();
    const effectiveConfigurations = {
      configurations: { '1': { assetPath: '/media/raw/movies/Akira/Akira.mkv', video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } } },
      missingAssetIds: [],
    };
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    queryClient.setQueryData(['queueJobs'], []);
    queryClient.setQueryData(['assets'], inventory);
    queryClient.setQueryData(['profiles'], []);
    queryClient.setQueryData(['libraries'], []);
    queryClient.setQueryData(['settings'], []);
    queryClient.setQueryData(['snapshotOperations'], { operations: [] });
    queryClient.setQueryData(['profileAssignments'], []);
    queryClient.setQueryData(['effectiveAssetConfiguration', 'batch', 1], effectiveConfigurations);
    queryClient.setQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'], snapshotState);
    queryClient.setQueryData(['assetScopeConfigurations'], assetConfigurations);
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    const expandButton = document.querySelector<HTMLButtonElement>('[aria-label="Expand Akira"]');
    expect(expandButton).not.toBeNull();
    fireEvent.click(expandButton!);
    const detailActions = within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for Akira.mkv' });
    expect(detailActions).toHaveProperty('disabled', false);
    openAssetInfoWithFireEvent();
    let dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog).not.toBeNull();
    let destinationMode = within(dialog!).getByLabelText('Destination mode');
    expect(destinationMode.textContent).toContain('Inherit');

    const destinationModeInput = destinationMode.parentElement?.querySelector('input');
    expect(destinationModeInput).not.toBeNull();
    fireEvent.change(destinationModeInput!, { target: { value: 'value' } });
    expect(destinationMode.textContent).toContain('Override');
    expect(api.updateAssetScopeConfiguration).not.toHaveBeenCalled();

    fireEvent.keyDown(dialog!, { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    openAssetInfoWithFireEvent();
    dialog = await screen.findByRole('dialog');
    destinationMode = within(dialog).getByLabelText('Destination mode');
    expect(destinationMode.textContent).toContain('Inherit');
    expect(api.updateAssetScopeConfiguration).not.toHaveBeenCalled();
  }, 10000);

  it('keeps every Asset Info configuration control locked for an active Queue job', async () => {
    const snapshotState: Awaited<ReturnType<typeof api.latestSnapshot>> = { found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] };
    vi.mocked(api.latestSnapshot).mockResolvedValue(snapshotState);
    vi.mocked(api.assets).mockResolvedValue(singleAssetInventory());
    vi.mocked(api.queueJobs).mockResolvedValue([activeQueueJob(101, '/media/raw/movies/Akira/Akira.mkv')]);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'], snapshotState);
    queryClient.setQueryData(['assetScopeConfigurations'], []);
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    fireEvent.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    expect(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for Akira.mkv' })).toHaveProperty('disabled', false);
    openAssetInfoWithFireEvent();
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('heading', { name: 'Overview' });

    for (const control of within(dialog).getAllByLabelText('Video profile')) expect(control).toHaveProperty('disabled', true);
    expect(within(dialog).getByLabelText('Audio profile')).toHaveProperty('disabled', true);
    expect(within(dialog).getByLabelText('Tracks profile')).toHaveProperty('disabled', true);
    expect(within(dialog).getByLabelText('Destination mode').getAttribute('aria-disabled')).toBe('true');
    expect(within(dialog).getByLabelText('Destination')).toHaveProperty('disabled', true);
    expect(within(dialog).getByLabelText('Rename file')).toHaveProperty('disabled', true);
  });

  it('renames a published converted asset when only a different asset has an active Queue job', async () => {
    const path = '/media/library/series/Akira/Converted Episode.mkv';
    const convertedAsset = {
      ...testAsset(11, path),
      libraryId: 1,
      libraryName: 'Series',
      relativePath: 'Akira/Converted Episode.mkv',
      groupPath: 'Akira',
      fileName: 'Converted Episode.mkv',
      status: 'unverified',
      lifecycle: {
        location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
        currentPublication: { state: 'published', id: 120 },
        actions: { rename: { allowed: true }, lab: { allowed: false, reason: 'not_raw' } },
      },
    } as Asset;
    const convertedGroup = {
      id: 'converted-akira',
      libraryId: 1,
      libraryName: 'Series',
      path: '/media/library/series/Akira',
      relativePath: 'Akira',
      status: 'converted',
      fileCount: 1,
      sizeBytes: convertedAsset.sizeBytes,
      modifiedAt: convertedAsset.modifiedAt,
      assets: [convertedAsset],
      review: convertedAsset.review,
      pathReview: convertedAsset.review,
      metadata: convertedAsset.metadata,
      pathMetadata: convertedAsset.metadata,
    };
    const inventory = {
      ...singleAssetInventory(),
      sourceGroups: [],
      unprocessed: [],
      converted: [convertedAsset],
      convertedGroups: [convertedGroup],
    } as unknown as AssetInventory;
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.queueJobs).mockResolvedValue([activeQueueJob(202, '/media/library/series/Other/Other.mkv')]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([{
      id: 12, scopeType: 'path', scopeKey: '/media/library/series/Akira',
      categorySelection: 'inherit', category: '', destinationSelection: 'inherit', destinationLibraryId: 0,
      createdAt: '', updatedAt: '',
    }] as never);
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(path), status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.renameAsset).mockResolvedValue({ oldPath: path, path: '/media/library/series/Akira/Akira - S01E01.mkv', fileName: 'Akira - S01E01.mkv' });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('tab', { name: /Library/ }));
    await user.click(await screen.findByRole('combobox', { name: 'Provenance' }));
    await user.click(await screen.findByRole('option', { name: /Converted/ }));
    const libraryTree = await screen.findByRole('tree', { name: 'Library folders' });
    await user.click(within(libraryTree).getByRole('button', { name: 'Expand Series' }));
    await user.click(within(libraryTree).getByRole('treeitem', { name: 'Akira' }));
    const convertedRow = await screen.findByText('Converted Episode.mkv');
    await user.click(convertedRow.closest('tr')!);
    const convertedDetails = await screen.findByTestId('selected-asset-detail');
    await user.click(within(convertedDetails).getByRole('button', { name: 'More actions for Converted Episode.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Asset Info' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Overview' }));
    expect(within(dialog).queryByRole('tab', { name: 'MVForge Suggestions' })).toBeNull();
    expect(within(dialog).queryByRole('tab', { name: 'Test Encode' })).toBeNull();
    const renameInput = within(dialog).getByLabelText('Rename file');
    expect(renameInput).toHaveProperty('disabled', false);
    fireEvent.change(renameInput, { target: { value: 'Akira - S01E01.mkv' } });
    await user.click(within(dialog).getByRole('button', { name: 'Rename' }));

    await waitFor(() => expect(vi.mocked(api.renameAsset).mock.calls[0]?.[0]).toEqual({ path, fileName: 'Akira - S01E01.mkv' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

	it.each([
		['queued', 'queued', 'QUEUED'],
		['running', 'claimed', 'PREPARING'],
		['running', 'converting', 'PROCESSING'],
	] as const)('shows Queue execution state %s/%s as %s without changing inventory state', async (status, stage, label) => {
		const inventory = singleAssetInventory();
		vi.mocked(api.assets).mockResolvedValue(inventory);
		vi.mocked(api.queueJobs).mockResolvedValue([{ ...activeQueueJob(101, '/media/raw/movies/Akira/Akira.mkv'), status, stage }]);
		render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

		fireEvent.click(await screen.findByRole('button', { name: 'Expand Akira' }));
		const rawTable = screen.getByRole('table', { name: 'RAW assets' });
		expect(await within(rawTable).findByText(label)).toBeTruthy();
		expect(inventory.unprocessed[0].status).toBe('unprocessed');
	});

  it('does not re-arm Refresh analysis after success and snapshot refetch', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Refresh analysis' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    await queryClient.invalidateQueries({ queryKey: ['assetSnapshot', '/media/raw/movies/Akira/Akira.mkv'] });

    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('keeps close and reopen read-only', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'stale', requiresAnalysis: true, staleComponents: ['interlace'] });
    const user = userEvent.setup();
    const view = render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText(/Snapshot evidence is stale/)).toBeTruthy();
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText(/Snapshot evidence is stale/)).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();

    view.unmount();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText(/Snapshot evidence is stale/)).toBeTruthy();
    expect(api.startSnapshotOperation).not.toHaveBeenCalled();
  });

  it('uses the latest stored snapshot after Analyze succeeds, closes, and reopens', async () => {
    const analyzedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'analyzed-codec');
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(analyzedSnapshot.path, analyzedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('analyzed-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Refresh analysis' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'newer-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText('newer-stored-codec')).toBeTruthy();
    expect(screen.queryByText('analyzed-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('does not let a completed Refresh analysis result override a newer stored snapshot after reopen', async () => {
    const rescannedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rescanned-codec');
    vi.mocked(api.latestSnapshot)
      .mockResolvedValueOnce({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'initial-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] })
      .mockResolvedValue({ found: true, snapshot: rescannedSnapshot, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(rescannedSnapshot.path, rescannedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Refresh analysis' }));
    await waitFor(() => expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('rescanned-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Refresh analysis' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'newest-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText('newest-stored-codec')).toBeTruthy();
    expect(screen.queryByText('rescanned-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 10000);

  it('clears a previous snapshot mutation error when Asset Info closes', async () => {
    vi.mocked(api.startSnapshotOperation).mockRejectedValue(new Error('snapshot test failure'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    expect(await screen.findByText(/snapshot test failure/)).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));

    expect(await screen.findByText('No snapshot available for the current source revision.')).toBeTruthy();
    expect(screen.queryByText(/snapshot test failure/)).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  });

  it('uses canonical snapshot cleanup when Rename closes Asset Info', async () => {
    const analyzedSnapshot = testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rename-analyzed-codec');
    vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshotOperation(analyzedSnapshot.path, analyzedSnapshot));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Analyze asset' }));
    expect(await screen.findByText('rename-analyzed-codec')).toBeTruthy();
    await waitFor(() => expect((screen.getByRole('button', { name: 'Refresh analysis' }) as HTMLButtonElement).disabled).toBe(false));
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot('/media/raw/movies/Akira/Akira.mkv', 'rename-newer-stored-codec'), status: 'current', requiresAnalysis: false, staleComponents: [] });

    await user.click(screen.getByRole('tab', { name: 'Overview' }));
    const renameInput = screen.getByLabelText('Rename file');
    fireEvent.change(renameInput, { target: { value: 'Akira Renamed.mkv' } });
    await user.click(screen.getByRole('button', { name: 'Rename' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());
    expect(vi.mocked(api.renameAsset).mock.calls[0]?.[0]).toEqual({ path: '/media/raw/movies/Akira/Akira.mkv', fileName: 'Akira Renamed.mkv' });

    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    expect(await screen.findByText('rename-newer-stored-codec')).toBeTruthy();
    expect(screen.queryByText('rename-analyzed-codec')).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 20000);

  it('does not reopen a snapshot error after Rename closes Asset Info', async () => {
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.startSnapshotOperation).mockRejectedValue(new Error('rename snapshot failure'));
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    await openAssetInfo(user);
    await user.click(screen.getByRole('tab', { name: 'Analysis' }));
    await user.click(await screen.findByRole('button', { name: 'Refresh analysis' }));
    expect(await screen.findByText(/rename snapshot failure/)).toBeTruthy();
    await user.click(screen.getByRole('tab', { name: 'Overview' }));
    const renameInput = screen.getByLabelText('Rename file');
    fireEvent.change(renameInput, { target: { value: 'Akira Renamed.mkv' } });
    await user.click(screen.getByRole('button', { name: 'Rename' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Asset Snapshot' })).toBeNull());

    await openAssetInfo(user);
    expect(await screen.findByRole('tab', { name: 'Overview' })).toBeTruthy();
    expect(screen.queryByText(/rename snapshot failure/)).toBeNull();
    expect(api.startSnapshotOperation).toHaveBeenCalledTimes(1);
  }, 20000);

  it('selects a RAW title and keeps missing descendants visible when browsing a path', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    const titleCheckbox = await screen.findByRole('checkbox', { name: 'Select title Akira' });
    await user.click(titleCheckbox);
    expect((titleCheckbox as HTMLInputElement).checked).toBe(true);
    expect(screen.getByText('1 title selected · 2 assets · 2.00 KB')).toBeTruthy();

    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    const rawTree = screen.getByRole('tree', { name: 'RAW sources' });
    await user.click(within(rawTree).getByRole('treeitem', { name: 'extras' }));
    const rawTable = screen.getByRole('table', { name: 'RAW assets' });
    expect(await within(rawTable).findByText('Missing.mkv')).toBeTruthy();
    expect((within(rawTable).getByRole('checkbox', { name: 'Select asset Missing.mkv' }) as HTMLInputElement).disabled).toBe(true);
    expect(within(rawTable).getByText('Trailer.mkv')).toBeTruthy();
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

  it('previews Queue selected and routes the same Asset IDs into Batch Preflight', async () => {
    vi.mocked(api.queueSelectedAssets).mockImplementation(async ({ assetIds, commit }) => commit ? {
      summary: { selected: 1, eligible: 0, queued: 0, skipped: 1, failed: 0, titleCount: 1, sizeBytes: 1024 },
      results: [
        { assetId: 1, outcome: 'skipped', reason: 'already_queued', message: 'Asset already has an open Queue job', analysisEvidence: { status: 'ready', sourceIdentity: { path: '/media/raw/movies/Akira/Akira.mkv', snapshotId: 11, sizeBytes: 100, modifiedAtUnixNano: 1, matchesCurrentSourceRevision: true } } },
      ],
      batches: [],
    } : {
      summary: { selected: 2, eligible: 1, queued: 0, skipped: 1, failed: 0, titleCount: 1, sizeBytes: 2048 },
      results: [
        { assetId: assetIds[0], outcome: 'eligible', batchId: 'selected-test-001', batchName: 'Akira', analysisEvidence: { status: 'ready', sourceIdentity: { path: '/media/raw/movies/Akira/Akira.mkv', snapshotId: 11, sizeBytes: 100, modifiedAtUnixNano: 1, matchesCurrentSourceRevision: true } } },
        { assetId: assetIds[1], outcome: 'skipped', reason: 'missing', message: 'Asset file is missing', analysisEvidence: { status: 'stale', staleComponents: ['cadence'] } },
      ],
      batches: [],
    });
    const user = userEvent.setup();
    vi.mocked(api.preflightSelectedAssets).mockImplementation(async (assetIds) => testBatchPreflightCandidate(assetIds, [1]) as never);
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await waitFor(() => expect(api.queueSelectedAssets).toHaveBeenCalledWith({ assetIds: [1, 2], commit: false }));
    await waitFor(() => expect(api.effectiveAssetConfigurations).toHaveBeenCalledWith([1, 2]));
    expect(api.createQueueBatch).not.toHaveBeenCalled();
    expect(await screen.findByText('Will be queued: 1')).toBeTruthy();
    const preflight = screen.getByRole('dialog');
    expect(within(preflight).getByText('Ready')).toBeTruthy();
    expect(within(preflight).getByText('Stale')).toBeTruthy();
    expect(within(preflight).getByText('Snapshot #11')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Queue 1 assets' }));
    await waitFor(() => expect(api.preflightSelectedAssets).toHaveBeenCalledWith([1, 2]));
    const batchPreflight = await screen.findByRole('dialog', { name: 'Batch Preflight · Akira' });
    expect(within(batchPreflight).getByText(/Reviewed plan is the plan frozen into Queue/)).toBeTruthy();
    expect(within(batchPreflight).getByText('Asset requires review')).toBeTruthy();
    await user.click(within(batchPreflight).getByRole('button', { name: 'Accept & Queue (1)' }));
    await waitFor(() => expect(api.acceptQueueBatchPreflight).toHaveBeenCalledWith('candidate-raw-test', 1, ['asset-1']));
    expect(api.createQueueBatch).not.toHaveBeenCalled();
  });

  it('Queue Remaining previews then opens Batch Preflight with the same complete group IDs', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    await user.click(screen.getByRole('button', { name: 'Queue Remaining' }));
    await waitFor(() => expect(api.queueRemainingPreview).toHaveBeenCalledWith([1, 2]));
    const dialogQueueButton = (await screen.findAllByRole('button', { name: /Queue Remaining/ })).at(-1)!;
    await user.click(dialogQueueButton);
    await waitFor(() => expect(api.preflightSelectedAssets).toHaveBeenCalledWith([1, 2]));
    const batchPreflight = await screen.findByRole('dialog', { name: 'Batch Preflight · Akira' });
    await user.click(within(batchPreflight).getByRole('button', { name: 'Accept & Queue (2)' }));
    await waitFor(() => expect(api.acceptQueueBatchPreflight).toHaveBeenCalledWith('candidate-raw-test', 1, ['asset-1', 'asset-2']));
  });

  it('keeps the RAW title selection in place after Batch Preflight accepts Queue selected', async () => {
    const user = userEvent.setup();
    render(<MemoryRouter initialEntries={['/assets?tab=unprocessed']}><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);

    await user.click(await screen.findByRole('checkbox', { name: 'Select title Akira' }));
    const assetCallsBeforeQueue = vi.mocked(api.assets).mock.calls.length;
    const scopeCallsBeforeQueue = vi.mocked(api.assetScopeConfigurations).mock.calls.length;
    await user.click(screen.getByRole('button', { name: 'Queue selected' }));
    await user.click(await screen.findByRole('button', { name: 'Queue 2 assets' }));

    const batchPreflight = await screen.findByRole('dialog', { name: 'Batch Preflight · Akira' });
    await user.click(within(batchPreflight).getByRole('button', { name: 'Accept & Queue (2)' }));
    await waitFor(() => expect(api.acceptQueueBatchPreflight).toHaveBeenCalledWith('candidate-raw-test', 1, ['asset-1', 'asset-2']));
    await waitFor(() => expect(vi.mocked(api.assets).mock.calls.length).toBeGreaterThan(assetCallsBeforeQueue));
    expect(vi.mocked(api.assetScopeConfigurations).mock.calls.length).toBe(scopeCallsBeforeQueue);
    await user.click(within(batchPreflight).getByRole('button', { name: 'Close' }));
    expect(screen.getByRole('checkbox', { name: 'Select title Akira' })).toHaveProperty('checked', true);
    expect(screen.getByRole('heading', { name: 'Akira' })).toBeTruthy();
  });

  it('keeps focus and restores Queue selected actions after preflight creation fails', async () => {
    vi.mocked(api.preflightSelectedAssets).mockRejectedValue(new Error('Queue service unavailable'));
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
    expect((screen.getByRole('button', { name: 'Run Advisor' }) as HTMLButtonElement).disabled).toBe(false);
    await openTitleActions(user);
    expect((screen.getByRole('menuitem', { name: 'Publish as-is' }) as HTMLLIElement).getAttribute('aria-disabled')).toBe('true');
    await user.keyboard('{Escape}');

    await user.click(screen.getByRole('button', { name: 'Configure title' }));
    expect(await screen.findByText('This scope has an active Queue job. Configuration is read-only until the job finishes.')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Apply' }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Configure title' })).toBeNull());

    await user.click(screen.getByRole('button', { name: 'Expand Akira' }));
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
    expect((screen.getByRole('menuitem', { name: 'Asset Info' }) as HTMLLIElement).getAttribute('aria-disabled')).toBeNull();
    expect((screen.getByRole('menuitem', { name: 'Queue asset' }) as HTMLLIElement).getAttribute('aria-disabled')).toBe('true');
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

  it('keeps Library path actions singular and exposes persisted filters through Advanced', async () => {
    const user = userEvent.setup();
    const convertedAsset: Asset = {
      ...testAsset(201, '/media/library/anime/Akira/Akira.mkv'),
      libraryId: 1,
      libraryName: 'Anime',
      status: 'converted',
      lifecycle: {
        location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed',
        publication: 'published', original: 'archived',
      },
    };
    const convertedGroup: AssetGroup = {
      id: 'akira', libraryId: 1, libraryName: 'Anime', path: '/media/library/anime/Akira', relativePath: 'anime/Akira',
      status: 'converted', fileCount: 1, sizeBytes: 1024, modifiedAt: '', assets: [convertedAsset],
      review: convertedAsset.review, pathReview: convertedAsset.review, metadata: convertedAsset.metadata, pathMetadata: convertedAsset.metadata,
    };
    vi.mocked(api.assets).mockResolvedValue({
      ...singleAssetInventory(),
      sourceGroups: [], unprocessed: [], library: [convertedAsset], converted: [convertedAsset],
      libraryGroups: [convertedGroup], convertedGroups: [convertedGroup],
    } as unknown as AssetInventory);
    vi.mocked(api.settings).mockResolvedValue([{ key: 'paths', value: { rawRoot: '/media/raw', libraryRoot: '/media/library' } }] as never);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([{
      id: 12, scopeType: 'path', scopeKey: '/media/library/anime/Akira', categorySelection: 'inherit', category: '',
      destinationSelection: 'inherit', destinationLibraryId: 0, createdAt: '', updatedAt: '',
    }] as never);
    render(
      <MemoryRouter initialEntries={['/assets?location=library&provenance=converted&original=archived']}>
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
          <AssetsPage />
          <LocationSearch />
        </QueryClientProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByRole('combobox', { name: 'Media area' })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: 'Provenance' })).toBeTruthy();
    expect(screen.getByPlaceholderText('Search path, file, library, status')).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: 'Original' })).toBeNull();
    await user.click(screen.getByRole('button', { name: /Advanced filters/ }));
    await user.click(screen.getByRole('combobox', { name: 'Original' }));
    expect(screen.queryByRole('option', { name: 'Archived' })).toBeNull();
    await user.keyboard('{Escape}');
    const libraryNavigation = await screen.findByRole('navigation', { name: 'Library tree' });
    await screen.findByRole('treeitem', { name: 'Anime' });
    await user.click(within(libraryNavigation).getByRole('treeitem', { name: 'Anime' }));
    await user.click(within(libraryNavigation).getByRole('button', { name: 'Expand Anime' }));
    await user.click(within(libraryNavigation).getByRole('treeitem', { name: 'anime' }));
    await user.click(within(libraryNavigation).getByRole('button', { name: 'Expand anime' }));
    await user.click(within(libraryNavigation).getByRole('treeitem', { name: 'Akira' }));
    await user.click(screen.getByRole('button', { name: 'More actions for /media/library/anime/Akira' }));
    await user.click(screen.getByRole('menuitem', { name: 'Path configuration and more actions' }));
    expect(screen.getAllByRole('button', { name: 'Move path' })).toHaveLength(1);
    expect(screen.getByRole('combobox', { name: 'Move path to library' })).toBeTruthy();

    await user.click(screen.getByRole('combobox', { name: 'Media area' }));
    await user.click(screen.getByRole('option', { name: 'media/anime' }));
    await user.type(screen.getByPlaceholderText('Search path, file, library, status'), 'Akira');
    const persisted = new URLSearchParams(screen.getByTestId('location-search').textContent ?? '');
    expect(persisted.get('provenance')).toBe('converted');
    expect(persisted.has('original')).toBe(false);
    expect(persisted.get('mediaArea')).toBe('anime');
    expect(persisted.get('search')).toBe('Akira');
  });
});
