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
import { AssetsPage, ReportsPage } from './AssetsPage';
import { assetLabHref } from '../components/assetsActionRoutes';

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
    technical: { videoCodec: 'hevc', encoder: 'hevc_videotoolbox', width: 1920, height: 1080, duration: 1420, bitrate: 4_000_000, hdr: false },
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

function assetsTree() {
  return screen.getByRole('tree', { name: 'Asset Library' });
}

function treeItem(name: string, level?: number) {
  const items = within(assetsTree()).getAllByRole('treeitem');
  const item = items.find((candidate) => candidate.textContent?.includes(name)
    && (level === undefined || candidate.getAttribute('aria-level') === String(level)));
  if (!item) throw new Error(`Missing Assets tree item ${name}${level ? ` at level ${level}` : ''}`);
  return item;
}

function treeExpander(name: string, level?: number) {
  const buttons = within(assetsTree()).getAllByRole('button', { name: `Expand ${name}` });
  const button = buttons.find((candidate) => level === undefined
    || candidate.closest('[role="treeitem"]')?.getAttribute('aria-level') === String(level));
  if (!button) throw new Error(`Missing Assets tree expander ${name}${level ? ` at level ${level}` : ''}`);
  return button;
}

function childTreeItem(parentName: string, name: string) {
  const parent = treeItem(parentName, 1);
  const group = parent.parentElement?.querySelector<HTMLElement>('[role="group"]');
  if (!group) throw new Error(`Missing child tree group for ${parentName}`);
  return within(group).getByRole('treeitem', { name });
}

function libraryTreeItem(name: string) {
  return childTreeItem('Libraries', name);
}

function rawTreeItem(name: string) {
  return childTreeItem('RAW', name);
}

