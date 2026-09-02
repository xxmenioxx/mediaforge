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
      createCompatiblePreviewRequest: vi.fn(),
      inspectCompatibleAssetPreview: vi.fn(),
      compatibleAssetFrameMetrics: vi.fn(),
      recommendEncoderQuality: vi.fn(),
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

const largeSavedProfile = {
  id: 77,
  name: 'Large restoration draft',
  scope: 'asset',
  videoCodec: 'x265',
  qualityValue: 18,
  workerConfig: {
    videoFilters: 'deblock=filter=strong:block=8,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024,exposure=exposure=0.12,eq=brightness=0:contrast=1:saturation=0.96:gamma=0.94',
    upscaleMode: 'auto', upscaleSharpen: 'custom', upscaleSharpenCustomStrength: 0.16,
    fieldStructureMode: 'deinterlace', deinterlaceFieldOrder: 'bff',
    restorationRecommendationProvenance: 'evidence,'.repeat(2_000),
  },
} as never;

const frameStructure = {
  framesAnalyzed: 120, iFrames: 2, pFrames: 80, bFrames: 38, bFrameRatio: 0.316,
  averageGopLength: 60, minimumGopLength: 60, maximumGopLength: 60, maxConsecutiveBFrames: 2,
  confidence: 'high', variability: 'low', windowCount: 3,
};

const previewInspection = {
  source: { codec: 'mpeg2video', width: 720, height: 480, frameRate: '30000/1001', fieldOrder: 'progressive' },
  output: { codec: 'hevc', width: 960, height: 720, frameRate: '30000/1001', fieldOrder: 'progressive', level: 93 },
  sourceFrameStructure: frameStructure,
  outputFrameStructure: frameStructure,
  qsvFrameWarnings: [], qsvFeatureStatus: {}, cacheHit: false, previewMode: 'quality', start: '0', seconds: 20,
  generatedPath: '/tmp/preview.mp4', normalization: { mode: 'normalize_bt709', applied: true, reason: 'test', sarPreserved: true },
  requestedEncoder: 'libx265', effectiveEncoder: 'libx265', requestedQSVRateControl: '', effectiveQSVRateControl: '',
  ffmpegArgs: ['-vf', 'scale=960:720'],
} as never;

afterEach(cleanup);

describe('Profile Lab Process Asset suggestions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    HTMLElement.prototype.scrollIntoView = vi.fn();
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
    vi.mocked(api.createCompatiblePreviewRequest).mockImplementation(async (options) => ({ requestId: `${String(options.start ?? 'base').replaceAll(':', '')}-${JSON.stringify(options.profile ?? {}).length}`, cacheIdentity: 'cache', expiresInSeconds: 7200, path: options.path }));
    vi.mocked(api.inspectCompatibleAssetPreview).mockResolvedValue(previewInspection);
    vi.mocked(api.compatibleAssetFrameMetrics).mockResolvedValue({ comparable: true, reason: '', sourceDimensions: '960x720', outputDimensions: '960x720', ssim: 0.99, psnr: 42 });
    vi.mocked(api.recommendEncoderQuality).mockResolvedValue(undefined as never);
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

  it('processes a large video draft through short compatible-preview request IDs', async () => {
    vi.mocked(api.profiles).mockResolvedValue([largeSavedProfile]);
    vi.mocked(api.profilesAdmin).mockResolvedValue([largeSavedProfile]);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&videoProfileId=77&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);

    await waitFor(() => expect(api.inspectCompatibleAssetPreview).toHaveBeenCalled(), { timeout: 12_000 });
    const requests = vi.mocked(api.createCompatiblePreviewRequest).mock.calls.map(([options]) => options);
    const conversion = requests.find((options) => options.profile);
    expect(conversion?.profile?.workerConfig?.upscaleSharpenCustomStrength).toBe(0.16);
    expect(String(conversion?.profile?.workerConfig?.restorationRecommendationProvenance).length).toBeGreaterThan(16_000);
    expect(vi.mocked(api.inspectCompatibleAssetPreview).mock.calls.every(([requestId]) => typeof requestId === 'string' && !requestId.includes('profile='))).toBe(true);
    expect(await screen.findByText('Profile Lab')).toBeTruthy();
  }, 20_000);
});
