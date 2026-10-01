// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetGroup, AssetSourceGroup, Library, ScanResult } from '../api/types';
import { AssetsHierarchyPanel, AssetsSummaryStrip, SelectedAssetDetail } from './AssetsWorkspace';
import { buildLibraryTree, buildRawHierarchy } from './assetsWorkspaceModel';

afterEach(cleanup);

function makeAsset(overrides: Partial<Asset> = {}): Asset {
  return {
    id: 1,
    libraryId: 1,
    libraryName: 'Anime',
    path: '/library/Anime/Series/Season 01/Episode 01.mkv',
    relativePath: 'Series/Season 01/Episode 01.mkv',
    groupPath: '/library/Anime/Series/Season 01',
    fileName: 'Episode 01.mkv',
    extension: '.mkv',
    sizeBytes: 1_500_000,
    modifiedAt: '2026-09-30T10:00:00Z',
    status: 'library',
    missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' },
    conversion: {},
    technical: { videoCodec: 'hevc', width: 1920, height: 1080, duration: 1420, bitrate: 4_000_000, hdr: false },
    ...overrides,
  };
}

function makeLibrary(): Library {
  return { id: 1, name: 'Anime', sourcePath: '/raw/Anime', destinationPath: '/library/Anime', type: 'anime', validationRules: {}, createdAt: '', updatedAt: '' };
}

function makeGroup(asset = makeAsset()): AssetGroup {
  return {
    id: 'group-1', libraryId: 1, libraryName: 'Anime', path: '/library/Anime/Series/Season 01', relativePath: 'Series/Season 01',
    status: 'library', fileCount: 1, sizeBytes: asset.sizeBytes, modifiedAt: asset.modifiedAt, assets: [asset],
    review: asset.review, pathReview: asset.review, metadata: asset.metadata, pathMetadata: asset.metadata,
  };
}

const scan = {
  id: 1,
  path: '/library/Anime/Series/Season 01/Episode 01.mkv',
  fileName: 'Episode 01.mkv',
  container: 'matroska',
  sizeBytes: 1_500_000,
  duration: 1420,
  bitrate: 4_000_000,
  videoCodec: 'hevc',
  width: 1920,
  height: 1080,
  hdr: false,
  audioTracks: 1,
  subtitleTracks: 2,
  chapters: 0,
  videoStreams: [],
  audioStreams: [{ index: 1, type: 'audio', codec: 'aac', codecLong: 'AAC', profile: '', language: 'ja', title: '', duration: 1420, bitrate: 192000, default: true, forced: false, comment: false, hearingImpaired: false, channels: 2 }],
  subtitleStreams: [
    { index: 2, type: 'subtitle', codec: 'ass', codecLong: 'ASS', profile: '', language: 'ja', title: '', duration: 1420, bitrate: 0, default: true, forced: false, comment: false, hearingImpaired: false },
    { index: 3, type: 'subtitle', codec: 'srt', codecLong: 'SubRip', profile: '', language: 'en', title: '', duration: 1420, bitrate: 0, default: false, forced: false, comment: false, hearingImpaired: false },
  ],
  attachmentStreams: [],
  attachmentInventoryAvailable: true,
  compatibilityAnalysis: { overall: 'client_dependent' },
} as unknown as ScanResult;

