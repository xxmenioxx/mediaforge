// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, AssetPath, QueueJob } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: Object.fromEntries(Object.entries(actual.api).map(([key, endpoint]) => [
      key,
      typeof endpoint === 'function' ? vi.fn().mockResolvedValue([]) : endpoint,
    ])),
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function asset(): Asset {
  return {
    id: 1, libraryId: 0, libraryName: 'Originals', path: '/media/raw/movies/Akira/Akira.mkv',
    relativePath: 'movies/Akira/Akira.mkv', groupPath: 'movies/Akira', fileName: 'Akira.mkv', extension: '.mkv',
    sizeBytes: 1024, modifiedAt: '2026-08-28T00:00:00Z', status: 'unprocessed', missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' }, conversion: {},
  } as Asset;
}

function inventory(): AssetInventory {
  const sourceAsset = asset();
  const path: AssetPath = {
    id: 'root', name: 'Root', path: '/media/raw/movies/Akira', relativePath: 'Root', displayPath: 'Root',
    isLogicalGroupRoot: true, fileCount: 1, assetCount: 1, sizeBytes: 1024, totalSizeBytes: 1024, assets: [sourceAsset],
  };
  return {
    sourceGroups: [{
      id: 1, name: 'Movies', relativePath: 'movies', sourcePath: '/media/raw/movies', fileCount: 1,
      assetCount: 1, titleCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024,
      logicalGroups: [{
        id: '/media/raw/movies/Akira', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'Akira',
        fileCount: 1, assetCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024, assetPaths: [path],
      }],
    }],
    unprocessed: [sourceAsset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], missing: [],
    libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], reports: {},
    sync: { lastSyncedAt: '', totalRecords: 1, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
  } as unknown as AssetInventory;
}

function queueJob(archiveStatus: 'pending' | 'archiving' | 'complete', archivedPath = ''): QueueJob {
  return {
    id: 401, batchId: 'archive-test', batchName: 'Archive test', batchPosition: 1,
    mediaPath: '/media/raw/movies/Akira/Akira.mkv', publishMode: 'standard', libraryId: 1,
    profileId: 1, profileVersion: 1, profileSnapshot: {}, audioProfileKey: '', trackProfileKey: '',
    processingMode: 'full_encode', priority: 5, queuePosition: 1, status: 'completed', stage: 'completed',
    stageHistory: [], progress: 100, workerName: '', outputPath: '', plannedPublishedPath: '', errorMessage: '', notes: '',
    validationStatus: 'passed', validationScore: 100, validationReport: {}, publicationState: 'PUBLISHED',
    publishedPath: '/media/library/Akira.mkv', originalArchiveStatus: archiveStatus,
    originalArchivedPath: archivedPath, replacementTargetPath: '', createdAt: '2026-08-28T00:00:00Z',
    updatedAt: '2026-08-28T00:00:00Z',
  } as QueueJob;
}

function renderAssetsPage() {
  return render(<MemoryRouter><QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><AssetsPage /></QueryClientProvider></MemoryRouter>);
}

describe('Assets page original archive lifecycle', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    for (const endpoint of Object.values(api)) {
      if (typeof endpoint === 'function') {
        (endpoint as ReturnType<typeof vi.fn>).mockReset().mockResolvedValue([] as never);
      }
    }
    vi.mocked(api.assets).mockResolvedValue(inventory());
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] } as never);
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: false, snapshot: null, status: 'missing', requiresAnalysis: true, staleComponents: [] });
    vi.mocked(api.assetMetadataSidecars).mockResolvedValue([]);
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({ path: '', fingerprint: '', streams: [], chapters: 0, durationSeconds: 0, maintenanceAllowed: true });
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([]);
    vi.mocked(api.subtitleEligibility).mockResolvedValue({ embedded: [], external: [], composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' } } as never);
    vi.mocked(api.subtitleExtractionOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.effectiveAssetConfigurations).mockResolvedValue({ configurations: {}, missingAssetIds: [] });
    vi.mocked(api.effectiveAssetConfiguration).mockResolvedValue({ assetPath: '', video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } } as never);
    vi.mocked(api.profileAssignments).mockResolvedValue([]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.runtimeState).mockResolvedValue({ encoders: {} } as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jobArtifacts).mockResolvedValue({ resultPath: '', asIsPath: '' } as never);
    vi.mocked(api.executionPlans).mockResolvedValue([]);
  });

  it.each(['pending', 'archiving'] as const)('refreshes Asset Info while archive is %s and shows only the committed canonical original', async (initialArchiveStatus) => {
    const archivedPath = '/media/archive/originals/Akira-source.mkv';
    const pendingInventory = inventory();
    const pendingLifecycle = {
      location: 'raw', lifecycle: 'available', provenance: 'converted', validation: 'passed', publication: 'published', original: 'available',
      currentPublication: { authority: 'queue_job', id: 401, state: 'PUBLISHED', publishedPath: '/media/library/Akira.mkv' }, archivedOriginals: [],
    };
    pendingInventory.unprocessed[0].lifecycle = pendingLifecycle as never;
    const completedInventory = inventory();
    completedInventory.unprocessed[0].lifecycle = {
      ...pendingLifecycle,
      original: 'archived',
      archivedOriginals: [{
        assetRecordId: 801, queueJobId: 401, path: archivedPath, sizeBytes: 1024,
        archivedAt: '2026-10-10T12:00:00Z', availability: 'available', relationship: 'current',
        provenance: { authority: 'queue_job', recordId: 401, sourcePath: '/media/raw/movies/Akira/Akira.mkv', publishedPath: '/media/library/Akira.mkv', sourceAssetRecordId: 1, current: true },
        retentionState: 'retained',
      }],
    } as never;
    vi.mocked(api.assets).mockResolvedValueOnce(pendingInventory).mockResolvedValue(completedInventory);
    vi.mocked(api.queueJobs).mockResolvedValueOnce([queueJob(initialArchiveStatus)]).mockResolvedValue([queueJob('complete', archivedPath)]);

    const user = userEvent.setup();
    renderAssetsPage();
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('tab', { name: 'Job Information' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Original' }));

    expect(await within(dialog).findByText(`Original archive · ${initialArchiveStatus === 'pending' ? 'Pending' : 'Archiving'}`)).toBeTruthy();
    expect(within(dialog).getByText('No archived originals linked to this asset.')).toBeTruthy();
    expect(within(dialog).queryByText(archivedPath)).toBeNull();
    const assetCallsBeforeInterval = vi.mocked(api.assets).mock.calls.length;
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 3300)); });

    await waitFor(() => expect(vi.mocked(api.assets).mock.calls.length).toBeGreaterThan(assetCallsBeforeInterval));
    expect(await within(dialog).findByText(archivedPath)).toBeTruthy();
    expect(within(dialog).getByText('Original archive · Complete')).toBeTruthy();
  }, 10000);
});
