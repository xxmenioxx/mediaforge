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
    publishedPath: '/media/library/Akira.mkv', publishedAt: '2026-08-28T00:01:00Z', originalArchiveStatus: archiveStatus,
    originalArchivedPath: archivedPath, replacementTargetPath: '', createdAt: '2026-08-28T00:00:00Z',
    updatedAt: '2026-08-28T00:00:00Z',
  } as QueueJob;
}

function renderAssetsPage(initialEntry = '/') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<MemoryRouter initialEntries={[initialEntry]}><QueryClientProvider client={queryClient}><AssetsPage /></QueryClientProvider></MemoryRouter>);
  return queryClient;
}

function libraryAsset(fileName: string, id: number, archivedOriginals?: unknown[]): Asset {
  const libraryAsset = {
    ...asset(),
    id,
    libraryId: 1,
    libraryName: 'Library',
    path: `/media/library/${fileName}`,
    relativePath: fileName,
    groupPath: '/media/library',
    fileName,
    status: 'library',
  } as Asset;
  if (archivedOriginals !== undefined) {
    libraryAsset.lifecycle = {
      location: 'library', lifecycle: 'available', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { authority: 'queue_job', id: id + 400, state: 'PUBLISHED', publishedPath: libraryAsset.path },
      archivedOriginals,
    } as never;
  }
  return libraryAsset;
}

function libraryInventory(rows: Asset[]): AssetInventory {
  const base = inventory();
  const group = {
    id: '/media/library', libraryId: 1, libraryName: 'Library', path: '/media/library', relativePath: '', status: 'library',
    fileCount: rows.length, sizeBytes: rows.length * 1024, modifiedAt: '2026-08-28T00:00:00Z', assets: rows,
    review: base.unprocessed[0].review, pathReview: base.unprocessed[0].review,
    metadata: base.unprocessed[0].metadata, pathMetadata: base.unprocessed[0].metadata,
  };
  return { ...base, library: rows, libraryGroups: [group] } as unknown as AssetInventory;
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

  it('shows the compact Library indicator only for current available originals and reflects refreshed lifecycle data', async () => {
    const currentAvailable = {
      assetRecordId: 801, queueJobId: 401, path: '/media/archive/current.mkv', sizeBytes: 1024,
      availability: 'available', relationship: 'current', retentionState: 'retained',
      provenance: { authority: 'queue_job', recordId: 401, sourcePath: '/media/raw/current.mkv', publishedPath: '/media/library/Available.mkv', sourceAssetRecordId: 1, current: true },
      restore: { allowed: false, reason: 'active_work' },
    };
    const libraryRows = [
      libraryAsset('Available.mkv', 1, [currentAvailable]),
      libraryAsset('Historical.mkv', 2, [{ ...currentAvailable, relationship: 'historical' }]),
      libraryAsset('SourceReplaced.mkv', 3, [{ ...currentAvailable, relationship: 'historical', queueJobId: 402 }]),
      libraryAsset('Missing.mkv', 4, [{ ...currentAvailable, availability: 'missing' }]),
      libraryAsset('Unresolved.mkv', 5, [{ ...currentAvailable, relationship: 'unresolved' }]),
      libraryAsset('NoArchive.mkv', 6, []),
      libraryAsset('NoLifecycle.mkv', 7),
    ];
    vi.mocked(api.assets).mockResolvedValueOnce(libraryInventory(libraryRows)).mockResolvedValue(libraryInventory(libraryRows.slice(1)));

    const queryClient = renderAssetsPage('/assets?location=library');
    await waitFor(() => expect(queryClient.getQueryData(['assets'])).toBeDefined());
    const indicator = await screen.findByRole('img', { name: 'Original disponible en Archive' });
    expect(screen.getAllByRole('img', { name: 'Original disponible en Archive' })).toHaveLength(1);
    expect(indicator.closest('td')?.textContent).toContain('Available.mkv');
    const rows = screen.getAllByTestId('asset-list-row');
    expect(rows).toHaveLength(libraryRows.length);
    expect(new Set(rows.map((row) => row.querySelectorAll(':scope > td').length)).size).toBe(1);
    expect(vi.mocked(api.assets)).toHaveBeenCalledTimes(1);

    const availableRow = indicator.closest('[data-testid="asset-list-row"]');
    expect(availableRow).toBeTruthy();
    const user = userEvent.setup();
    await user.hover(indicator);
    expect((await screen.findByRole('tooltip')).textContent).toBe('Original disponible en Archive');
    await user.click(screen.getByText('Available.mkv'));
    expect(availableRow?.getAttribute('aria-selected')).toBe('true');

    await act(async () => { await queryClient.invalidateQueries({ queryKey: ['assets'] }); });
    await waitFor(() => expect(screen.queryByRole('img', { name: 'Original disponible en Archive' })).toBeNull());
  });

  it('does not show the Library indicator for RAW assets', async () => {
    const rawInventory = inventory();
    rawInventory.unprocessed[0].lifecycle = {
      location: 'raw', lifecycle: 'available', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { authority: 'queue_job', id: 401, state: 'PUBLISHED', publishedPath: '/media/library/Akira.mkv' },
      archivedOriginals: [{ relationship: 'current', availability: 'available' }],
    } as never;
    vi.mocked(api.assets).mockResolvedValue(rawInventory);

    renderAssetsPage('/assets?location=raw');
    await screen.findByRole('button', { name: 'Expand Akira' });
    expect(screen.queryByRole('img', { name: 'Original disponible en Archive' })).toBeNull();
  });

  it('shows a temporary Rename explanation during canonical archival and unlocks after the lifecycle refresh', async () => {
    const pendingInventory = inventory();
    pendingInventory.unprocessed[0].lifecycle = {
      location: 'raw', lifecycle: 'available', provenance: 'converted', validation: 'passed', publication: 'published', original: 'available',
      currentPublication: { authority: 'queue_job', id: 401, state: 'PUBLISHED', publishedPath: '/media/library/Akira.mkv' },
      actions: { rename: { allowed: false, reason: 'original_archive_in_progress' } },
    } as never;
    const completeInventory = inventory();
    completeInventory.unprocessed[0].lifecycle = {
      location: 'raw', lifecycle: 'available', provenance: 'converted', validation: 'passed', publication: 'published', original: 'archived',
      currentPublication: { authority: 'queue_job', id: 401, state: 'PUBLISHED', publishedPath: '/media/library/Akira.mkv' },
      actions: { rename: { allowed: true } },
    } as never;
    vi.mocked(api.assets).mockResolvedValueOnce(pendingInventory).mockResolvedValue(completeInventory);
    vi.mocked(api.queueJobs).mockResolvedValueOnce([queueJob('pending')]).mockResolvedValue([queueJob('complete', '/media/archive/Akira.mkv')]);

    const user = userEvent.setup();
    const queryClient = renderAssetsPage();
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const dialog = await screen.findByRole('dialog');
    const renameField = await within(dialog).findByRole('textbox', { name: 'Rename file' });
    expect(renameField).toHaveProperty('disabled', true);
    expect(within(dialog).getByText('Original archival is still in progress. Try Rename again once it finishes.')).toBeTruthy();

    await act(async () => { await queryClient.invalidateQueries({ queryKey: ['assets'] }); });
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Rename file' })).toHaveProperty('disabled', false));
    expect(within(dialog).queryByText('Original archival is still in progress. Try Rename again once it finishes.')).toBeNull();
  }, 10000);
});