describe('Assets workspace presentation', () => {
  it('renders compact canonical inventory summaries', () => {
    render(<AssetsSummaryStrip total={220} processing={3} completed={140} needsReview={4} />);

    expect(screen.getByRole('region', { name: 'Asset summary' })).toBeTruthy();
    expect(screen.getByLabelText('Total Assets: 220').textContent).toBe('220');
    expect(screen.getByLabelText('Processing: 3').textContent).toBe('3');
    expect(screen.getByLabelText('Completed: 140').textContent).toBe('140');
    expect(screen.getByLabelText('Needs Review: 4').textContent).toBe('4');
  });

  it('composes RAW and configured Library roots in one accessible tree while retaining separate projections', async () => {
    const user = userEvent.setup();
    const group = makeGroup();
    const tree = buildLibraryTree([group], [makeLibrary()]);
    expect(tree[0].label).toBe('Anime');
    expect(tree[0].children[0].label).toBe('Series');
    expect(tree[0].children[0].children[0].label).toBe('Season 01');
    const sourceGroups = [{
      id: 8,
      name: 'RAW Anime',
      relativePath: 'anime',
      sourcePath: '/raw/anime',
      logicalGroups: [{ id: '/raw/anime/Attack', name: 'Attack', assetPaths: [] }],
    }] as unknown as AssetSourceGroup[];
    expect(buildRawHierarchy(sourceGroups)[0].selection).toEqual({ kind: 'source', sourceGroupId: 8 });

    const onSelectRaw = vi.fn();
    const onSelectLibrary = vi.fn();
    const libraries = [makeLibrary(), { ...makeLibrary(), id: 2, name: 'Movies', destinationPath: '/library/Movies', type: 'movies' }];
    render(<AssetsHierarchyPanel
      sourceGroups={sourceGroups}
      groups={[group]}
      libraries={libraries}
      location="raw"
      rawSelection={{ kind: 'source', sourceGroupId: 8 }}
      librarySelection={{ libraryId: null, path: '' }}
      onSelectRaw={onSelectRaw}
      onSelectLibrary={onSelectLibrary}
    />);
    const hierarchy = screen.getByRole('tree', { name: 'Assets hierarchy' });
    const topLevelItems = within(hierarchy).getAllByRole('treeitem').filter((item) => item.getAttribute('aria-level') === '1');
    expect(topLevelItems[0].getAttribute('aria-label')).toBe('RAW source inventory');
    expect(topLevelItems.slice(1).map((item) => item.textContent?.trim())).toEqual(['Anime', 'Movies']);
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Converted' })).toBeNull();
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Archive' })).toBeNull();
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Reports' })).toBeNull();
    expect(within(hierarchy).getAllByRole('treeitem').find((item) => item.getAttribute('aria-level') === '2')?.getAttribute('aria-selected')).toBe('true');

    (topLevelItems[1] as HTMLElement).focus();
    await user.keyboard('{ArrowRight}');
    expect(topLevelItems[1].getAttribute('aria-expanded')).toBe('true');
    const seriesNode = within(hierarchy).getByRole('treeitem', { name: 'Series' });
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(seriesNode);
    await user.keyboard('{ArrowUp}');
    expect(document.activeElement).toBe(topLevelItems[1]);
    await user.keyboard('{Enter}');
    expect(onSelectLibrary).toHaveBeenCalledWith({ libraryId: 1, path: '' });
    await user.click(topLevelItems[0]);
    expect(onSelectRaw).toHaveBeenCalledWith({ kind: 'all' });

    expect(screen.queryByRole('img')).toBeNull();
  });

  it('shows compact technical details with one integrated preview Play control', async () => {
    const user = userEvent.setup();
    const play = vi.fn();
    const assign = vi.fn();
    const testEncode = vi.fn();
    const more = vi.fn();
    const asset = makeAsset();
    render(
      <SelectedAssetDetail
        asset={asset}
        pathContext="Anime · Series/Season 01"
        status="Ready"
        assignedProfile="HEVC Main"
        snapshot={scan}
        snapshotLoading={false}
        subtitlesSummary="2 tracks · ja, en"
        modifiedLabel="Sep 30, 2026"
        sizeLabel="1.5 MB"
        onPlayPreview={play}
        onAssignProfile={assign}
        onTestEncode={testEncode}
        onMore={(anchor) => more(anchor)}
        canAssignProfile
        canTestEncode
      />,
    );

    expect(screen.getByText('Video · hevc')).toBeTruthy();
    expect(screen.getByText('Container · MATROSKA')).toBeTruthy();
    expect(screen.getByText('Resolution · 1920 × 1080')).toBeTruthy();
    expect(screen.getByText('Audio · aac 2ch ja')).toBeTruthy();
    expect(screen.getByText('Subtitles')).toBeTruthy();
    expect(screen.getByText('2 tracks · ja, en')).toBeTruthy();
    expect(screen.getAllByRole('button', { name: /Play preview of Episode 01.mkv/ })).toHaveLength(1);
    expect((screen.getByRole('img', { name: 'Episode 01.mkv preview' }) as HTMLImageElement).getAttribute('src')).toContain('/api/assets/preview?path=');
    expect(screen.queryByRole('button', { name: /View.*Play|Play.*View/ })).toBeNull();

    await user.click(screen.getByRole('button', { name: /Play preview of Episode 01.mkv/ }));
    await user.click(screen.getByRole('button', { name: 'Assign Profile' }));
    await user.click(screen.getByRole('button', { name: 'Test Encode' }));
    fireEvent.click(screen.getByRole('button', { name: 'More actions for Episode 01.mkv' }));
    expect(play).toHaveBeenCalledOnce();
    expect(assign).toHaveBeenCalledOnce();
    expect(testEncode).toHaveBeenCalledOnce();
    expect(more).toHaveBeenCalledOnce();
  });

  it('omits empty descriptive metadata and renders canonical tags when present', () => {
    const asset = makeAsset();
    const base = {
      asset, pathContext: 'Anime · Series', status: 'Ready', assignedProfile: 'Inherited', snapshot: scan, snapshotLoading: false,
      subtitlesSummary: '2 tracks', modifiedLabel: 'Sep 30, 2026', sizeLabel: '1.5 MB', onPlayPreview: vi.fn(), onAssignProfile: vi.fn(),
      onTestEncode: vi.fn(), onMore: vi.fn(), canAssignProfile: true, canTestEncode: true,
    };
    const view = render(<SelectedAssetDetail {...base} />);
    expect(screen.queryByText(/^Metadata ·/)).toBeNull();

    view.rerender(<SelectedAssetDetail {...base} asset={makeAsset({ metadata: { categories: ['Anime'], tags: ['TV'] , updatedAt: '' } })} />);
    expect(screen.getByText('Metadata · Anime, TV')).toBeTruthy();
  });
});
