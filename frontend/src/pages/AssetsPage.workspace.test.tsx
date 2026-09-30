// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetGroup, AssetInventory, AssetPath, AssetSourceGroup, Library, ProfileAssignment, ScanResult } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
      queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
      profileAssignments: vi.fn(), assetScopeConfigurations: vi.fn(), latestSnapshot: vi.fn(), testEncodes: vi.fn(), jellyfinOutcomes: vi.fn(), effectiveAssetConfigurations: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

afterEach(cleanup);

const path = '/media/library/anime/Series/Season 01';

function asset(id: number, fileName: string, tags: string[] = []): Asset {
  return {
    id,
    libraryId: 1,
    libraryName: 'Anime',
    path: `${path}/${fileName}`,
    relativePath: `Series/Season 01/${fileName}`,
    groupPath: path,
    fileName,
    extension: '.mkv',
    sizeBytes: 1_500_000,
    modifiedAt: '2026-09-30T10:00:00Z',
    status: 'unverified',
    missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: tags.length ? ['Anime'] : [], tags, updatedAt: '' },
    conversion: {},
    technical: { videoCodec: 'hevc', width: 1920, height: 1080, duration: 1420, bitrate: 4_000_000, hdr: false },
  };
}

function inventory(assets: Asset[]): AssetInventory {
  const review = assets[0]?.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
  const metadata = assets[0]?.metadata ?? { categories: [], tags: [], updatedAt: '' };
  const group: AssetGroup = {
    id: 'season-01', libraryId: 1, libraryName: 'Anime', path, relativePath: 'Series/Season 01', status: 'unverified',
    fileCount: assets.length, sizeBytes: assets.reduce((total, value) => total + value.sizeBytes, 0), modifiedAt: '2026-09-30T10:00:00Z',
    assets, review, pathReview: review, metadata, pathMetadata: metadata,
  };
  return {
    sourceGroups: [], raw: [], unprocessed: [], library: assets, converted: [], unverified: assets, accepted: [], missing: [],
    unprocessedGroups: [], rawGroups: [], libraryGroups: [group], convertedGroups: [], unverifiedGroups: [group], acceptedGroups: [],
    reports: {},
    sync: { lastSyncedAt: '2026-09-30T10:00:00Z', totalRecords: assets.length, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
    counts: { raw: 0, library: assets.length, processing: 1, needsReview: 0, validationReview: 0, publicationBlocked: 0, converted: 3, originalsArchived: 0, missing: 0, historicalPaths: 0 },
  } as unknown as AssetInventory;
}

function rawWorkspaceInventory(): AssetInventory {
  const rootAsset: Asset = { ...asset(11, 'S01E01.mkv'), libraryId: 0, libraryName: 'RAW', path: '/media/raw/anime/Attack on Titan/S01E01.mkv', relativePath: 'anime/Attack on Titan/S01E01.mkv', groupPath: 'Attack on Titan', status: 'unprocessed' };
  const extraAsset: Asset = { ...asset(12, 'Trailer.mkv'), libraryId: 0, libraryName: 'RAW', path: '/media/raw/anime/Attack on Titan/extras/Trailer.mkv', relativePath: 'anime/Attack on Titan/extras/Trailer.mkv', groupPath: 'Attack on Titan/extras', status: 'unprocessed' };
  const rootPath = { id: 'root', name: 'Root', path: '/media/raw/anime/Attack on Titan', relativePath: 'Root', displayPath: 'Root', isLogicalGroupRoot: true, fileCount: 1, assetCount: 1, sizeBytes: 1_500_000, totalSizeBytes: 1_500_000, assets: [rootAsset] } as AssetPath;
  const extrasPath = { id: 'extras', name: 'extras', path: '/media/raw/anime/Attack on Titan/extras', relativePath: 'extras', displayPath: 'extras', isLogicalGroupRoot: false, fileCount: 1, assetCount: 1, sizeBytes: 1_500_000, totalSizeBytes: 1_500_000, assets: [extraAsset] } as AssetPath;
  const logicalGroup = { id: '/media/raw/anime/Attack on Titan', name: 'Attack on Titan', path: '/media/raw/anime/Attack on Titan', relativePath: 'Attack on Titan', fileCount: 2, assetCount: 2, pathCount: 2, sizeBytes: 3_000_000, totalSizeBytes: 3_000_000, assetPaths: [rootPath, extrasPath] };
  const sourceGroup = { id: 7, name: 'Anime', relativePath: 'anime', sourcePath: '/media/raw/anime', fileCount: 2, assetCount: 2, titleCount: 1, pathCount: 2, sizeBytes: 3_000_000, totalSizeBytes: 3_000_000, logicalGroups: [logicalGroup] } as AssetSourceGroup;
  const libraryAsset = asset(20, 'Library Episode.mkv');
  return {
    ...inventory([libraryAsset]),
    sourceGroups: [sourceGroup], raw: [rootAsset, extraAsset], unprocessed: [rootAsset, extraAsset],
    rawGroups: [], unprocessedGroups: [],
    counts: { raw: 2, library: 1, processing: 0, needsReview: 0, validationReview: 0, publicationBlocked: 0, converted: 0, originalsArchived: 0, missing: 0, historicalPaths: 0 },
  } as unknown as AssetInventory;
}

function persistedSnapshot(assetPath: string): ScanResult {
  return {
    id: 1, path: assetPath, fileName: assetPath.split('/').pop() ?? '', container: 'matroska', sizeBytes: 1_500_000,
    duration: 1420, bitrate: 4_000_000, videoCodec: 'hevc', width: 1920, height: 1080, hdr: false,
    audioTracks: 1, subtitleTracks: 2, chapters: 0, videoStreams: [],
    audioStreams: [{ index: 1, type: 'audio', codec: 'aac', codecLong: 'AAC', profile: '', language: 'ja', title: '', duration: 1420, bitrate: 192000, default: true, forced: false, comment: false, hearingImpaired: false, channels: 2 }],
    subtitleStreams: [
      { index: 2, type: 'subtitle', codec: 'ass', codecLong: 'ASS', profile: '', language: 'ja', title: '', duration: 1420, bitrate: 0, default: true, forced: false, comment: false, hearingImpaired: false },
      { index: 3, type: 'subtitle', codec: 'srt', codecLong: 'SubRip', profile: '', language: 'en', title: '', duration: 1420, bitrate: 0, default: false, forced: false, comment: false, hearingImpaired: false },
    ],
    attachmentStreams: [], attachmentInventoryAvailable: true, compatibilityAnalysis: {},
  } as unknown as ScanResult;
}

const library: Library = { id: 1, name: 'Anime', sourcePath: '/media/raw/anime', destinationPath: '/media/library/anime', type: 'anime', validationRules: {}, createdAt: '', updatedAt: '' };
const pathAssignment: ProfileAssignment = { id: 1, targetType: 'path', targetPath: path, mediaType: 'video', selection: 'profile', videoProfileId: 1, createdAt: '', updatedAt: '' };
const profile = { id: 1, name: 'Library HEVC', scope: 'path', disabled: false, workerConfig: {} };

function SearchProbe() {
  return <output data-testid="asset-query">{useLocation().search}</output>;
}

function renderLibraryPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <MemoryRouter initialEntries={['/assets?location=library&search=Episode']}>
      <QueryClientProvider client={queryClient}>
        <AssetsPage />
        <SearchProbe />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return { ...view, queryClient };
}

