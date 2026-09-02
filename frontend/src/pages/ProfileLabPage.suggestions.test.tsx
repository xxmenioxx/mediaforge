// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, ProfileSuggestion, ScanResult } from '../api/types';

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
      runtimeSnapshot: vi.fn(),
      workerNodes: vi.fn(),
      latestSnapshot: vi.fn(),
      suggestProfile: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { ProfileLabPage } from './ProfileLabPage';

const assetPath = '/media/raw/movies/Test/Test.mkv';

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
} satisfies Asset;

const scan = {
  id: 1,
  path: assetPath,
  fileName: 'Test.mkv',
  container: 'matroska',
  sizeBytes: 1024,
  duration: 120,
  bitrate: 4_000_000,
  videoCodec: 'mpeg2video',
  width: 720,
  height: 480,
  hdr: false,
  audioTracks: 0,
  subtitleTracks: 0,
  chapters: 0,
  videoStreams: [],
  audioStreams: [],
  subtitleStreams: [],
  compatibilityAnalysis: { warnings: [] },
  interlaceAnalysis: { status: 'progressive', confidence: 0.95 },
  cropAnalysis: { status: 'none' },
  rawProbe: {},
} as unknown as ScanResult;

const nullableSuggestion = {
  matchType: 'create',
  summary: 'A conservative restoration profile is suggested.',
  scan,
  candidates: [],
  proposedProfile: { name: 'Suggested', videoCodec: 'x265', workerConfig: {} },
  insights: {
    recommendedCrf: 18,
    estimatedMinBytes: 100,
    estimatedMaxBytes: 200,
    estimatedSavingsLow: 10,
    estimatedSavingsHigh: 20,
    recommendations: ['Review the source evidence before applying changes.'],
  },
  findings: [],
  restorationPlan: {
    version: 1,
    applyLocked: false,
    recommendations: [
      {
        id: 'deblock',
        domain: 'Deblock',
        state: 'manual_review',
        currentValue: 'off',
        recommendedValue: '',
        confidence: 'medium',
        reasons: null,
        warnings: null,
        supportingEvidence: null,
      },
      {
        id: 'upscale',
        domain: 'Smart Upscale',
        state: 'recommended',
        currentValue: 'disabled',
        recommendedValue: '720p',
        confidence: 'high',
        reasons: ['Reliable progressive SD evidence supports a conservative 720p target.'],
        warnings: [],
        supportingEvidence: [],
        patch: { upscaleMode: 'auto' },
      },
    ],
    restorationEvidence: {
      version: 1,
      status: 'unavailable',
      source: 'sampled_ffmpeg_metrics',
      windows: 0,
      sampledFrames: 0,
    },
  },
} as unknown as ProfileSuggestion;

afterEach(cleanup);

describe('Profile Lab Process Asset suggestions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const inventory = {
      sourceGroups: [], unprocessed: [asset], library: [], converted: [], unverified: [], accepted: [], archive: [], missing: [],
      unprocessedGroups: [], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], archiveGroups: [],
      reports: {}, sync: {},
    } as unknown as AssetInventory;
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.profilesAdmin).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.runtimeSnapshot).mockResolvedValue({ encoders: {} } as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: scan, status: 'current', requiresAnalysis: false, staleComponents: [] });
    vi.mocked(api.suggestProfile).mockResolvedValue(nullableSuggestion);
  });

  it('keeps Lab mounted and opens Suggestions when legacy collection fields are null', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processAsset = await screen.findByRole('button', { name: 'Process Asset' });
    await waitFor(() => expect((processAsset as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processAsset);

    expect(await screen.findByRole('dialog', { name: 'MVForge Suggestions' })).toBeTruthy();
    expect(screen.getByText('Profile Lab')).toBeTruthy();
    expect(screen.getByText('Current: Off')).toBeTruthy();
    expect(screen.getByText('Recommended: Manual review')).toBeTruthy();
    expect(screen.getByText('confidence Medium')).toBeTruthy();
    expect(screen.getByText('Reason: Reliable progressive SD evidence supports a conservative 720p target.')).toBeTruthy();
    expect(api.suggestProfile).toHaveBeenCalledWith(assetPath);
  }, 15_000);
});
