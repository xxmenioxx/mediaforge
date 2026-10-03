// @vitest-environment jsdom

import { cleanup, fireEvent, render as rtlRender, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ComponentProps, ReactNode } from 'react';
import type { Asset, AssetGroup, AssetSourceGroup, Library, ScanResult } from '../api/types';
import {
  ASSETS_HIERARCHY_DEFAULT_WIDTH,
  ASSETS_HIERARCHY_MAX_WIDTH,
  ASSETS_HIERARCHY_MIN_WIDTH,
  ASSETS_HIERARCHY_WIDTH_STORAGE_KEY,
  AssetsHierarchyPanel,
  AssetsSummaryStrip,
  SelectedAssetArtwork,
  SelectedAssetDetail,
} from './AssetsWorkspace';
import { buildLibraryTree, buildRawHierarchy } from './assetsWorkspaceModel';

afterEach(cleanup);

function render(ui: ReactNode) {
  return rtlRender(ui, { wrapper: MemoryRouter });
}

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

function selectedAssetDetailProps(overrides: Partial<ComponentProps<typeof SelectedAssetDetail>> = {}): ComponentProps<typeof SelectedAssetDetail> {
  return {
    asset: makeAsset(),
    pathContext: 'Series/Season 01',
    categoryLabel: 'Anime',
    containerLabel: 'matroska',
    snapshot: scan,
    snapshotLoading: false,
    statusLabel: 'Library',
    statusColor: 'default',
    assignedProfileLabel: 'Anime Main V2',
    modifiedLabel: 'Sep 30, 2026',
    sizeLabel: '1.5 MB',
    assignProfileUnavailableReason: 'Profile assignment is unavailable for this asset in its current Library lifecycle.',
    testEncodeUnavailableReason: 'Test Encode is unavailable for this asset in its current lifecycle.',
    onPlayPreview: vi.fn(),
    onAssignProfile: vi.fn(),
    onTestEncode: vi.fn(),
    onMore: vi.fn(),
    canAssignProfile: true,
    canTestEncode: true,
    ...overrides,
  };
}

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
    const hierarchy = screen.getByRole('tree', { name: 'Asset Library' });
    const topLevelItems = within(hierarchy).getAllByRole('treeitem').filter((item) => item.getAttribute('aria-level') === '1');
    expect(topLevelItems.map((item) => item.getAttribute('aria-label'))).toEqual(['RAW', 'Libraries']);
    const libraryGroup = topLevelItems[1]!.parentElement?.querySelector<HTMLElement>('[role="group"]');
    const libraryRoots = within(libraryGroup!).getAllByRole('treeitem').filter((item) => item.getAttribute('aria-level') === '2');
    expect(libraryRoots.map((item) => item.getAttribute('aria-label'))).toEqual(['Anime', 'Movies']);
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Converted' })).toBeNull();
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Archive' })).toBeNull();
    expect(within(hierarchy).queryByRole('treeitem', { name: 'Reports' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Configure libraries' }).getAttribute('href')).toBe('/libraries');
    expect(within(hierarchy).getAllByRole('treeitem').find((item) => item.getAttribute('aria-level') === '2')?.getAttribute('aria-selected')).toBe('true');

    (topLevelItems[1] as HTMLElement).focus();
    await user.keyboard('{ArrowRight}');
    expect(topLevelItems[1].getAttribute('aria-expanded')).toBe('true');
    const libraryRoot = within(libraryGroup!).getByRole('treeitem', { name: 'Anime' });
    expect(document.activeElement).toBe(libraryRoot);
    await user.keyboard('{ArrowRight}');
    const seriesNode = await within(hierarchy).findByRole('treeitem', { name: 'Series' });
    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(seriesNode);
    await user.keyboard('{ArrowUp}');
    expect(document.activeElement).toBe(libraryRoot);
    await user.keyboard('{Enter}');
    expect(onSelectLibrary).toHaveBeenCalledWith({ libraryId: 1, path: '' });
    await user.keyboard(' ');
    expect(onSelectLibrary).toHaveBeenLastCalledWith({ libraryId: 1, path: '' });
    await user.click(topLevelItems[0]);
    expect(onSelectRaw).toHaveBeenCalledWith({ kind: 'all' });

    expect(screen.queryByRole('img')).toBeNull();
  });

  it('moves ArrowLeft from a nested Library child to its immediate parent', async () => {
    const user = userEvent.setup();
    render(<AssetsHierarchyPanel
      sourceGroups={[]}
      groups={[makeGroup()]}
      libraries={[makeLibrary()]}
      location="library"
      rawSelection={null}
      librarySelection={{ libraryId: 1, path: '' }}
      onSelectRaw={vi.fn()}
      onSelectLibrary={vi.fn()}
    />);

    const libraryRoot = screen.getByRole('treeitem', { name: 'Anime' });
    libraryRoot.focus();
    await user.keyboard('{ArrowRight}');
    const seriesNode = screen.getByRole('treeitem', { name: 'Series' });
    seriesNode.focus();
    await user.keyboard('{ArrowLeft}');

    expect(document.activeElement).toBe(libraryRoot);
  });

  it('moves ArrowLeft through RAW path parents and collapses an expanded node first', async () => {
    const user = userEvent.setup();
    const sourceGroups = [{
      id: 8,
      name: 'RAW Anime',
      relativePath: 'anime',
      sourcePath: '/raw/anime',
      logicalGroups: [{
        id: '/raw/anime/Attack',
        name: 'Attack',
        assetPaths: [{
          id: 'extras-path',
          name: 'extras',
          path: '/raw/anime/Attack/extras',
          relativePath: 'extras',
          displayPath: 'extras',
          isLogicalGroupRoot: false,
          fileCount: 1,
          assetCount: 1,
          sizeBytes: 1,
          totalSizeBytes: 1,
          assets: [],
        }],
      }],
    }] as unknown as AssetSourceGroup[];
    render(<AssetsHierarchyPanel
      sourceGroups={sourceGroups}
      groups={[]}
      libraries={[]}
      location="raw"
      rawSelection={{ kind: 'all' }}
      librarySelection={{ libraryId: null, path: '' }}
      onSelectRaw={vi.fn()}
      onSelectLibrary={vi.fn()}
    />);

    await user.click(screen.getByRole('button', { name: 'Expand RAW' }));
    const sourceNode = await screen.findByRole('treeitem', { name: 'RAW Anime' });
    sourceNode.focus();
    await user.keyboard('{ArrowRight}');
    const logicalGroupNode = screen.getByRole('treeitem', { name: 'Attack' });
    logicalGroupNode.focus();
    await user.keyboard('{ArrowRight}');
    const pathNode = screen.getByRole('treeitem', { name: 'extras' });

    pathNode.focus();
    await user.keyboard('{ArrowLeft}');
    expect(document.activeElement).toBe(logicalGroupNode);

    await user.keyboard('{ArrowLeft}');
    expect(logicalGroupNode.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(logicalGroupNode);

    await user.keyboard('{ArrowLeft}');
    expect(document.activeElement).toBe(sourceNode);
  });

  it('resizes the hierarchy with pointer and keyboard controls and persists the chosen width', () => {
    window.localStorage.removeItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY);
    const props = {
      sourceGroups: [] as AssetSourceGroup[],
      groups: [] as AssetGroup[],
      libraries: [] as Library[],
      location: 'library' as const,
      rawSelection: { kind: 'all' as const },
      librarySelection: { libraryId: null, path: '' },
      onSelectRaw: vi.fn(),
      onSelectLibrary: vi.fn(),
    };
    const view = render(<AssetsHierarchyPanel {...props} />);
    const separator = screen.getByRole('separator', { name: 'Resize Assets hierarchy' });
    expect(separator.getAttribute('aria-valuenow')).toBe(String(ASSETS_HIERARCHY_DEFAULT_WIDTH));
    expect(separator.getAttribute('aria-valuemin')).toBe(String(ASSETS_HIERARCHY_MIN_WIDTH));
    expect(separator.getAttribute('aria-valuemax')).toBe(String(ASSETS_HIERARCHY_MAX_WIDTH));

    fireEvent.pointerDown(separator, { button: 0, clientX: 100, pointerId: 1 });
    fireEvent.pointerMove(separator, { clientX: 164, pointerId: 1 });
    fireEvent.pointerUp(separator, { clientX: 164, pointerId: 1 });
    expect(separator.getAttribute('aria-valuenow')).toBe('326');
    expect(window.localStorage.getItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY)).toBe('326');

    fireEvent.keyDown(separator, { key: 'End' });
    expect(separator.getAttribute('aria-valuenow')).toBe(String(ASSETS_HIERARCHY_MAX_WIDTH));
    expect(window.localStorage.getItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY)).toBe(String(ASSETS_HIERARCHY_MAX_WIDTH));
    view.unmount();
    render(<AssetsHierarchyPanel {...props} />);
    expect(screen.getByRole('separator', { name: 'Resize Assets hierarchy' }).getAttribute('aria-valuenow')).toBe(String(ASSETS_HIERARCHY_MAX_WIDTH));
    cleanup();
    window.localStorage.setItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY, '999');
    const clamped = render(<AssetsHierarchyPanel {...props} />);
    expect(screen.getByRole('separator', { name: 'Resize Assets hierarchy' }).getAttribute('aria-valuenow')).toBe(String(ASSETS_HIERARCHY_MAX_WIDTH));
    clamped.unmount();
    window.localStorage.setItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY, 'not-a-width');
    render(<AssetsHierarchyPanel {...props} />);
    expect(screen.getByRole('separator', { name: 'Resize Assets hierarchy' }).getAttribute('aria-valuenow')).toBe(String(ASSETS_HIERARCHY_DEFAULT_WIDTH));
  });

  it('uses the generic artwork placeholder when no supported artwork URL is available', () => {
    render(<SelectedAssetArtwork fileName="Episode 01.mkv" />);

    const placeholder = screen.getByRole('img', { name: 'MVForge artwork placeholder for Episode 01.mkv' });
    expect(placeholder.getAttribute('data-artwork-source')).toBe('placeholder');
    expect(screen.queryByRole('img', { name: 'Episode 01.mkv artwork' })).toBeNull();
  });

  it('keeps required status, profile, subtitle, audio, technical, and action details in the compact Figma composition', async () => {
    const user = userEvent.setup();
    const play = vi.fn();
    const assignProfile = vi.fn();
    const testEncode = vi.fn();
    const more = vi.fn();
    render(
      <SelectedAssetDetail
        {...selectedAssetDetailProps({ onAssignProfile: assignProfile, onTestEncode: testEncode, onMore: more })}
        onPlayPreview={play}
      />,
    );

    const details = screen.getByTestId('selected-asset-detail');
    const preview = screen.getByTestId('selected-asset-preview');
    const identity = screen.getByTestId('selected-asset-identity');
    const information = screen.getByTestId('selected-asset-information');
    const media = within(screen.getByTestId('selected-asset-technical'));
    expect(getComputedStyle(details).position).toBe('sticky');
    expect(getComputedStyle(details).bottom).toBe('0px');
    expect(getComputedStyle(preview).aspectRatio.replace(/\s/g, '')).toBe('4/3');
    expect(identity.textContent).toContain('Episode 01.mkv');
    expect(identity.textContent).toContain('Series/Season 01');
    expect(identity.contains(screen.getByTestId('selected-asset-state-context'))).toBe(false);
    expect(identity.contains(screen.getByTestId('selected-asset-technical'))).toBe(false);
    expect(information.contains(identity)).toBe(true);
    expect(information.contains(screen.getByTestId('selected-asset-quick-facts'))).toBe(true);
    expect(information.contains(screen.getByTestId('selected-asset-state-context'))).toBe(true);
    expect(information.contains(screen.getByTestId('selected-asset-technical'))).toBe(true);
    const quickFacts = within(screen.getByTestId('selected-asset-quick-facts'));
    expect(quickFacts.getByText('Category / Library')).toBeTruthy();
    expect(quickFacts.getByText('Anime')).toBeTruthy();
    expect(quickFacts.getByText('matroska')).toBeTruthy();
    const context = within(screen.getByTestId('selected-asset-state-context'));
    expect(context.getByText('Profile')).toBeTruthy();
    expect(context.getByText('Anime Main V2')).toBeTruthy();
    expect(context.getByText('Subtitles')).toBeTruthy();
    expect(context.getByText('2 embedded tracks')).toBeTruthy();
    expect(context.getByText('Duration')).toBeTruthy();
    expect(context.getByText('Modified')).toBeTruthy();
    expect(within(details).getByTestId('selected-asset-status').getAttribute('aria-label')).toBe('Status: Library');
    expect(quickFacts.getByText('Size')).toBeTruthy();
    expect(quickFacts.getByText('Resolution')).toBeTruthy();
    expect(quickFacts.getByText('Codec')).toBeTruthy();
    expect(quickFacts.getByText('1920 × 1080')).toBeTruthy();
    expect(quickFacts.getByText('hevc')).toBeTruthy();
    expect(media.getByText('MEDIA')).toBeTruthy();
    expect(media.getByText('Video')).toBeTruthy();
    expect(media.getByText('hevc · 1920 × 1080')).toBeTruthy();
    expect(within(details).getAllByText('1920 × 1080')).toHaveLength(1);
    expect(media.getByText('Encoder')).toBeTruthy();
    expect(media.getAllByText('Unavailable').length).toBeGreaterThan(0);
    expect(media.getByText('Duration')).toBeTruthy();
    expect(media.getByText('Bitrate')).toBeTruthy();
    expect(media.getByText('4.00 Mbps')).toBeTruthy();
    expect(media.getByText('Range')).toBeTruthy();
    expect(media.getByText('SDR')).toBeTruthy();
    expect(media.getByText('Size')).toBeTruthy();
    expect(media.getByText('Modified')).toBeTruthy();
    expect(media.getByText('Audio')).toBeTruthy();
    expect(media.getByText('aac 2 ch')).toBeTruthy();
    const actions = within(screen.getByTestId('selected-asset-technical')).getByRole('group', { name: 'Selected asset actions' });
    expect(within(details).getByRole('button', { name: 'Assign Profile' })).toBeTruthy();
    expect(within(details).getByRole('button', { name: 'Test Encode' })).toBeTruthy();
    const moreButton = within(details).getByRole('button', { name: 'More actions for Episode 01.mkv' });
    expect(within(details).queryByRole('img', { name: 'MVForge artwork placeholder for Episode 01.mkv' })).toBeTruthy();
    expect(within(details).getAllByText('23m 40s')).toHaveLength(3);
    expect(actions.contains(within(details).getByRole('button', { name: 'Assign Profile' }))).toBe(true);
    expect(actions.contains(moreButton)).toBe(true);

    await user.click(screen.getByRole('button', { name: 'Play preview of Episode 01.mkv' }));
    await user.click(within(details).getByRole('button', { name: 'Assign Profile' }));
    await user.click(within(details).getByRole('button', { name: 'Test Encode' }));
    await user.click(moreButton);
    expect(play).toHaveBeenCalledOnce();
    expect(assignProfile).toHaveBeenCalledOnce();
    expect(testEncode).toHaveBeenCalledOnce();
    expect(more).toHaveBeenCalledWith(moreButton);
  });

  it('collapses and expands selected details with an accessible keyboard control', async () => {
    const user = userEvent.setup();
    render(
      <SelectedAssetDetail {...selectedAssetDetailProps()} />,
    );

    const details = screen.getByTestId('selected-asset-detail');
    await user.click(screen.getByRole('button', { name: 'Collapse selected asset details for Episode 01.mkv' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: /Play preview/ })).toBeNull());
    expect(within(details).getByText('Episode 01.mkv')).toBeTruthy();
    const expand = screen.getByRole('button', { name: 'Expand selected asset details for Episode 01.mkv' });
    expand.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByRole('button', { name: /Play preview/ })).toBeTruthy());
  });

  it('keeps unavailable detail metadata explicit instead of fabricating it', () => {
    render(
      <SelectedAssetDetail {...selectedAssetDetailProps({
        asset: makeAsset({ metadata: { categories: [], tags: [], updatedAt: '' } }),
        pathContext: 'Series/Season 01',
        categoryLabel: 'Unavailable',
        containerLabel: 'Unavailable',
        snapshot: null,
        assignedProfileLabel: 'Inherited',
        snapshotLoading: false,
        canAssignProfile: false,
        canTestEncode: false,
      })} />,
    );
    expect(screen.getByTestId('selected-asset-path-context').textContent).toBe('Series/Season 01');
    const facts = within(screen.getByTestId('selected-asset-quick-facts'));
    expect(facts.getAllByText('Unavailable')).toHaveLength(2);
    const context = within(screen.getByTestId('selected-asset-state-context'));
    expect(context.getByText('Inherited')).toBeTruthy();
    expect(context.getAllByText('Unavailable')).toHaveLength(1);
    const media = within(screen.getByTestId('selected-asset-technical'));
    expect(media.getByText('Range')).toBeTruthy();
    expect(media.getByText('Bitrate')).toBeTruthy();
    expect(media.getByText('4.00 Mbps')).toBeTruthy();
    expect(media.getAllByText('Unavailable').length).toBeGreaterThan(0);
    expect(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'Assign Profile' }).hasAttribute('disabled')).toBe(true);
    expect(within(screen.getByTestId('selected-asset-detail')).getByRole('button', { name: 'Test Encode' }).hasAttribute('disabled')).toBe(true);
    expect(screen.getByLabelText('Profile assignment is unavailable for this asset in its current Library lifecycle.').getAttribute('tabindex')).toBe('0');
    expect(screen.getByLabelText('Test Encode is unavailable for this asset in its current lifecycle.').getAttribute('tabindex')).toBe('0');
  });

  it('preserves known zero dimensions without inventing unsupported technical fields', () => {
    render(<SelectedAssetDetail {...selectedAssetDetailProps({
      asset: makeAsset({ technical: { videoCodec: '', width: 0, height: 0, duration: 0, bitrate: 0, hdr: undefined as unknown as boolean } }),
      snapshot: null,
      snapshotLoading: false,
      categoryLabel: 'Unavailable',
      containerLabel: 'Unavailable',
      modifiedLabel: 'Unavailable',
      sizeLabel: '0 B',
    })} />);

    const facts = within(screen.getByTestId('selected-asset-quick-facts'));
    const media = within(screen.getByTestId('selected-asset-technical'));
    expect(facts.getByText('0 × 0')).toBeTruthy();
    expect(media.getByText('0 × 0')).toBeTruthy();
    expect(media.getByText('Bitrate')).toBeTruthy();
    expect(media.getByText('0 bps')).toBeTruthy();
    expect(media.getByText('Range')).toBeTruthy();
    expect(media.getAllByText('Unavailable').length).toBeGreaterThan(0);
    expect(media.queryByText('SDR')).toBeNull();
  });

});