function renderRawPage(initialUrl = '/assets?location=raw') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <MemoryRouter initialEntries={[initialUrl]}>
      <QueryClientProvider client={queryClient}>
        <AssetsPage />
        <SearchProbe />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return { ...view, queryClient };
}

describe('Library Assets workspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const first = asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv', ['TV']);
    const second = asset(2, 'Episode 02.mkv');
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(inventory([first, second]));
    vi.mocked(api.profiles).mockResolvedValue([profile] as never);
    vi.mocked(api.libraries).mockResolvedValue([library]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([pathAssignment]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockImplementation(async (assetPath) => ({ found: true, snapshot: persistedSnapshot(assetPath), status: 'current', requiresAnalysis: false, staleComponents: [] }));
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
  });

  it('renders the six scan columns, compact counts, and a path-filtered accessible hierarchy', async () => {
    const user = userEvent.setup();
    renderLibraryPage();

    expect(await screen.findByRole('columnheader', { name: 'Name' })).toBeTruthy();
    for (const name of ['Status', 'Profile', 'Subtitles', 'Duration', 'Modified']) {
      expect(screen.getByRole('columnheader', { name })).toBeTruthy();
    }
    expect(screen.getByLabelText('Total Assets: 2').textContent).toBe('2');
    expect(screen.getByLabelText('Processing: 1').textContent).toBe('1');
    expect(screen.getByLabelText('Completed: 3').textContent).toBe('3');
    expect(screen.getByLabelText('Needs Review: 0').textContent).toBe('0');
    expect(screen.queryByRole('img')).toBeNull();
    expect(screen.getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').getAttribute('aria-label')).toBe('A Very Long Episode Filename That Should Stay Inside Its Column.mkv');
    expect(api.latestSnapshot).not.toHaveBeenCalled();

    await user.click(screen.getByRole('treeitem', { name: /Anime/ }));
    await user.click(screen.getByRole('button', { name: 'Expand Anime' }));
    await user.click(screen.getByRole('treeitem', { name: /Series/ }));
    expect(screen.getByTestId('asset-query').textContent).toBe('?location=library&search=Episode');
    expect(screen.getByText(/2 assets/)).toBeTruthy();
    expect(api.latestSnapshot).not.toHaveBeenCalled();
  });

  it('selects an asset, loads persisted detail only for it, and exposes existing actions', async () => {
    const user = userEvent.setup();
    const { queryClient } = renderLibraryPage();

    const row = await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv');
    await user.click(row.closest('tr')!);

    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByText('Profile · Library HEVC')).toBeTruthy();
    expect(within(details).getByText(/Subtitles · 2 tracks/)).toBeTruthy();
    expect(within(details).getByText('Video · hevc')).toBeTruthy();
    expect(within(details).getByText('Resolution · 1920 × 1080')).toBeTruthy();
    expect(within(details).getByText(/Audio · aac 2ch ja/)).toBeTruthy();
    expect(within(details).getByText('Metadata · Anime, TV')).toBeTruthy();
    expect((within(details).getByRole('img') as HTMLImageElement).getAttribute('src')).toContain('/api/assets/preview?path=');
    const selectedRow = row.closest('tr')!;
    expect(within(selectedRow).getByText('Unverified')).toBeTruthy();
    expect(within(selectedRow).getByText('Library HEVC')).toBeTruthy();
    expect(within(selectedRow).getByText('2 tracks')).toBeTruthy();
    expect(within(selectedRow).getByText('23m 40s')).toBeTruthy();
    expect(within(selectedRow).getByText(/Sep 30, 2026/)).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledTimes(1));
    expect(api.latestSnapshot).toHaveBeenCalledWith(`${path}/A Very Long Episode Filename That Should Stay Inside Its Column.mkv`);

    const assetsCallsBeforeRefresh = vi.mocked(api.assets).mock.calls.length;
    await act(async () => { await queryClient.invalidateQueries({ queryKey: ['assets'] }); });
    await waitFor(() => expect(vi.mocked(api.assets).mock.calls.length).toBeGreaterThan(assetsCallsBeforeRefresh));
    expect(screen.getByTestId('selected-asset-detail').getAttribute('aria-label')).toContain('A Very Long Episode Filename');

    await user.click(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for A Very Long Episode Filename That Should Stay Inside Its Column.mkv' }));
    expect(await screen.findByRole('menuitem', { name: 'Asset Info' })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: 'Queue replacement' })).toBeTruthy();
    await user.keyboard('{Escape}');

    const refreshedDetails = screen.getByTestId('selected-asset-detail');
    expect(within(refreshedDetails).getAllByRole('button', { name: /Play preview/ })).toHaveLength(1);
    expect(within(refreshedDetails).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(refreshedDetails).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    await user.click(within(refreshedDetails).getByRole('button', { name: 'Assign Profile' }));
    const assignmentDialog = await screen.findByRole('dialog', { name: /Assign Profile/ });
    expect(assignmentDialog).toBeTruthy();
    await user.click(within(assignmentDialog).getByRole('button', { name: 'Done' }));
    await user.click(await screen.findByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    expect(api.testEncodes).toHaveBeenCalledWith(`${path}/A Very Long Episode Filename That Should Stay Inside Its Column.mkv`);
  });

  it('keeps path Advisor direct and Queue/configuration actions reachable', async () => {
    const user = userEvent.setup();
    renderLibraryPage();

    await screen.findByRole('columnheader', { name: 'Name' });
    await user.click(screen.getByRole('treeitem', { name: /Anime/ }));
    await user.click(screen.getByRole('button', { name: 'Expand Anime' }));
    await user.click(screen.getByRole('treeitem', { name: /Series/ }));
    await user.click(screen.getByRole('button', { name: 'Expand Series' }));
    await user.click(screen.getByRole('treeitem', { name: /Season 01/ }));

    expect(await screen.findByRole('button', { name: 'Advisor' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Queue' })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: `More actions for ${path}` }));
    expect(await screen.findByRole('menuitem', { name: 'Path configuration and more actions' })).toBeTruthy();
  });
});

describe('RAW Assets workspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(rawWorkspaceInventory());
    vi.mocked(api.profiles).mockResolvedValue([profile] as never);
    vi.mocked(api.libraries).mockResolvedValue([library]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([pathAssignment]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockImplementation(async (assetPath) => ({ found: true, snapshot: persistedSnapshot(assetPath), status: 'current', requiresAnalysis: false, staleComponents: [] }));
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((id) => [String(id), {
        assetPath: `/asset/${id}`, video: { selection: 'inherit' }, audio: { selection: 'inherit' },
        tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' },
      }])),
      missingAssetIds: [],
    }));
  });

  it('exposes only RAW and Library and switches URL state without leaking context filters', async () => {
    const user = userEvent.setup();
    renderRawPage('/assets?location=raw&search=Attack&sourceGroup=7&logicalGroup=attack&path=%2Fmedia%2Fraw%2Fanime%2FAttack&provenance=converted&validation=failed&workflow=queued&attention=needs_review&mediaArea=TV');

    const rawTab = await screen.findByRole('tab', { name: 'RAW' });
    const libraryTab = screen.getByRole('tab', { name: 'Library' });
    expect(rawTab.getAttribute('aria-selected')).toBe('true');
    expect(libraryTab.getAttribute('aria-selected')).toBe('false');
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent?.trim())).toEqual(['RAW', 'Library']);

    await user.click(libraryTab);
    let query = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(query.get('location')).toBe('library');
    expect(query.get('search')).toBe('Attack');
    expect(query.get('provenance')).toBe('converted');
    expect(query.get('validation')).toBe('failed');
    expect(query.get('workflow')).toBe('queued');
    expect(query.get('attention')).toBe('needs_review');
    expect(query.get('mediaArea')).toBe('TV');
    for (const key of ['sourceGroup', 'logicalGroup', 'path', 'tab']) expect(query.has(key)).toBe(false);

    await user.click(await screen.findByRole('tab', { name: 'RAW' }));
    query = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(query.get('location')).toBe('raw');
    expect(query.get('search')).toBe('Attack');
    for (const key of ['provenance', 'validation', 'workflow', 'attention', 'mediaArea', 'sourceGroup', 'logicalGroup', 'path']) expect(query.has(key)).toBe(false);
  });

  it('renders Source Group, Logical Group, and Path navigation with compact RAW actions and one selected preview', async () => {
    const user = userEvent.setup();
    renderRawPage();

    await screen.findByRole('treeitem', { name: 'Anime' });
    const navigation = screen.getByRole('navigation', { name: 'RAW source hierarchy' });
    const tree = within(navigation).getByRole('tree', { name: 'RAW sources' });
    expect(within(tree).getByRole('treeitem', { name: 'Anime' })).toBeTruthy();
    const title = within(tree).getByRole('treeitem', { name: 'Attack on Titan' });
    expect(title.getAttribute('aria-selected')).toBe('true');
    expect(screen.getByRole('button', { name: 'Run Advisor' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Configure title' })).toBeTruthy();

    (title as HTMLElement).focus();
    await user.keyboard('{ArrowRight}');
    expect(title.getAttribute('aria-expanded')).toBe('true');
    expect(within(tree).getByRole('treeitem', { name: 'Root' })).toBeTruthy();
    const extras = within(tree).getByRole('treeitem', { name: 'extras' });
    await user.click(extras);
    expect(extras.getAttribute('aria-selected')).toBe('true');
    const rawTable = await screen.findByRole('table', { name: 'RAW assets' });
    expect(within(rawTable).getByText('Trailer.mkv')).toBeTruthy();
    expect(within(rawTable).queryByText('S01E01.mkv')).toBeNull();
    expect(screen.getByRole('button', { name: 'Queue' })).toBeTruthy();
    const pathActions = screen.getByRole('table', { name: /Actions for .*extras/ });
    expect(within(pathActions).getByRole('button', { name: 'Configure' })).toBeTruthy();

    expect(within(rawTable).getByRole('columnheader', { name: 'Name' })).toBeTruthy();
    expect(within(rawTable).getByRole('columnheader', { name: 'Status' })).toBeTruthy();
    expect(within(rawTable).queryByRole('img')).toBeNull();
    const details = screen.getByTestId('selected-asset-detail');
    expect(within(details).getByRole('button', { name: /Play preview/ })).toBeTruthy();
    expect(within(details).getAllByRole('button', { name: /Play preview/ })).toHaveLength(1);
    expect(within(details).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    expect(within(details).getByText('Profile · Inherited')).toBeTruthy();

    await user.click(within(details).getByRole('button', { name: 'More actions for Trailer.mkv' }));
    expect(await screen.findByRole('menuitem', { name: 'Asset Info' })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: 'Queue asset' })).toBeTruthy();
  });
});
