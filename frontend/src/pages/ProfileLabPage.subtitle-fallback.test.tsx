// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      assets: vi.fn(),
      profiles: vi.fn(),
      profilesAdmin: vi.fn(),
      settings: vi.fn(),
      libraries: vi.fn(),
      testEncodes: vi.fn().mockResolvedValue([]),
      runtimeState: vi.fn(),
      workerNodes: vi.fn(),
      latestSnapshot: vi.fn(),
      resolveTrackProfilePreview: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { ProfileLabPage } from './ProfileLabPage';

const assetPath = '/media/raw/movies/Test/Test.mkv';
const fallbackNotice = 'ASS + SRT compatibility fallback';

const asset = {
  id: 1,
  libraryId: 0,
  libraryName: 'Originals',
  path: assetPath,
  relativePath: 'movies/Test/Test.mkv',
  groupPath: 'movies/Test',
  fileName: 'Test.mkv',
  extension: '.mkv',
  sizeBytes: 1024,
  modifiedAt: '2026-09-01T00:00:00Z',
  status: 'unprocessed',
  missing: false,
  review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
  metadata: { categories: [], tags: [], updatedAt: '' },
  conversion: {},
};

const snapshot = {
  id: 1,
  path: assetPath,
  fileName: 'Test.mkv',
  container: 'matroska',
  sizeBytes: 1024,
  duration: 120,
  videoCodec: 'mpeg2video',
  width: 720,
  height: 480,
  audioTracks: 0,
  subtitleTracks: 1,
  chapters: 0,
  videoStreams: [],
  audioStreams: [],
  subtitleStreams: [],
  compatibilityAnalysis: { warnings: [] },
  interlaceAnalysis: { status: 'progressive', confidence: 0.95 },
  cropAnalysis: { status: 'none' },
  rawProbe: {},
};

function renderLab() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/lab?section=tracks&trackProfileKey=path-subtitles&assetPath=${encodeURIComponent(assetPath)}`]}>
        <ProfileLabPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function mockLabAPI(generateSRTFallbackForASS: boolean) {
  vi.mocked(api.assets).mockResolvedValue({
    sourceGroups: [],
    unprocessed: [asset],
    library: [],
    converted: [],
    unverified: [],
    accepted: [],
    missing: [],
    unprocessedGroups: [],
    libraryGroups: [],
    convertedGroups: [],
    unverifiedGroups: [],
    acceptedGroups: [],
    reports: {},
    sync: {},
  } as never);
  vi.mocked(api.profiles).mockResolvedValue([]);
  vi.mocked(api.profilesAdmin).mockResolvedValue([]);
  vi.mocked(api.settings).mockResolvedValue([
    {
      key: 'subtitleSettings',
      value: { generateSRTFallbackForASS },
    },
    {
      key: 'trackProfiles',
      value: {
        profiles: [{
          key: 'path-subtitles',
          name: 'Path subtitles',
          scope: 'path',
          subtitleRules: [{ language: 'spa', action: 'extract', sidecarFormats: ['ass'] }],
        }],
      },
    },
  ] as never);
  vi.mocked(api.libraries).mockResolvedValue([]);
  vi.mocked(api.runtimeState).mockResolvedValue({ encoders: {} } as never);
  vi.mocked(api.workerNodes).mockResolvedValue([]);
  vi.mocked(api.latestSnapshot).mockResolvedValue({
    found: true,
    snapshot: snapshot as never,
    status: 'current',
    requiresAnalysis: false,
    staleComponents: [],
  });
  vi.mocked(api.resolveTrackProfilePreview).mockResolvedValue(undefined as never);
}

afterEach(cleanup);

describe('Profile Lab subtitle fallback visibility', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    HTMLElement.prototype.scrollIntoView = vi.fn();
  });

  it.each([
    { enabled: true, visible: true },
    { enabled: false, visible: false },
  ])('renders the language-rule fallback notice when enabled=$enabled', async ({ enabled, visible }) => {
    mockLabAPI(enabled);
    renderLab();

    if (visible) {
      expect(await screen.findByText(fallbackNotice)).toBeTruthy();
    } else {
      await screen.findByDisplayValue('ass');
      expect(screen.queryByText(fallbackNotice)).toBeNull();
    }
  });
});
