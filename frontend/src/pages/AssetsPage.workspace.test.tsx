// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetGroup, AssetInventory, AssetPath, AssetSourceGroup, ExternalSubtitle, Library, ProfileAssignment, ScanResult } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ApiRequestError: actual.ApiRequestError,
    api: {
      ...actual.api,
      queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
      profileAssignments: vi.fn(), assetScopeConfigurations: vi.fn(), latestSnapshot: vi.fn(), testEncodes: vi.fn(), jellyfinOutcomes: vi.fn(), effectiveAssetConfigurations: vi.fn(), deleteConvertedAsset: vi.fn(),
      runtimeState: vi.fn(), createQueueJob: vi.fn(), createQueueBatchPreflight: vi.fn(),
      externalAssetSubtitles: vi.fn(), assetMetadataSidecars: vi.fn(), subtitleEligibility: vi.fn(), trackMaintenanceInventory: vi.fn(),
      subtitleExtractionOperations: vi.fn(), subtitleTranslationCapability: vi.fn(), startSubtitleTranslation: vi.fn(),
    },
  };
});

import { api, ApiRequestError } from '../api/client';
import { AssetsPage, ReportsPage } from './AssetsPage';
import { assetLabHref } from '../components/assetsActionRoutes';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

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

function publishedAssetForTranslation(status: string, provenance: string, fileName: string): Asset {
  return {
    ...asset(1, fileName),
    status,
    lifecycle: {
      location: 'library', lifecycle: 'published', provenance, validation: 'passed', publication: 'published', original: 'archived',
      actions: {
        manageSubtitles: { allowed: true },
        manageTracks: { allowed: true },
        rename: { allowed: true },
        deleteArchivedOriginal: { allowed: false, reason: 'original_unavailable' },
      },
    },
  } as Asset;
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

function subtitleSidecar(mediaPath: string, suffix = 'eng'): ExternalSubtitle {
  const pathValue = `${mediaPath.slice(0, mediaPath.lastIndexOf('/') + 1)}Episode 01.${suffix}.ass`;
  return {
    path: pathValue,
    fileName: pathValue.slice(pathValue.lastIndexOf('/') + 1),
    format: 'ass', language: 'eng', default: false, forced: suffix.includes('forced'), sizeBytes: 100, modifiedAt: '',
  };
}

function translationCapabilityFor(mediaPath: string, sidecar: ExternalSubtitle, eligible: boolean, available: boolean, reason: string) {
  return {
    source: {
      type: 'external', sourceIdentity: `external:${sidecar.path}:revision-1`, sourceRevision: 'revision-1',
      representation: 'ass', externalPath: sidecar.path, artifactId: `external-subtitle:${sidecar.path}`,
      codec: 'ass', language: 'eng', default: false, sourceForced: false, effectiveForced: false,
      role: 'unknown', sourcePath: mediaPath,
    },
    eligible, available, reason,
    sourceLanguages: ['auto'], targetLocales: ['es-419', 'es-ES'],
    provider: { name: 'ollama', modelSelectable: false }, outputRepresentations: ['ass'],
    context: { status: 'unavailable', source: 'none' },
  } as never;
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

let rawDestinationLibraryId = 1;

function effectiveConfigurationsForIds(assetIds: number[]) {
  const libraryTargets: Asset[] = [
    asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'),
    asset(2, 'Episode 02.mkv'),
  ];
  const assetsById = new Map([...(rawWorkspaceInventory().raw ?? []), ...libraryTargets].map((entry) => [entry.id, entry]));
  const rawDest = rawDestinationLibraryId > 0
    ? { selection: 'value', destinationLibraryId: rawDestinationLibraryId }
    : { selection: 'inherit', destinationLibraryId: 0 };
  return {
    configurations: Object.fromEntries(assetIds.map((id) => {
      const target = assetsById.get(id);
      return [String(id), {
        assetPath: target?.path ?? `/asset/${id}`,
        video: { selection: 'profile', videoProfileId: 1 },
        audio: { selection: 'inherit' },
        tracks: { selection: 'inherit' },
        category: { selection: 'inherit' },
        destination: target?.status === 'unprocessed' ? rawDest : { selection: 'value', destinationLibraryId: 1 },
      }];
    })),
    missingAssetIds: [],
  };
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

function renderLibraryPage(queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } }), initialUrl = '/assets?location=library&search=Episode') {
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
    vi.mocked(api.runtimeState).mockResolvedValue({ disks: { raw: { type: 'local', totalBytes: 1000, availableBytes: 400 } } } as never);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([pathAssignment]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockImplementation(async (assetPath) => ({ found: true, snapshot: persistedSnapshot(assetPath), status: 'current', requiresAnalysis: false, staleComponents: [] }));
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => effectiveConfigurationsForIds(assetIds) as never);
    vi.mocked(api.createQueueJob).mockResolvedValue({ id: 99 } as never);
    vi.mocked(api.createQueueBatchPreflight).mockResolvedValue({ id: 'candidate-1', revision: 1, resolved: true, status: 'active', batchId: 'batch-1', batchName: 'Single asset Queue review', assets: [] } as never);
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([]);
    vi.mocked(api.assetMetadataSidecars).mockResolvedValue([]);
    vi.mocked(api.subtitleEligibility).mockResolvedValue({ embedded: [], external: [], composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' } } as never);
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({ path: '', fingerprint: '', streams: [], chapters: 0, durationSeconds: 0, maintenanceAllowed: true } as never);
    vi.mocked(api.subtitleExtractionOperations).mockResolvedValue({ operations: [] } as never);
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue({} as never);
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({} as never);
  });

  it.each([
    { status: 'converted', provenance: 'converted' },
    { status: 'published_as_is', provenance: 'published_as_is' },
    { status: 'accepted', provenance: 'imported' },
  ])('opens exact sidecar translation for the $status lifecycle', async ({ status, provenance }) => {
    const user = userEvent.setup();
    const filename = 'Episode 01.mkv';
    const published = publishedAssetForTranslation(status, provenance, filename);
    const sidecar = subtitleSidecar(published.path);
    const sibling = subtitleSidecar(published.path, 'eng.forced');
    vi.mocked(api.assets).mockResolvedValue(inventory([published]));
    vi.mocked(api.subtitleEligibility).mockResolvedValue({ embedded: [], external: [sidecar, sibling], composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' } } as never);
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([sidecar, sibling]);
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(translationCapabilityFor(
      published.path,
      sidecar,
      true,
      false,
      'Subtitle translation is disabled. Enable it in Settings to translate subtitles.',
    ));

    renderLibraryPage();
    const rowText = await screen.findByText(filename);
    await user.click(rowText);
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const assetInfo = await screen.findByRole('dialog');
    await user.click(within(assetInfo).getByRole('tab', { name: 'Tracks' }));

    const translate = await screen.findByRole('button', { name: `Translate ${sidecar.fileName}` });
    expect(translate).toHaveProperty('disabled', false);
    await user.click(translate);
    await screen.findByText('Translate subtitle');
    const dialog = screen.getAllByRole('dialog').at(-1)!;
    expect(await within(dialog).findByText('Subtitle translation is disabled. Enable it in Settings to translate subtitles.')).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(api.subtitleTranslationCapability).toHaveBeenCalledWith({
      path: published.path,
      source: { type: 'external', external: { subtitlePath: sidecar.path } },
    });
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  }, 15000);

  it('keeps active-work blockers visible and leaves adjacent sidecar mutations disabled', async () => {
    const user = userEvent.setup();
    const filename = 'Episode 01.mkv';
    const published = publishedAssetForTranslation('converted', 'converted', filename);
    const sidecar = subtitleSidecar(published.path);
    published.lifecycle = {
      ...published.lifecycle!,
      actions: {
        ...published.lifecycle!.actions,
        manageSubtitles: { allowed: false, reason: 'active_work' },
      },
    } as never;
    vi.mocked(api.assets).mockResolvedValue(inventory([published]));
    vi.mocked(api.subtitleEligibility).mockResolvedValue({ embedded: [], external: [sidecar], composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' } } as never);
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([sidecar]);

    renderLibraryPage();
    const rowText = await screen.findByText(filename);
    await user.click(rowText);
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const assetInfo = await screen.findByRole('dialog');
    await user.click(within(assetInfo).getByRole('tab', { name: 'Tracks' }));

    const translate = await screen.findByRole('button', { name: `Translate ${sidecar.fileName}` });
    expect(translate).toHaveProperty('disabled', true);
    await user.hover(translate.parentElement!);
    expect(await screen.findByText('An active Queue or maintenance operation must finish before subtitle translation is available.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Rename subtitle file' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('button', { name: 'Edit subtitle text' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('button', { name: 'Delete external subtitle' })).toHaveProperty('disabled', true);
    expect(api.subtitleTranslationCapability).not.toHaveBeenCalled();
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  }, 15000);

  it('shows exact backend source blockers in the dialog without enabling Start', async () => {
    const user = userEvent.setup();
    const filename = 'Episode 01.mkv';
    const published = publishedAssetForTranslation('converted', 'converted', filename);
    const sidecar = subtitleSidecar(published.path);
    vi.mocked(api.assets).mockResolvedValue(inventory([published]));
    vi.mocked(api.subtitleEligibility).mockResolvedValue({ embedded: [], external: [sidecar], composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' } } as never);
    vi.mocked(api.externalAssetSubtitles).mockResolvedValue([sidecar]);
    vi.mocked(api.subtitleTranslationCapability).mockRejectedValue(new ApiRequestError(
      'Archived originals cannot manage external subtitles.',
      422,
      { reason: 'Archived originals cannot manage external subtitles.' },
    ));

    renderLibraryPage();
    const rowText = await screen.findByText(filename);
    await user.click(rowText);
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const assetInfo = await screen.findByRole('dialog');
    await user.click(within(assetInfo).getByRole('tab', { name: 'Tracks' }));
    await user.click(await screen.findByRole('button', { name: `Translate ${sidecar.fileName}` }));

    await screen.findByText('Translate subtitle');
    const dialog = screen.getAllByRole('dialog').at(-1)!;
    expect(await within(dialog).findByText('Archived originals cannot manage external subtitles.')).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  }, 15000);

  it('creates no row thumbnail requests and only one image request per selected asset', async () => {
    const user = userEvent.setup();
    vi.mocked(api.assets).mockResolvedValue(inventory(Array.from({ length: 12 }, (_, index) => asset(index + 1, `Episode ${index + 1}.mkv`))));
    // Each new/changed image src initiates the browser request. jsdom does not
    // fetch images, so observe that boundary directly rather than mock fetch.
    const imageAttributes = vi.spyOn(HTMLImageElement.prototype, 'setAttribute');
    const thumbnailRequests = () => imageAttributes.mock.calls
      .filter(([name, value]) => name === 'src' && value.endsWith('/thumbnail'))
      .map(([, value]) => value);
    const { queryClient } = renderLibraryPage();
    const row = (await screen.findByText('Episode 1.mkv')).closest('tr')!;
    expect(thumbnailRequests()).toEqual([]);
    expect(screen.queryByTestId('selected-asset-detail')).toBeNull();
    expect(screen.getByRole('table', { name: 'Library assets' }).querySelectorAll('img')).toHaveLength(0);

    await user.click(row);
    const image = await screen.findByRole('img', { name: 'Episode 1.mkv artwork' });
    expect(thumbnailRequests()).toEqual(['/api/assets/1/thumbnail']);
    fireEvent.error(image);
    expect(screen.getByRole('img', { name: 'MVForge artwork placeholder for Episode 1.mkv' })).toBeTruthy();
    await act(async () => { await queryClient.invalidateQueries({ queryKey: ['assets'] }); });
    expect(thumbnailRequests()).toEqual(['/api/assets/1/thumbnail']);

    await user.click(screen.getByText('Episode 2.mkv'));
    expect(await screen.findByRole('img', { name: 'Episode 2.mkv artwork' })).toBeTruthy();
    expect(thumbnailRequests()).toEqual(['/api/assets/1/thumbnail', '/api/assets/2/thumbnail']);
    expect(screen.getByRole('table', { name: 'Library assets' }).querySelectorAll('img')).toHaveLength(0);
  });

  it('uses the batched global effective profile for Library labels, Advisor keys, and Test Encode', async () => {
    const user = userEvent.setup();
    vi.mocked(api.profileAssignments).mockResolvedValue([]);
    const globalProfile = { ...profile, id: 7, name: 'Global HEVC' };
    vi.mocked(api.profiles).mockResolvedValue([globalProfile] as never);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      Object.values(result.configurations).forEach((config) => { config.video = { selection: 'profile', videoProfileId: 7, source: 'global' } as never; });
      return result as never;
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(['advisor', 'asset-row', `${path}/Episode 02.mkv`, 7], { score: 94 });
    renderLibraryPage(client);
    const row = (await screen.findByText('Episode 02.mkv')).closest('tr')!;
    await user.click(row);
    const details = await screen.findByTestId('selected-asset-detail');
    expect(details.textContent).toContain('Global HEVC');
    expect(within(row).getByText('94')).toBeTruthy();
    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    expect(api.effectiveAssetConfigurations).toHaveBeenCalledWith([1, 2]);
  });

  it('uses the effective Asset profile instead of a stale Path assignment in Library V2', async () => {
    const assetProfile = { ...profile, id: 8, name: 'Asset HEVC', scope: 'asset' };
    vi.mocked(api.profiles).mockResolvedValue([profile, assetProfile] as never);
    vi.mocked(api.profileAssignments).mockResolvedValue([
      pathAssignment,
      { ...pathAssignment, id: 2, targetType: 'asset', targetPath: `${path}/A Very Long Episode Filename That Should Stay Inside Its Column.mkv`, videoProfileId: 8 },
    ] as never);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].video = { selection: 'profile', videoProfileId: 8, source: 'asset' } as never;
      return result as never;
    });
    const user = userEvent.setup();
    renderLibraryPage();
    await user.click(await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv'));
    const details = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect(details.textContent).toContain('Asset HEVC'));
  });

  it('shows the effective Path category in Library V2 rows and Selected Asset detail', async () => {
    const libraryInventory = inventory([{
      ...asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'),
      metadata: { categories: ['TV'], tags: [], updatedAt: '' },
    }, asset(2, 'Episode 02.mkv')]);
    vi.mocked(api.assets).mockResolvedValue(libraryInventory);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].category = { selection: 'value', category: 'Path Drama', source: 'path' } as never;
      return result as never;
    });
    const user = userEvent.setup();
    renderLibraryPage();
    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    await user.click(await screen.findByText(filename));
    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByLabelText('Category / Library: Path Drama')).toBeTruthy();
    const row = within(screen.getByRole('region', { name: 'Library asset list' })).getByText(filename).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    expect((await screen.findByRole('combobox', { name: 'Category' }) as HTMLInputElement).value).toBe('TV');
  });

  it('shows the effective Asset category over a Path category in Library V2', async () => {
    const libraryInventory = inventory([
      { ...asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'), metadata: { categories: ['TV'], tags: [], updatedAt: '' } },
      asset(2, 'Episode 02.mkv'),
    ]);
    libraryInventory.libraryGroups[0].pathMetadata = { categories: ['Path Drama'], tags: [], updatedAt: '' };
    vi.mocked(api.assets).mockResolvedValue(libraryInventory);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].category = { selection: 'value', category: 'Asset Mystery', source: 'asset' } as never;
      return result as never;
    });
    const user = userEvent.setup();
    renderLibraryPage();
    await user.click(await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv'));
    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByLabelText('Category / Library: Asset Mystery')).toBeTruthy();
    expect(within(details).queryByLabelText('Category / Library: Path Drama')).toBeNull();
    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    const row = within(screen.getByRole('region', { name: 'Library asset list' })).getByText(filename).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    expect((await screen.findByRole('combobox', { name: 'Category' }) as HTMLInputElement).value).toBe('TV');
  });

  it('shows disabled effective category instead of stale Library metadata', async () => {
    vi.mocked(api.assets).mockResolvedValue(inventory([{
      ...asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'),
      metadata: { categories: ['TV'], tags: [], updatedAt: '' },
    }, asset(2, 'Episode 02.mkv')]));
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].category = { selection: 'disabled', source: 'path' } as never;
      return result as never;
    });
    const user = userEvent.setup();
    renderLibraryPage();
    await user.click(await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv'));
    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByLabelText('Category / Library: Disabled')).toBeTruthy();
    expect(within(details).queryByLabelText('Category / Library: Anime')).toBeNull();
    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    const row = within(screen.getByRole('region', { name: 'Library asset list' })).getByText(filename).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    expect((await screen.findByRole('combobox', { name: 'Category' }) as HTMLInputElement).value).toBe('TV');
  });

  it('uses existing metadata fallback when effective Library category inherits', async () => {
    vi.mocked(api.assets).mockResolvedValue(inventory([{
      ...asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'),
      metadata: { categories: ['TV'], tags: [], updatedAt: '' },
    }, asset(2, 'Episode 02.mkv')]));
    const user = userEvent.setup();
    renderLibraryPage();
    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    await user.click(await screen.findByText(filename));
    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByLabelText('Category / Library: TV')).toBeTruthy();
    const row = within(screen.getByRole('region', { name: 'Library asset list' })).getByText(filename).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    expect((await screen.findByRole('combobox', { name: 'Category' }) as HTMLInputElement).value).toBe('TV');
  });

  it('saves Asset Info metadata without changing an effective Path category', async () => {
    const user = userEvent.setup();
    vi.mocked(api.assets).mockResolvedValue(inventory([{
      ...asset(1, 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv'),
      metadata: { categories: ['TV'], tags: [], updatedAt: '' },
    }, { ...asset(2, 'Episode 02.mkv'), metadata: { categories: ['Anime'], tags: [], updatedAt: '' } }]));
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].category = { selection: 'value', category: 'Path Drama', source: 'path' } as never;
      return result as never;
    });
    const update = vi.spyOn(api, 'updateAssetEdit').mockImplementation(async (input) => ({
      path: input.path,
      metadata: { ...input.metadata, updatedAt: '', inheritedFrom: undefined },
      conversion: input.conversion,
    } as never));
    const updateScope = vi.spyOn(api, 'updateAssetScopeConfiguration');
    renderLibraryPage();
    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    await user.click(await screen.findByText(filename));
    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByLabelText('Category / Library: Path Drama')).toBeTruthy();
    const row = within(screen.getByRole('region', { name: 'Library asset list' })).getByText(filename).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: `More actions for ${filename}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
    const category = await screen.findByRole('combobox', { name: 'Category' }) as HTMLInputElement;
    expect(category.value).toBe('TV');
    await user.click(category);
    await user.click(await screen.findByRole('option', { name: 'anime' }));
    expect(category.value).toBe('anime');
    expect(within(details).getByLabelText('Category / Library: Path Drama')).toBeTruthy();
    await user.click(await screen.findByRole('tab', { name: 'Quick Asset Overrides' }));
    await user.click(await screen.findByRole('button', { name: 'Save' }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      path: `${path}/${filename}`,
      metadata: expect.objectContaining({ categories: ['anime'] }),
    }));
    expect(update).not.toHaveBeenCalledWith(expect.objectContaining({ metadata: expect.objectContaining({ categories: ['Disabled'] }) }));
    expect(updateScope).not.toHaveBeenCalled();
    expect(within(details).getByLabelText('Category / Library: Path Drama')).toBeTruthy();
    update.mockRestore();
    updateScope.mockRestore();
  });

  it.each([
    { selection: 'inherit', label: 'Inherit · no effective profile' },
    { selection: 'disabled', label: 'Disabled' },
  ])('preserves Library effective video state $selection', async ({ selection, label }) => {
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].video = { selection, videoProfileId: 0, source: 'path' } as never;
      return result as never;
    });
    const user = userEvent.setup();
    renderLibraryPage();
    await user.click(await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv'));
    const details = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect(details.textContent).toContain(label));
    expect((within(details).getByRole('button', { name: 'Test Encode' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('allows Global Custom Test Encode in Library V2 only when effective profile data exists', async () => {
    const user = userEvent.setup();
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      result.configurations['1'].video = { selection: 'value', source: 'global', profileData: { videoCodec: 'libx265' } } as never;
      return result as never;
    });
    renderLibraryPage();
    await user.click(await screen.findByText('A Very Long Episode Filename That Should Stay Inside Its Column.mkv'));
    const details = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect(details.textContent).toContain('Custom'));
    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
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
    expect(screen.getByTestId('assets-storage-footer')).toBeTruthy();
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

  it('opens selected detail separately from Library bulk selection and loads only persisted media evidence', async () => {
    const user = userEvent.setup();
    const { queryClient } = renderLibraryPage();

    const filename = 'A Very Long Episode Filename That Should Stay Inside Its Column.mkv';
    const rowText = await screen.findByText(filename);
    const row = rowText.closest('tr')!;
    await user.click(rowText);

    const list = screen.getByRole('region', { name: 'Library asset list' });
    const table = within(list).getByRole('table', { name: 'Library assets' });
    const details = await screen.findByTestId('selected-asset-detail');
    expect(getComputedStyle(list).overflowY).toBe('auto');
    expect(list.contains(table)).toBe(true);
    expect(list.contains(details)).toBe(false);
    expect(list.contains(screen.getByLabelText('Total Assets: 2'))).toBe(false);
    expect(screen.getByTestId('assets-topbar').contains(screen.getByLabelText('Search assets, series, movies'))).toBe(true);
    expect(within(details).getByTestId('selected-asset-identity').textContent).toContain(filename);
    expect(within(details).getByTestId('selected-asset-path-context').textContent).toBe('Series/Season 01');
    const media = within(within(details).getByTestId('selected-asset-technical'));
    expect(media.getByText('Video')).toBeTruthy();
    expect(media.getByText('hevc')).toBeTruthy();
    expect(media.getByText('Technical information')).toBeTruthy();
    expect(within(details).getAllByText('1920 × 1080')).toHaveLength(2);
    expect(media.queryByText('Encoder')).toBeNull();
    expect(media.queryByText('Bitrate')).toBeNull();
    expect(media.queryByText('Range')).toBeNull();
    expect(within(details).getByText('Duration')).toBeTruthy();
    expect(within(details).getByText('Modified')).toBeTruthy();
    expect(media.getByText('Audio')).toBeTruthy();
    expect(within(details).getByTestId('selected-asset-status').getAttribute('aria-label')).toContain('Status:');
    expect(within(details).getByText('Profile')).toBeTruthy();
    expect(within(details).getByText('Subtitles')).toBeTruthy();
    expect(within(details).getByText('Audio')).toBeTruthy();
    expect(within(screen.getByTestId('selected-asset-quick-facts')).getByLabelText('Category / Library: Anime')).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: /More actions/ })).toBeTruthy();
    const detailAssign = within(details).getByRole('button', { name: 'Assign Profile' }) as HTMLButtonElement;
    await waitFor(() => expect(detailAssign.disabled).toBe(false));
    await user.click(detailAssign);
    expect(await screen.findByRole('dialog', { name: /Assign Profile/ })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Assign Profile/ })).toBeNull());
    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Generate Test Encode' })).toBeNull());
    await user.click(within(details).getByRole('button', { name: /More actions/ }));
    expect(await screen.findByRole('menuitem', { name: 'Asset Info' })).toBeTruthy();
    await user.keyboard('{Escape}');
    expect((within(row).getByRole('checkbox', { name: `Select asset ${filename}` }) as HTMLInputElement).disabled).toBe(true);
    expect(screen.queryByLabelText('1 selected')).toBeNull();
    expect(within(row).getByText('Unverified')).toBeTruthy();
    expect(within(row).getByText('hevc')).toBeTruthy();
    expect(within(row).getByText('hevc_videotoolbox')).toBeTruthy();
    expect(within(row).getByText('23m 40s')).toBeTruthy();
    expect(within(row).getByText(/Sep 30, 2026/)).toBeTruthy();
    await waitFor(() => expect(api.latestSnapshot).toHaveBeenCalledTimes(1));
    expect(api.latestSnapshot).toHaveBeenCalledWith(`${path}/${filename}`);

    const assetsCallsBeforeRefresh = vi.mocked(api.assets).mock.calls.length;
    await act(async () => { await queryClient.invalidateQueries({ queryKey: ['assets'] }); });
    await waitFor(() => expect(vi.mocked(api.assets).mock.calls.length).toBeGreaterThan(assetsCallsBeforeRefresh));
    expect(screen.getByTestId('selected-asset-detail').getAttribute('aria-label')).toContain(filename);
    expect(api.latestSnapshot).toHaveBeenCalledTimes(1);
  });

  it('offers Library Safe Delete checkboxes only for eligible assets and deletes only checkbox-selected targets', async () => {
    const user = userEvent.setup();
    const eligiblePath = `${path}/Converted Episode.mkv`;
    const secondEligiblePath = `${path}/Second Converted Episode.mkv`;
    const blockedPath = `${path}/Blocked Episode.mkv`;
    const lifecycle = (allowed: boolean) => ({
      location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed',
      publication: 'published', original: 'archived',
      actions: { deleteArchivedOriginal: { allowed, reason: allowed ? '' : 'original_unavailable' } },
    });
    const eligible = { ...asset(31, 'Converted Episode.mkv'), status: 'converted', lifecycle: lifecycle(true) } as Asset;
    const secondEligible = { ...asset(33, 'Second Converted Episode.mkv'), status: 'converted', lifecycle: lifecycle(true) } as Asset;
    const blocked = { ...asset(32, 'Blocked Episode.mkv'), status: 'converted', lifecycle: lifecycle(false) } as Asset;
    vi.mocked(api.assets).mockResolvedValue(inventory([eligible, secondEligible, blocked]));
    vi.mocked(api.deleteConvertedAsset).mockResolvedValue({ message: 'Deleted and restored.' } as never);
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderLibraryPage(undefined, `/assets?location=library&libraryId=1&libraryPath=${encodeURIComponent(path)}`);

    const eligibleRowText = await screen.findByText('Converted Episode.mkv');
    const eligibleRow = eligibleRowText.closest('tr')!;
    const secondEligibleRow = screen.getByText('Second Converted Episode.mkv').closest('tr')!;
    const blockedRow = screen.getByText('Blocked Episode.mkv').closest('tr')!;
    const eligibleCheckbox = within(eligibleRow).getByRole('checkbox', { name: 'Select asset Converted Episode.mkv' });
    const secondEligibleCheckbox = within(secondEligibleRow).getByRole('checkbox', { name: 'Select asset Second Converted Episode.mkv' });
    const blockedCheckbox = within(blockedRow).getByRole('checkbox', { name: 'Select asset Blocked Episode.mkv' });
    const headerCheckbox = screen.getByRole('checkbox', { name: 'Select all visible Library assets' });
    expect((eligibleCheckbox as HTMLInputElement).disabled).toBe(false);
    expect((blockedCheckbox as HTMLInputElement).disabled).toBe(true);
    expect((headerCheckbox as HTMLInputElement).disabled).toBe(false);
    expect((headerCheckbox as HTMLInputElement).checked).toBe(false);

    await user.click(eligibleRowText);
    expect(await screen.findByTestId('selected-asset-detail')).toBeTruthy();
    expect(screen.queryByLabelText('1 selected')).toBeNull();
    await user.click(eligibleCheckbox);
    expect((eligibleCheckbox as HTMLInputElement).checked).toBe(true);
    expect((blockedCheckbox as HTMLInputElement).checked).toBe(false);
    expect(headerCheckbox.getAttribute('aria-checked')).toBe('mixed');
    expect(screen.getByLabelText('1 selected')).toBeTruthy();
    expect(screen.getByTestId('selected-asset-detail').getAttribute('aria-label')).toContain('Converted Episode.mkv');

    await user.click(headerCheckbox);
    expect((headerCheckbox as HTMLInputElement).checked).toBe(true);
    expect((secondEligibleCheckbox as HTMLInputElement).checked).toBe(true);
    expect((blockedCheckbox as HTMLInputElement).checked).toBe(false);
    expect(screen.getByLabelText('2 selected')).toBeTruthy();
    await user.click(secondEligibleCheckbox);
    expect(headerCheckbox.getAttribute('aria-checked')).toBe('mixed');
    expect(screen.getByLabelText('1 selected')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Delete selected' }));
    await waitFor(() => expect(api.deleteConvertedAsset).toHaveBeenCalledTimes(1));
    expect(api.deleteConvertedAsset).toHaveBeenCalledWith(eligiblePath);
    expect(api.deleteConvertedAsset).not.toHaveBeenCalledWith(secondEligiblePath);
    expect(api.deleteConvertedAsset).not.toHaveBeenCalledWith(blockedPath);
    expect(confirm).toHaveBeenCalledOnce();
    confirm.mockRestore();
  });

  it('keeps Move path and Rename path icons visible in the Assets V2 toolbar', async () => {
    renderLibraryPage(undefined, `/assets?location=library&libraryId=1&libraryPath=${encodeURIComponent(path)}`);
    await screen.findByRole('columnheader', { name: 'Asset' });
    const toolbar = screen.getByTestId('assets-path-toolbar');
    for (const name of ['Move path', 'Rename path']) {
      const button = within(toolbar).getByRole('button', { name });
      expect(button.querySelector('.MuiButton-startIcon svg')).toBeTruthy();
    }
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
    expect(within(toolbar).getByRole('switch', { name: 'Confidence On' })).toBeTruthy();
    await user.click(within(toolbar).getByRole('button', { name: 'Configure Path' }));
    const configure = await screen.findByRole('dialog', { name: 'Configure Path' });
    await waitFor(() => expect(within(configure).getByRole('checkbox', { name: 'Change video' })).toBeTruthy());
    expect((within(configure).getByRole('checkbox', { name: 'Change destination' }) as HTMLInputElement).disabled).toBe(true);
    await user.click(within(configure).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Configure Path' })).toBeNull());

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
    expect(await within(cachedRow!).findByText('87')).toBeTruthy();
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
    await user.keyboard('{Escape}');
    await user.click(row);
    const details = await screen.findByTestId('selected-asset-detail');
    const assignButton = within(details).getByRole('button', { name: 'Assign Profile' }) as HTMLButtonElement;
    const testEncodeButton = within(details).getByRole('button', { name: 'Test Encode' }) as HTMLButtonElement;
    expect(assignButton.disabled).toBe(true);
    expect(testEncodeButton.disabled).toBe(true);
    expect(within(details).getByLabelText(/current Library lifecycle/)).toBeTruthy();
    expect(within(details).getByLabelText(/current lifecycle/)).toBeTruthy();
  });
});

describe('RAW Assets workspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    rawDestinationLibraryId = 1;
    vi.mocked(api.queueJobs).mockResolvedValue([]);
    vi.mocked(api.assets).mockResolvedValue(rawWorkspaceInventory());
    vi.mocked(api.profiles).mockResolvedValue([profile] as never);
    vi.mocked(api.libraries).mockResolvedValue([library]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.runtimeState).mockResolvedValue({ disks: { raw: { type: 'local', totalBytes: 1000, availableBytes: 400 } } } as never);
    vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
    vi.mocked(api.profileAssignments).mockResolvedValue([pathAssignment]);
    vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockImplementation(async (assetPath) => ({ found: true, snapshot: persistedSnapshot(assetPath), status: 'current', requiresAnalysis: false, staleComponents: [] }));
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => effectiveConfigurationsForIds(assetIds) as never);
    vi.mocked(api.createQueueJob).mockResolvedValue({ id: 99 } as never);
    vi.mocked(api.createQueueBatchPreflight).mockResolvedValue({ id: 'candidate-1', revision: 1, resolved: true, status: 'active', batchId: 'batch-1', batchName: 'Single asset Queue review', assets: [] } as never);
  });

  it('loads artwork for the one selected RAW identity and keeps every RAW row image-free', async () => {
    const user = userEvent.setup();
    const imageAttributes = vi.spyOn(HTMLImageElement.prototype, 'setAttribute');
    const thumbnailRequests = () => imageAttributes.mock.calls
      .filter(([name, value]) => name === 'src' && value.endsWith('/thumbnail'))
      .map(([, value]) => value);
    renderRawPage();
    const table = await screen.findByRole('table', { name: 'RAW assets' });
    const selected = await screen.findByRole('img', { name: 'S01E01.mkv artwork' });
    expect(selected.getAttribute('src')).toBe('/api/assets/11/thumbnail');
    expect(thumbnailRequests()).toEqual(['/api/assets/11/thumbnail']);
    expect(table.querySelectorAll('img')).toHaveLength(0);
    await user.click(within(table).getByText('Trailer.mkv'));
    expect(await screen.findByRole('img', { name: 'Trailer.mkv artwork' })).toBeTruthy();
    expect(thumbnailRequests()).toEqual(['/api/assets/11/thumbnail', '/api/assets/12/thumbnail']);
    expect(table.querySelectorAll('img')).toHaveLength(0);
  });

  it('selects only visible RAW assets through the shared header checkbox', async () => {
    const user = userEvent.setup();
    renderRawPage();
    const table = await screen.findByRole('table', { name: 'RAW assets' });
    const header = within(table).getByRole('checkbox', { name: 'Select all visible RAW assets' }) as HTMLInputElement;
    expect(header.disabled).toBe(false);
    await user.click(header);
    expect(header.checked).toBe(true);
    expect((within(table).getByRole('checkbox', { name: 'Select asset S01E01.mkv' }) as HTMLInputElement).checked).toBe(true);
    expect((within(table).getByRole('checkbox', { name: 'Select asset Trailer.mkv' }) as HTMLInputElement).checked).toBe(true);
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
    const titleToolbar = screen.getByTestId('assets-path-toolbar');
    for (const name of ['Scan', 'Advisor', 'Configure Path', 'Move to library']) {
      expect(within(titleToolbar).getByRole('button', { name })).toBeTruthy();
    }
    expect(within(titleToolbar).getByRole('checkbox', { name: 'Select title Attack on Titan' })).toBeTruthy();
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
    expect(screen.getByTestId('assets-storage-footer')).toBeTruthy();
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
    expect(within(details).getByTestId('selected-asset-technical').textContent).toContain('Technical information');
    expect(within(details).getByTestId('selected-asset-technical').textContent).toContain('hevc');
    expect(within(details).getByTestId('selected-asset-technical').textContent).toContain('1920 × 1080');
    expect(within(details).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: /More actions/ })).toBeTruthy();
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

  it('routes Play, Assign Profile, Test Encode, and More from RAW selected detail to the selected asset', async () => {
    const user = userEvent.setup();
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');

    const details = await screen.findByTestId('selected-asset-detail');
    expect(within(details).getByTestId('selected-asset-identity').textContent).toContain('S01E01.mkv');
    await user.click(within(details).getByRole('button', { name: 'Play preview of S01E01.mkv' }));
    expect(await screen.findByRole('dialog', { name: 'Asset Preview' })).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Asset Preview' })).toBeNull());

    await user.click(within(details).getByRole('button', { name: 'Assign Profile' }));
    expect(await screen.findByRole('dialog', { name: /Assign Profile/ })).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: /Assign Profile/ })).toBeNull());

    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Generate Test Encode' })).toBeNull());

    await user.click(within(details).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    expect(await screen.findByRole('menuitem', { name: 'Asset Info' })).toBeTruthy();
    expect(screen.getByRole('menuitem', { name: 'Queue asset' })).toBeTruthy();
  });

  it('persists a suggested Asset profile, reports failure, and resolves saved configuration for Queue and Test Encode', async () => {
    const user = userEvent.setup();
    const suggested = { ...profile, id: 8, name: 'Suggested Asset HEVC', scope: 'asset', container: 'mkv', videoCodec: 'hevc', qualityValue: 21 };
    vi.mocked(api.profiles).mockResolvedValue([profile, suggested] as never);
    const suggest = vi.spyOn(api, 'suggestProfile').mockResolvedValue({ matchType: 'existing', summary: 'Suggested from persisted evidence', scan: persistedSnapshot('/media/raw/anime/Attack on Titan/S01E01.mkv'), suggestedProfile: suggested, candidates: [], insights: {}, findings: [] } as never);
    const assign = vi.spyOn(api, 'updateProfileAssignment').mockRejectedValueOnce(new Error('Profile save rejected')).mockImplementation(async (input) => {
      vi.mocked(api.profileAssignments).mockResolvedValue([{ ...input, id: 99 }] as never);
      vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
        const result = effectiveConfigurationsForIds(ids);
        Object.values(result.configurations).forEach((config) => { config.video = { selection: 'profile', videoProfileId: 8 }; });
        return result as never;
      });
      return input as never;
    });
    const createTest = vi.spyOn(api, 'createTestEncode').mockResolvedValue({ id: 30 } as never);
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Asset Info' }));
    await user.click(await screen.findByRole('tab', { name: 'MVForge Suggestions' }));
    await user.click(await screen.findByRole('button', { name: 'Select suggested profile' }));
    expect(await within(await screen.findByRole('dialog')).findByText('Profile save rejected')).toBeTruthy();
    expect(details.textContent).not.toContain('Suggested Asset HEVC');
    await user.click(screen.getByRole('button', { name: 'Select suggested profile' }));
    await waitFor(() => expect(details.textContent).toContain('Suggested Asset HEVC'));
    expect(assign.mock.calls.at(-1)?.[0]).toEqual(expect.objectContaining({ targetType: 'asset', mediaType: 'video', selection: 'profile', videoProfileId: 8 }));
    const discardDraft = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const assetInfoDialog = screen.getByRole('dialog');
    await user.click(assetInfoDialog.parentElement!);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    const refreshedDetails = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect((within(refreshedDetails).getByRole('button', { name: 'Test Encode' }) as HTMLButtonElement).disabled).toBe(false));
    await user.click(within(refreshedDetails).getByRole('button', { name: 'Test Encode' }));
    await user.click(await screen.findByRole('button', { name: 'Generate New Test Encode' }));
    await waitFor(() => expect(createTest).toHaveBeenCalledWith(expect.objectContaining({ profileId: 8, resolveAssignments: true, libraryId: 1 })));
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    await user.click(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Queue asset' }));
    await waitFor(() => expect(api.createQueueBatchPreflight).toHaveBeenCalledWith(expect.objectContaining({ jobs: [expect.objectContaining({ profileId: 8, resolveProfileAssignments: true, libraryId: 1 })] })));
    discardDraft.mockRestore();
    suggest.mockRestore(); assign.mockRestore(); createTest.mockRestore();
  }, 10000);

  it('shows compact individual Queue failure feedback outside Asset Info', async () => {
    const user = userEvent.setup();
    let rejectQueue!: (reason: Error) => void;
    vi.mocked(api.createQueueBatchPreflight).mockImplementation(() => new Promise((_, reject) => { rejectQueue = reject; }));
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Queue asset' }));
    await act(async () => rejectQueue(new Error('Destination is unavailable')));
    expect(await screen.findByText('Destination is unavailable')).toBeTruthy();
    expect(screen.queryByRole('dialog', { name: /Asset Info/ })).toBeNull();
  });

  it('keeps the dirty Asset Info Queue warning visible after the dialog closes', async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const table = await screen.findByRole('table', { name: 'RAW assets' });
    await user.click(within(table).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Asset Info' }));
    await user.click(await screen.findByRole('combobox', { name: 'Category' }));
    await user.click(await screen.findByRole('option', { name: 'anime' }));

    fireEvent.click(within(table).getByRole('button', { name: 'More actions for S01E01.mkv', hidden: true }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Queue asset', hidden: true }));
    const queueWarning = 'Save Asset Info changes before Queue. Unsaved changes are not queued.';
    expect((await screen.findAllByText(queueWarning)).length).toBeGreaterThan(0);
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.getByText(queueWarning)).toBeTruthy();
    confirm.mockRestore();
  });

  it.each(['audio_only', 'override_only', 'disabled'])('explains unsupported %s Test Encode configuration', async (selection) => {
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      Object.values(result.configurations).forEach((config) => { config.video = { selection, videoProfileId: 0 }; });
      return result as never;
    });
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const details = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect((within(details).getByRole('button', { name: 'Test Encode' }) as HTMLButtonElement).disabled).toBe(true));
    expect(details.textContent).toContain(selection === 'audio_only' ? 'Audio only' : selection === 'disabled' ? 'Disabled' : 'Overrides only');
  });

  it('keeps Test Encode disabled with a truthful reason while effective configuration is missing', async () => {
    vi.mocked(api.effectiveAssetConfigurations).mockResolvedValue({ configurations: {}, missingAssetIds: [11, 12] } as never);
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const details = await screen.findByTestId('selected-asset-detail');
    await waitFor(() => expect((within(details).getByRole('button', { name: 'Test Encode' }) as HTMLButtonElement).disabled).toBe(true));
    expect(within(details).getByLabelText(/Effective video configuration is loading or unavailable/i)).toBeTruthy();
  });

  it('keeps global Custom Test Encode enabled with the effective destination', async () => {
    const user = userEvent.setup();
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (ids) => {
      const result = effectiveConfigurationsForIds(ids);
      Object.values(result.configurations).forEach((config) => { config.video = { selection: 'value', source: 'global', profileData: { videoCodec: 'libx265' } } as never; });
      return result as never;
    });
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const details = await screen.findByTestId('selected-asset-detail');
    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    expect(await screen.findByRole('dialog', { name: 'Generate Test Encode' })).toBeTruthy();
  });

  it('reacts to an inherited destination update without opening Asset Info or remounting the RAW row', async () => {
    const user = userEvent.setup();
    rawDestinationLibraryId = 0;
    const { queryClient } = renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');
    const table = await screen.findByRole('table', { name: 'RAW assets' });
    const row = within(table).getByText('S01E01.mkv').closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    expect((screen.getByRole('menuitem', { name: 'Queue asset' }) as HTMLLIElement).getAttribute('aria-disabled')).toBe('true');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog', { name: /Asset Info/ })).toBeNull();

    rawDestinationLibraryId = 1;
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] });
    });
    await waitFor(() => expect(within(table).getByText('S01E01.mkv').closest('tr')).toBe(row));
    await user.click(within(row).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    expect((screen.getByRole('menuitem', { name: 'Queue asset' }) as HTMLLIElement).getAttribute('aria-disabled')).not.toBe('true');
  });

  it('submits a backend-resolved Custom Queue from a compact RAW row', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createQueueBatchPreflight).mockResolvedValue({
      id: 'candidate-single-1', revision: 1, resolved: true, status: 'active', batchId: 'single-1', batchName: 'Single asset: S01E01.mkv',
      assets: [{
        id: 'asset-single-1', mediaPath: '/media/raw/anime/Attack on Titan/S01E01.mkv', batchId: 'single-1', batchName: 'Single asset: S01E01.mkv',
        input: { mediaPath: '/media/raw/anime/Attack on Titan/S01E01.mkv', libraryId: 1, profileId: 0, processingMode: 'full_encode', priority: 5 },
        status: 'ready', blockers: [], warnings: [],
      }],
    } as never);
    vi.mocked(api.effectiveAssetConfigurations).mockImplementation(async (assetIds) => ({
      configurations: Object.fromEntries(assetIds.map((id) => [String(id), {
        assetPath: `/media/raw/anime/Attack on Titan/S01E01.mkv`,
        video: { selection: 'value', profileData: { videoCodec: 'libx265', crf: 21 } },
        audio: { selection: 'value', profileData: { codec: 'aac' } },
        tracks: { selection: 'value', profileData: { keepDefaultAudio: true } },
        category: { selection: 'inherit' },
        destination: { selection: 'value', destinationLibraryId: 1 },
      }])),
      missingAssetIds: [],
    }) as never);
    renderRawPage('/assets?location=raw&sourceGroup=7&logicalGroup=%2Fmedia%2Fraw%2Fanime%2FAttack%20on%20Titan');

    const table = await screen.findByRole('table', { name: 'RAW assets' });
    const row = within(table).getByText('S01E01.mkv').closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'More actions for S01E01.mkv' }));
    await user.click(screen.getByRole('menuitem', { name: 'Queue asset' }));
    await waitFor(() => expect(api.createQueueBatchPreflight).toHaveBeenCalledOnce());
    expect(await screen.findByRole('button', { name: /Accept & Queue/ })).toBeTruthy();
    expect(api.createQueueBatchPreflight).toHaveBeenCalledWith(expect.objectContaining({
      batchName: 'Single asset: S01E01.mkv',
      jobs: [expect.objectContaining({
        mediaPath: '/media/raw/anime/Attack on Titan/S01E01.mkv',
        libraryId: 1,
        profileId: 0,
        resolveProfileAssignments: true,
        batchPosition: 1,
      })],
    }));
  });

  it('shows Queue / Preflight failures in the visible V2 path toolbar area', async () => {
    const user = userEvent.setup();
    vi.mocked(api.createQueueBatchPreflight).mockRejectedValue(new Error('effective custom configuration rejected'));
    renderRawPage();

    const title = await screen.findByRole('treeitem', { name: 'Attack on Titan' });
    await user.click(rawTreeItem('Anime'));
    await user.click(title);
    title.focus();
    await user.keyboard('{ArrowRight}');
    await user.click(treeItem('extras', 4));

    const toolbar = await screen.findByTestId('assets-path-toolbar');
    await user.click(within(toolbar).getByRole('button', { name: 'Queue / Preflight' }));
    expect((await screen.findByTestId('assets-v2-path-feedback')).textContent).toContain('effective custom configuration rejected');
  });
});