function renderLibraryPage(queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
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

function renderReportsPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter initialEntries={['/reports']}>
      <QueryClientProvider client={queryClient}>
        <ReportsPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

function inventoryWithReports(): AssetInventory {
  const missingAsset = { ...asset(99, 'Missing Episode.mkv'), status: 'missing', missing: true };
  return {
    ...rawWorkspaceInventory(),
    missing: [missingAsset],
    reports: {
      unprocessedFiles: 2,
      libraryFiles: 1,
      unverifiedFiles: 1,
      acceptedFiles: 0,
      convertedFiles: 0,
      convertedSpaceSavedBytes: 0,
      convertedComparedFiles: 0,
      convertedOriginalBytes: 0,
      convertedOutputBytes: 0,
      archiveFiles: 0,
      archiveBytes: 0,
      expiredArchive: 0,
      missingFiles: 1,
      missingActionable: 1,
      missingHistorical: 0,
    },
  } as unknown as AssetInventory;
}

describe('Reports page content', () => {
  it('uses persisted inventory reports and keeps missing-record actions reachable', async () => {
    vi.clearAllMocks();
    vi.mocked(api.assets).mockResolvedValue(inventoryWithReports());
    renderReportsPage();

    expect((await screen.findAllByText('Unprocessed')).length).toBeGreaterThan(0);
    expect(screen.getByText('Missing assets requiring attention')).toBeTruthy();
    const missingTable = screen.getByRole('table');
    expect(within(missingTable).getByText('/media/library/anime/Series/Season 01/Missing Episode.mkv')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Remove missing asset Missing Episode.mkv' })).toBeTruthy();
    expect(api.assets).toHaveBeenCalledTimes(1);
  });
});

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

  it('renders Library technical columns, compact counts, and a path-filtered accessible hierarchy', async () => {
    const user = userEvent.setup();
    renderLibraryPage();

    expect(await screen.findByRole('columnheader', { name: 'Asset' })).toBeTruthy();
    const pageLayout = screen.getByTestId('assets-page-layout');
    const topbar = screen.getByTestId('assets-topbar');
    const pageBody = screen.getByTestId('assets-page-body');
    expect(pageLayout.firstElementChild).toBe(topbar);
    expect(topbar.nextElementSibling).toBe(pageBody);
    expect(within(topbar).getByRole('heading', { name: 'Assets' })).toBeTruthy();
    expect(within(topbar).getByRole('textbox', { name: 'Search assets, series, movies' })).toBeTruthy();
    expect(topbar.contains(screen.getByRole('navigation', { name: 'Asset Library' }))).toBe(false);
    const hierarchyTitle = within(screen.getByRole('navigation', { name: 'Asset Library' })).getByText('Asset Library');
    expect(hierarchyTitle).toBeTruthy();
    const contentColumn = screen.getByTestId('assets-content-column');
    expect(contentColumn.contains(screen.getByRole('region', { name: 'Asset summary' }))).toBe(true);
    expect(pageBody.contains(screen.getByTestId('assets-inventory-controls'))).toBe(true);
    expect(pageBody.contains(screen.getByTestId('assets-list-scroll'))).toBe(true);
    for (const name of ['Advisor', 'Status', 'Codec', 'Resolution', 'Encoder', 'Duration', 'Bitrate', 'HDR/SDR', 'Size', 'Modified', 'Actions']) {
      expect(screen.getByRole('columnheader', { name })).toBeTruthy();
    }
    expect(screen.getAllByText('Not evaluated')).toHaveLength(2);
    expect(screen.getByLabelText('Total Assets: 2').textContent).toBe('2');
    expect(screen.getByLabelText('Processing: 1').textContent).toBe('1');
    expect(screen.getByLabelText('Completed: 3').textContent).toBe('3');
    expect(screen.getByLabelText('Needs Review: 0').textContent).toBe('0');
    expect(screen.queryByRole('img')).toBeNull();
    expect(libraryTreeItem('Anime').getAttribute('aria-selected')).toBe('false');
    expect(screen.queryByRole('tab', { name: 'RAW' })).toBeNull();
    expect(screen.queryByRole('tab', { name: 'Library' })).toBeNull();
    const firstRow = screen.getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').closest('tr');
    expect(firstRow).toBeTruthy();
    expect(firstRow?.getAttribute('data-row-surface')).toBe('compact-asset');
    expect(within(firstRow!).queryByText(/Series\/Season 01/)).toBeNull();
    expect(within(firstRow!).queryByText('/media/library/anime/Series/Season 01')).toBeNull();
    expect(api.latestSnapshot).not.toHaveBeenCalled();
    const controls = screen.getByTestId('assets-inventory-controls');
    expect(within(controls).getByRole('button', { name: 'Sync' })).toBeTruthy();
    expect(within(controls).getByRole('button', { name: /Filter assets/ })).toBeTruthy();
    expect((within(controls).getByRole('button', { name: 'Sort assets (unavailable)' }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(within(controls).getByRole('button', { name: /Filter assets/ }));
    expect(await screen.findByTestId('assets-filter-popover-content')).toBeTruthy();
    await user.keyboard('{Escape}');

    await user.click(libraryTreeItem('Anime'));
    expect(libraryTreeItem('Anime').getAttribute('aria-selected')).toBe('true');
    await user.click(treeExpander('Anime', 2));
    await user.click(treeItem('Series', 3));
    expect(treeItem('Series', 3).getAttribute('aria-selected')).toBe('true');
    const libraryQuery = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(libraryQuery.get('location')).toBe('library');
    expect(libraryQuery.get('search')).toBe('Episode');
    expect(libraryQuery.get('libraryId')).toBe('1');
    expect(screen.getByText(/2 assets/)).toBeTruthy();
    expect(screen.getByRole('navigation', { name: 'Library location' }).textContent).toContain('Series');
    expect(api.latestSnapshot).not.toHaveBeenCalled();
  });

  it('keeps historical Legacy Analysis Decisions out of the main Assets browsing workspace', async () => {
    vi.mocked(api.settings).mockResolvedValue([{ key: 'analysisRecords', value: { records: [{ id: 'legacy-1', assetName: 'Old episode.mkv', decision: 'keep' }] } }] as never);
    renderLibraryPage();

    await screen.findByRole('columnheader', { name: 'Asset' });
    expect(screen.queryByTestId('legacy-analysis-settings')).toBeNull();
    expect(screen.getByRole('link', { name: 'Open Assets Settings' }).getAttribute('href')).toBe('/settings?section=assets');
  });

  it('selects an asset, loads persisted detail only for it, and exposes existing actions', async () => {
    const user = userEvent.setup();
    const { queryClient } = renderLibraryPage();

    const row = await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv');
    await user.click(row.closest('tr')!);

    const list = screen.getByRole('region', { name: 'Library asset list' });
    const table = within(list).getByRole('table', { name: 'Library assets' });
    const details = await screen.findByTestId('selected-asset-detail');
    expect(getComputedStyle(list).overflowY).toBe('auto');
    expect(list.contains(table)).toBe(true);
    expect(list.contains(details)).toBe(false);
    expect(list.contains(screen.getByLabelText('Total Assets: 2'))).toBe(false);
    expect(screen.getByTestId('assets-topbar').contains(screen.getByLabelText('Search assets, series, movies'))).toBe(true);
    expect(within(details).getByText('Perfil')).toBeTruthy();
    expect(within(details).getByText('Library HEVC')).toBeTruthy();
    expect(within(details).getByText('Subtítulos')).toBeTruthy();
    expect(within(details).getByText(/2 tracks/)).toBeTruthy();
    expect(within(details).getByText('Video codec')).toBeTruthy();
    expect(within(details).getAllByText('hevc')).toHaveLength(2);
    expect(within(details).getAllByText('Resolution')).toHaveLength(2);
    expect(within(details).getAllByText('1920 × 1080')).toHaveLength(2);
    expect(within(details).getByText('Audio')).toBeTruthy();
    expect(within(details).getByText('aac 2ch ja')).toBeTruthy();
    expect(within(details).getByText('Category / Library')).toBeTruthy();
    expect(within(details).getByText('Anime')).toBeTruthy();
    expect(within(details).getByRole('img', { name: /MVForge artwork placeholder/ }).getAttribute('data-artwork-source')).toBe('placeholder');
    const selectedRow = row.closest('tr')!;
    expect(within(selectedRow).getByText('Unverified')).toBeTruthy();
    expect(within(selectedRow).getByText('hevc')).toBeTruthy();
    expect(within(selectedRow).getByText('hevc_videotoolbox')).toBeTruthy();
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
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const evaluateAdvisor = vi.spyOn(api, 'evaluateAdvisor').mockResolvedValue({ score: 88, recommendation: 'maybe' } as never);
    renderLibraryPage(queryClient);

    await screen.findByRole('columnheader', { name: 'Asset' });
    await user.click(libraryTreeItem('Anime'));
    await user.click(treeExpander('Anime', 2));
    await user.click(treeItem('Series', 3));
    await user.click(treeExpander('Series', 3));
    await user.click(treeItem('Season 01', 4));

    const toolbar = screen.getByTestId('assets-path-toolbar');
    expect(within(toolbar).getByRole('button', { name: 'Advisor' })).toBeTruthy();
    expect(within(toolbar).getByRole('button', { name: 'Queue / Preflight' })).toBeTruthy();
    expect(within(toolbar).getByRole('button', { name: 'Move path' })).toBeTruthy();
    expect(within(toolbar).getByRole('button', { name: 'Rename path' })).toBeTruthy();
    expect(within(toolbar).getByRole('navigation', { name: 'Library location' })).toBeTruthy();
    expect(screen.queryByRole('table', { name: `Actions for ${path}` })).toBeNull();
    expect(evaluateAdvisor).not.toHaveBeenCalled();

    await user.click(within(toolbar).getByRole('button', { name: 'Advisor' }));
    await waitFor(() => expect(evaluateAdvisor).toHaveBeenCalledTimes(2));
    expect(queryClient.getQueryData(['advisor', 'asset-row', `${path}/A Very Long Episode Filename That Should Stay Inside Its Column.mkv`, 1])).toEqual({ score: 88, recommendation: 'maybe' });
    expect(queryClient.getQueryData(['advisor', 'asset-row', `${path}/Episode 02.mkv`, 1])).toEqual({ score: 88, recommendation: 'maybe' });
    evaluateAdvisor.mockRestore();
  });

  it('shows a cached canonical Advisor score without evaluating rows and keeps row overflow asset-scoped', async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const firstPath = `${path}/A Very Long Episode Filename That Should Stay Inside Its Column.mkv`;
    queryClient.setQueryData(['advisor', 'asset-row', firstPath, 1], { score: 87, summary: 'Cached canonical Advisor result.' });
    const evaluateAdvisor = vi.spyOn(api, 'evaluateAdvisor').mockResolvedValue({ score: 90 } as never);
    renderLibraryPage(queryClient);

    const table = await screen.findByRole('table', { name: 'Library assets' });
    const cachedRow = screen.getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').closest('tr');
    expect(cachedRow).toBeTruthy();
    expect(within(cachedRow!).getByText('87')).toBeTruthy();
    expect(within(cachedRow!).getByTestId('advisor-score-indicator')).toBeTruthy();
    expect(within(table).getAllByText('Not evaluated')).toHaveLength(1);
    const unevaluatedRow = screen.getByText('Episode 02.mkv').closest('tr')!;
    expect(within(unevaluatedRow).queryByTestId('advisor-score-indicator')).toBeNull();
    expect(evaluateAdvisor).not.toHaveBeenCalled();

    const rowAfterAssignment = within(table).getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').closest('tr')!;
    await user.click(within(rowAfterAssignment).getByRole('button', { name: 'More actions for A Very Long Episode Filename That Should Stay Inside Its Column.mkv' }));
    for (const name of ['Asset Info', 'Advisor', 'Open in Lab', 'Assign Profile', 'Test Encode', 'Queue replacement']) {
      expect(await screen.findByRole('menuitem', { name })).toBeTruthy();
    }
    expect(assetLabHref(firstPath)).toBe(`/profile-lab?assetPath=${encodeURIComponent(firstPath)}`);
    await user.click(screen.getByRole('menuitem', { name: 'Assign Profile' }));
    expect(await screen.findByRole('dialog', { name: /Assign Profile/ })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Assign Profile/ })).toBeNull());
    const currentTable = screen.getByRole('table', { name: 'Library assets' });
    const rowBeforeTestEncode = within(currentTable).getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').closest('tr')!;
    await user.click(within(rowBeforeTestEncode).getByRole('button', { name: 'More actions for A Very Long Episode Filename That Should Stay Inside Its Column.mkv' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    expect(evaluateAdvisor).not.toHaveBeenCalled();
    evaluateAdvisor.mockRestore();
  });

  it('shows unavailable only after an explicit failed Advisor request and distinguishes confidence Off', async () => {
    const user = userEvent.setup();
    const evaluateAdvisor = vi.spyOn(api, 'evaluateAdvisor').mockRejectedValue(new Error('Advisor service unavailable'));
    renderLibraryPage();

    const table = await screen.findByRole('table', { name: 'Library assets' });
    const row = within(table).getByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv').closest('tr')!;
    expect(within(row).getByLabelText('Advisor not evaluated')).toBeTruthy();
    expect(evaluateAdvisor).not.toHaveBeenCalled();
    await user.click(within(row).getByRole('button', { name: 'More actions for A Very Long Episode Filename That Should Stay Inside Its Column.mkv' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Advisor' }));
    await waitFor(() => expect(within(table).getByLabelText('Advisor unavailable')).toBeTruthy());
    expect(evaluateAdvisor).toHaveBeenCalledTimes(1);
    evaluateAdvisor.mockRestore();

    cleanup();
    vi.mocked(api.settings).mockResolvedValue([{ key: 'advisorAutomationPathOverrides', value: { disabledPaths: [path] } }] as never);
    const offEvaluation = vi.spyOn(api, 'evaluateAdvisor');
    renderLibraryPage();
    const offTable = await screen.findByRole('table', { name: 'Library assets' });
    expect(within(offTable).getAllByLabelText('Advisor off')).toHaveLength(2);
    expect(offEvaluation).not.toHaveBeenCalled();
    offEvaluation.mockRestore();
  });

  it('keeps lifecycle destructive actions capability-gated in the compact row menu', async () => {
    const user = userEvent.setup();
    const converted = {
      ...asset(21, 'Converted Episode.mkv'),
      status: 'converted',
      lifecycle: {
        location: 'library',
        lifecycle: 'published',
        provenance: 'converted',
        validation: 'passed',
        publication: 'published',
        original: 'archived',
        actions: {
          lab: { allowed: false, reason: 'Lab unavailable' },
          queue: { allowed: false, reason: 'Queue unavailable' },
          deleteArchivedOriginal: { allowed: false, reason: 'Original cannot be restored' },
          refreshAnalysis: { allowed: false, reason: 'Analysis unavailable' },
        },
      },
    } as Asset;
    vi.mocked(api.assets).mockResolvedValue(inventory([converted]));
    renderLibraryPage();

    const row = await screen.findByText('Converted Episode.mkv');
    await user.click(within(row.closest('tr')!).getByRole('button', { name: 'More actions for Converted Episode.mkv' }));
    const menu = await screen.findByRole('menu');
    for (const name of ['Open in Lab', 'Queue asset', 'Safely delete and restore original']) {
      expect(within(menu).getByRole('menuitem', { name }).getAttribute('aria-disabled')).toBe('true');
    }
    expect(within(menu).getByRole('separator')).toBeTruthy();
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

  it('uses one accessible Assets tree to switch RAW and Library while preserving common search', async () => {
    const user = userEvent.setup();
    vi.mocked(api.libraries).mockResolvedValue([library, { ...library, id: 2, name: 'Movies', sourcePath: '/media/raw/movies', destinationPath: '/media/library/movies', type: 'movies' }]);
    renderRawPage('/assets?location=raw&search=Attack&sourceGroup=7&logicalGroup=attack&path=%2Fmedia%2Fraw%2Fanime%2FAttack');

    const tree = assetsTree();
    expect(screen.queryByRole('tab', { name: 'RAW' })).toBeNull();
    expect(screen.queryByRole('tab', { name: 'Library' })).toBeNull();
    expect(screen.getAllByRole('tree', { name: 'Asset Library' })).toHaveLength(1);
    await screen.findByRole('treeitem', { name: 'Movies' });
    expect(treeItem('RAW', 1).getAttribute('aria-label')).toBe('RAW');
    expect(rawTreeItem('Anime')).toBeTruthy();
    expect(libraryTreeItem('Movies')).toBeTruthy();
    expect(within(tree).queryByRole('treeitem', { name: 'Converted' })).toBeNull();
    expect(within(tree).queryByRole('treeitem', { name: 'Archive' })).toBeNull();
    expect(within(tree).queryByRole('treeitem', { name: 'Reports' })).toBeNull();

    await user.click(libraryTreeItem('Anime'));
    let query = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(query.get('location')).toBe('library');
    expect(query.get('search')).toBe('Attack');
    expect(query.get('libraryId')).toBe('1');
    for (const key of ['sourceGroup', 'logicalGroup', 'path', 'rawScope', 'tab']) expect(query.has(key)).toBe(false);

    await user.click(treeItem('RAW', 1));
    expect(treeItem('RAW', 1).getAttribute('aria-selected')).toBe('true');
    query = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(query.get('location')).toBe('raw');
    expect(query.get('search')).toBe('Attack');
    expect(query.get('rawScope')).toBe('all');
    for (const key of ['libraryId', 'libraryPath', 'sourceGroup', 'logicalGroup', 'path', 'tab']) expect(query.has(key)).toBe(false);
  });

  it('clears Library-only state when selecting RAW and retains safe search', async () => {
    const user = userEvent.setup();
    renderRawPage('/assets?location=library&search=Attack&libraryId=1&libraryPath=%2Fmedia%2Flibrary%2Fanime&provenance=converted&mediaArea=series&validation=failed&original=available&workflow=queued&attention=needs_review&evidence=scan');
    await user.click(treeItem('RAW', 1));
    const query = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(query.get('location')).toBe('raw');
    expect(query.get('search')).toBe('Attack');
    for (const key of ['libraryId', 'libraryPath', 'provenance', 'mediaArea', 'validation', 'original', 'workflow', 'attention', 'evidence']) expect(query.has(key)).toBe(false);
  });

  it('renders Source Group, Logical Group, and Path navigation with compact RAW actions and one selected preview', async () => {
    const user = userEvent.setup();
    renderRawPage();

    const title = await screen.findByRole('treeitem', { name: 'Attack on Titan' });
    expect(title.getAttribute('aria-selected')).toBe('true');
    expect(treeItem('RAW', 1).getAttribute('aria-selected')).toBe('false');
    const rawBreadcrumbs = screen.getByRole('navigation', { name: 'RAW title location' }).textContent ?? '';
    expect(rawBreadcrumbs).toContain('RAW');
    expect(rawBreadcrumbs).toContain('Anime');
    expect(rawBreadcrumbs).toContain('Attack on Titan');
    expect(await screen.findByTestId('selected-asset-detail')).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledTimes(1));
    expect(api.latestSnapshot).toHaveBeenCalledWith('/media/raw/anime/Attack on Titan/S01E01.mkv');
    expect(screen.getByRole('button', { name: 'Advisor' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Configure Path' })).toBeTruthy();

    const sourceGroupNode = rawTreeItem('Anime');
    await user.click(sourceGroupNode);
    expect(sourceGroupNode.getAttribute('aria-selected')).toBe('true');
    let sourceQuery = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(sourceQuery.get('sourceGroup')).toBe('7');
    expect(sourceQuery.has('logicalGroup')).toBe(false);
    expect(sourceQuery.has('path')).toBe(false);
    const sourceTable = screen.getByRole('table', { name: 'RAW assets' });
    expect(within(sourceTable).getByText('S01E01.mkv')).toBeTruthy();
    expect(within(sourceTable).getByText('Trailer.mkv')).toBeTruthy();

    await user.click(title);
    sourceQuery = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(sourceQuery.get('sourceGroup')).toBe('7');
    expect(sourceQuery.get('logicalGroup')).toBe('/media/raw/anime/Attack on Titan');
    expect(sourceQuery.has('path')).toBe(false);
    (title as HTMLElement).focus();
    await user.keyboard('{ArrowRight}');
    expect(title.getAttribute('aria-expanded')).toBe('true');
    expect(treeItem('Root', 4)).toBeTruthy();
    const extras = treeItem('extras', 4);
    await user.click(extras);
    expect(extras.getAttribute('aria-selected')).toBe('true');
    const rawQuery = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(rawQuery.get('location')).toBe('raw');
    expect(rawQuery.get('sourceGroup')).toBe('7');
    expect(rawQuery.get('logicalGroup')).toBe('/media/raw/anime/Attack on Titan');
    expect(rawQuery.get('path')).toBe('/media/raw/anime/Attack on Titan/extras');
    expect(screen.getByRole('navigation', { name: 'RAW location' }).textContent).toContain('extras');
    const rawTable = await screen.findByRole('table', { name: 'RAW assets' });
    expect(within(rawTable).getByText('Trailer.mkv')).toBeTruthy();
    expect(within(rawTable).queryByText('S01E01.mkv')).toBeNull();
    const pathToolbar = screen.getByTestId('assets-path-toolbar');
    for (const name of ['Scan', 'Advisor', 'Queue / Preflight', 'Configure Path']) {
      expect(within(pathToolbar).getByRole('button', { name })).toBeTruthy();
    }
    expect(within(pathToolbar).queryByRole('button', { name: 'Rename path' })).toBeNull();
    expect(screen.queryByRole('table', { name: /Actions for .*extras/ })).toBeNull();

    for (const name of ['Asset', 'Advisor', 'Status', 'Source / Codec', 'Resolution', 'Duration', 'Bitrate', 'Size', 'Modified', 'Actions']) {
      expect(within(rawTable).getByRole('columnheader', { name })).toBeTruthy();
    }
    expect(within(rawTable).queryByRole('img')).toBeNull();
    const rawAssetRow = within(rawTable).getByText('Trailer.mkv').closest('tr');
    expect(rawAssetRow).toBeTruthy();
    expect(within(rawAssetRow!).queryByText(/Attack on Titan|extras/)).toBeNull();
    const details = screen.getByTestId('selected-asset-detail');
    const rawList = screen.getByRole('region', { name: 'RAW asset list' });
    expect(getComputedStyle(rawList).overflowY).toBe('auto');
    expect(rawList.contains(rawTable)).toBe(true);
    expect(rawList.contains(details)).toBe(false);
    expect(within(details).getByRole('button', { name: /Play preview/ })).toBeTruthy();
    expect(within(details).getAllByRole('button', { name: /Play preview/ })).toHaveLength(1);
    expect(within(details).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    expect(within(details).getByText('Perfil')).toBeTruthy();
    expect(within(details).getByText('Inherited')).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledTimes(2));
    expect(api.latestSnapshot).toHaveBeenLastCalledWith('/media/raw/anime/Attack on Titan/extras/Trailer.mkv');

    await user.click(within(pathToolbar).getByRole('button', { name: 'Scan' }));
    expect(await screen.findByRole('dialog', { name: /Snapshots/ })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Snapshots/ })).toBeNull());

    const currentRawTable = screen.getByRole('table', { name: 'RAW assets' });
    const currentRawRow = within(currentRawTable).getByText('Trailer.mkv').closest('tr')!;
    await user.click(within(currentRawRow).getByRole('button', { name: 'More actions for Trailer.mkv' }));
    for (const name of ['Scan / Analyze Asset', 'Advisor', 'Open in Lab', 'Assign Profile', 'Test Encode', 'Asset Info']) {
      expect(await screen.findByRole('menuitem', { name })).toBeTruthy();
    }
    expect(screen.getByRole('menuitem', { name: 'Queue asset' })).toBeTruthy();
    await user.keyboard('{Escape}');
    await user.click(treeItem('RAW', 1));
    const rawRootQuery = new URLSearchParams(screen.getByTestId('asset-query').textContent ?? '');
    expect(rawRootQuery.get('location')).toBe('raw');
    expect(rawRootQuery.get('rawScope')).toBe('all');
    for (const key of ['sourceGroup', 'logicalGroup', 'path']) expect(rawRootQuery.has(key)).toBe(false);
    expect(treeItem('RAW', 1).getAttribute('aria-selected')).toBe('true');
    expect(api.assets).toHaveBeenCalledTimes(1);
  });
});
