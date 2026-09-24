// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, PreviewInspection, ProfileSuggestion, ScanResult } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    waitForCompatiblePreviewReady: vi.fn().mockImplementation(async (requestId: string) => ({
      requestId,
      artifactId: `artifact-${requestId}`,
      generationToken: `generation-${requestId}`,
      generationVersion: 1,
      sourceRevision: { size: 1024, modifiedAtNS: 1, mode: 0o600 },
      start: '00:00:00',
      seconds: 20,
      normalization: { mode: 'normalize_bt709', applied: true, sarPreserved: true },
      state: 'ready',
      code: 'ready',
    })),
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
      compatiblePreviewStatus: vi.fn(),
      inspectCompatibleAssetPreview: vi.fn(),
      compatibleAssetFrameMetrics: vi.fn(),
      startLabFidelityOperation: vi.fn(),
      labFidelityOperation: vi.fn(),
      cancelLabFidelityOperation: vi.fn(),
      recommendEncoderQuality: vi.fn(),
    },
  };
});

import { ApiRequestError, api, waitForCompatiblePreviewReady } from '../api/client';
import { ProfileLabPage } from './ProfileLabPage';
import { FrameStructureControls } from '../components/FrameStructureControls';

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

const qsvSavedProfile = {
  id: 78,
  name: 'QSV RDO draft',
  scope: 'asset',
  videoCodec: 'x265',
  qualityValue: 20,
  workerConfig: {
    preferredEncoder: 'hardware',
    useHardwareIfAvailable: true,
    videoEncoder: 'hevc_qsv',
    hardwareQualityPreset: 'custom',
    pixFmt: 'p010le',
    qsvRateControl: 'icq',
    qsvMBBRCMode: 'auto',
    qsvRDOMode: 'auto',
  },
} as never;

const frameStructure = {
  framesAnalyzed: 120, iFrames: 2, pFrames: 80, bFrames: 38, bFrameRatio: 0.316,
  averageGopLength: 14.9, medianGopLength: 15, p25GopLength: 15, p75GopLength: 15,
  minimumGopLength: 15, maximumGopLength: 15, completeGops: 8, maxConsecutiveBFrames: 2,
  confidence: 'high', variability: 'low', windowCount: 3,
};

const frameStructureRecommendation = {
  version: 2,
  sourceAnalysisVersion: 2,
  fps: 24000 / 1001,
  sourceFps: 24000 / 1001,
  sourceAnchorFrames: 15,
  sourceAnchorSeconds: 15 / (24000 / 1001),
  autoStrategy: 'balanced',
  analysisDriven: true,
  recommendedMaxBFrames: 2,
  confidence: 'high',
  byMode: {
    compatible: { targetGopFrames: 15, targetGopSeconds: 15 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
    balanced: { targetGopFrames: 19, targetGopSeconds: 19 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
    maximum_compression: { targetGopFrames: 26, targetGopSeconds: 26 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [], warnings: [] },
  },
};

const previewInspection = {
  source: { codec: 'mpeg2video', width: 720, height: 480, frameRate: '30000/1001', fieldOrder: 'progressive' },
  output: { codec: 'hevc', width: 960, height: 720, frameRate: '30000/1001', fieldOrder: 'progressive', level: 93 },
  sourceFrameStructure: frameStructure,
  outputFrameStructure: frameStructure,
  qsvFrameWarnings: [], qsvFeatureStatus: {}, cacheHit: false, previewMode: 'quality', start: '0', seconds: 20,
  generatedPath: '/tmp/preview.mp4', normalization: { mode: 'normalize_bt709', applied: true, reason: 'test', sarPreserved: true },
  requestedEncoder: 'libx265', effectiveEncoder: 'libx265', requestedQSVRateControl: '', effectiveQSVRateControl: '',
  frameStructureRecommendation,
  ffmpegArgs: ['-vf', 'scale=960:720'],
} as unknown as PreviewInspection;

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
    vi.mocked(api.compatiblePreviewStatus).mockImplementation(async (requestId) => ({
      requestId,
      artifactId: `artifact-${requestId}`,
      generationToken: `generation-${requestId}`,
      generationVersion: 1,
      sourceRevision: { size: 1024, modifiedAtNS: 1, mode: 0o600 },
      start: '00:00:00',
      seconds: 20,
      normalization: { mode: 'normalize_bt709', applied: true, sarPreserved: true },
      state: 'ready',
      code: 'ready',
    }));
    vi.mocked(api.inspectCompatibleAssetPreview).mockResolvedValue(previewInspection);
    vi.mocked(api.compatibleAssetFrameMetrics).mockResolvedValue({ comparable: true, reason: '', sourceDimensions: '960x720', outputDimensions: '960x720', ssim: 0.99, psnr: 42 });
    vi.mocked(api.startLabFidelityOperation).mockImplementation(async ({ primaryRequestId, referenceRequestId }) => ({
      id: 'lab-fidelity-test',
      status: 'completed',
      phase: 'aggregate',
      primaryRequestId,
      primaryArtifactId: `artifact-${primaryRequestId}`,
      primaryGeneration: `generation-${primaryRequestId}:1`,
      materialSignature: `material-${primaryRequestId}`,
      completedWindows: 1,
      totalWindows: 1,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      result: {
        assetPath,
        source: previewInspection.source,
        reference: previewInspection.source,
        conversion: previewInspection.output,
        sourceFrameStructure: previewInspection.sourceFrameStructure,
        outputFrameStructure: previewInspection.outputFrameStructure,
        qsvFrameWarnings: [],
        qsvFeatureStatus: {},
        frameRecommendation: { targetGopFrames: 19, targetGopSeconds: 19 / (24000 / 1001), maxBFrames: 2, confidence: 'high', reasons: [] },
        frameValidation: { verdict: 'safe', confidence: 'high', reasons: [] },
        hevcLevelValidation: { verdict: 'safe', status: 'validated', reason: '' },
        requestedEncoder: previewInspection.requestedEncoder,
        effectiveEncoder: previewInspection.effectiveEncoder,
        requestedQSVRateControl: '',
        effectiveQSVRateControl: '',
        referenceEncoder: 'source',
        ffmpegArgs: previewInspection.ffmpegArgs,
        normalization: previewInspection.normalization,
        metrics: { comparable: true, reason: '', sourceDimensions: '960x720', outputDimensions: '960x720', ssim: 0.99, psnr: 42 },
        referenceRequestId,
        conversionRequestId: primaryRequestId,
        fidelityWindows: [{
          position: 0.5,
          startSeconds: 0,
          durationSeconds: 20,
          source: previewInspection.source,
          output: previewInspection.output,
          metrics: { comparable: true, reason: '', sourceDimensions: '960x720', outputDimensions: '960x720', ssim: 0.99, psnr: 42 },
          requestId: primaryRequestId,
          canonicalFidelity: { status: 'passed', fields: {} },
        }],
        canonicalFidelity: { status: 'passed', fields: {} },
        canonicalStatus: 'passed',
        aggregateStatus: 'completed',
        visualAggregate: {
          comparableCount: 1,
          totalCount: 1,
          averageSSIM: 0.99,
          minimumSSIM: 0.99,
          averagePSNR: 42,
          minimumPSNR: 42,
          worstWindowIndex: 0,
          worstPosition: 0.5,
          metricVerdict: 'Safe',
        },
        overallVerdict: 'Safe',
        completedAt: Date.now(),
      },
    } as never));
    vi.mocked(api.labFidelityOperation).mockResolvedValue({ status: 'completed' } as never);
    vi.mocked(api.cancelLabFidelityOperation).mockResolvedValue({ status: 'canceled' } as never);
    vi.mocked(api.recommendEncoderQuality).mockResolvedValue(undefined as never);
  });

  function referencePreviewRequests() {
    return vi
      .mocked(api.createCompatiblePreviewRequest)
      .mock.calls
      .map(([options]) => options)
      .filter(
        (options) =>
          options.videoCodec === 'x264' &&
          options.videoEncoder === 'libx264' &&
          options.useHardwareIfAvailable ===
            false &&
          !options.profile,
      );
  }

  it(
    'reuses Sample A when Process Video runs again with the same reference inputs',
    async () => {
      vi.mocked(api.profiles).mockResolvedValue([
        qsvSavedProfile,
      ]);

      vi.mocked(api.profilesAdmin).mockResolvedValue([
        qsvSavedProfile,
      ]);

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      });

      render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter
            initialEntries={[
              `/lab?assetPath=${encodeURIComponent(
                assetPath,
              )}&videoProfileId=78&section=video`,
            ]}
          >
            <ProfileLabPage />
          </MemoryRouter>
        </QueryClientProvider>,
      );

      const processVideo =
        await screen.findByRole(
          'button',
          { name: 'Process Video' },
          { timeout: 12_000 },
        );

      await waitFor(() =>
        expect(
          (processVideo as HTMLButtonElement)
            .disabled,
        ).toBe(false),
      );

      await userEvent.click(processVideo);

      await waitFor(
        () =>
          expect(
            referencePreviewRequests(),
          ).toHaveLength(1),
        { timeout: 12_000 },
      );

      await waitFor(() =>
        expect(
          (processVideo as HTMLButtonElement)
            .disabled,
        ).toBe(false),
      );

      const requestCountAfterFirstProcess =
        vi.mocked(
          api.createCompatiblePreviewRequest,
        ).mock.calls.length;

      await userEvent.click(processVideo);

      await waitFor(() =>
        expect(
          vi.mocked(
            api.createCompatiblePreviewRequest,
          ).mock.calls.length,
        ).toBeGreaterThan(
          requestCountAfterFirstProcess,
        ),
      );

      expect(
        referencePreviewRequests(),
      ).toHaveLength(1);
    },
    20_000,
  );

  it('rejects a same-path Sample A cache when the backend source revision changes', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);
    await waitFor(() => expect(referencePreviewRequests()).toHaveLength(1), { timeout: 12_000 });

    const revisionB = { size: 2048, modifiedAtNS: 2, mode: 0o600 };
    const readyB = {
      requestId: 'revision-b-reference', artifactId: 'artifact-revision-b', generationToken: 'generation-revision-b',
      generationVersion: 2, sourceRevision: revisionB, start: '00:00:00', seconds: 20,
      normalization: { mode: 'normalize_bt709', applied: true, sarPreserved: true }, state: 'ready', code: 'ready',
    } as never;
    vi.mocked(api.compatiblePreviewStatus).mockResolvedValue(readyB);
    vi.mocked(waitForCompatiblePreviewReady).mockResolvedValue(readyB);

    await userEvent.click(processVideo);
    await waitFor(() => expect(referencePreviewRequests()).toHaveLength(2), { timeout: 12_000 });
    expect(vi.mocked(api.inspectCompatibleAssetPreview)).toHaveBeenCalledTimes(2);
  }, 20_000);

  it('publishes a playable primary preview before Fidelity completes and cancels only Fidelity', async () => {
    vi.mocked(api.startLabFidelityOperation).mockImplementationOnce(async ({ primaryRequestId }) => ({
      id: 'lab-fidelity-running',
      status: 'running',
      phase: 'additional_window_encode',
      primaryRequestId,
      primaryArtifactId: `artifact-${primaryRequestId}`,
      primaryGeneration: `generation-${primaryRequestId}:1`,
      materialSignature: `material-${primaryRequestId}`,
      completedWindows: 1,
      totalWindows: 5,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    } as never));
    vi.mocked(api.labFidelityOperation).mockImplementation(() => new Promise(() => undefined) as never);

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);

    await waitFor(() => expect(api.startLabFidelityOperation).toHaveBeenCalled(), { timeout: 12_000 });
    expect(await screen.findByText('Video draft')).toBeTruthy();
    expect(await screen.findByText(/Analyzing Fidelity window 2 of 5/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Process Video' })).toBeTruthy();

    await userEvent.click(screen.getByRole('tab', { name: 'Frame Fidelity' }));
    await userEvent.click(screen.getByRole('button', { name: 'Cancel Fidelity' }));
    await waitFor(() => expect(api.cancelLabFidelityOperation).toHaveBeenCalledWith('lab-fidelity-running'));
    expect(screen.getByText(/Fidelity canceled\. Sample B remains ready and playable/i)).toBeTruthy();
    await userEvent.click(screen.getByRole('tab', { name: 'A/B Previews' }));
    expect(screen.getByText('Video draft')).toBeTruthy();
  }, 20_000);

  it('retries Fidelity with the existing primary request and stops polling for terminal results', async () => {
    vi.mocked(api.startLabFidelityOperation).mockResolvedValueOnce({
      id: 'lab-fidelity-failed',
      status: 'failed',
      phase: 'additional_window_metrics',
      primaryRequestId: 'will-be-replaced-by-mock',
      primaryArtifactId: 'artifact-will-be-replaced-by-mock',
      primaryGeneration: 'generation-will-be-replaced-by-mock:1',
      materialSignature: 'material-will-be-replaced-by-mock',
      completedWindows: 1,
      totalWindows: 5,
      error: 'additional evidence failed',
      retryable: true,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    } as never);

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);
    await screen.findByText('Video draft');
    await userEvent.click(screen.getByRole('tab', { name: 'Frame Fidelity' }));
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Frame Fidelity' }).getAttribute('aria-selected')).toBe('true'));
    await waitFor(() => expect(screen.getByText(/additional evidence failed/i)).toBeTruthy(), { timeout: 12_000 });
    expect(api.labFidelityOperation).not.toHaveBeenCalled();

    const primaryRequestId = vi.mocked(api.startLabFidelityOperation).mock.calls[0]?.[0].primaryRequestId;
    const previewGenerationRequests = vi.mocked(api.createCompatiblePreviewRequest).mock.calls.length;
    await userEvent.click(screen.getByRole('button', { name: 'Retry Fidelity' }));
    await waitFor(() => expect(api.startLabFidelityOperation).toHaveBeenCalledTimes(2), { timeout: 12_000 });
    expect(vi.mocked(api.startLabFidelityOperation).mock.calls[1]?.[0].primaryRequestId).toBe(primaryRequestId);
    expect(vi.mocked(api.createCompatiblePreviewRequest).mock.calls.length).toBe(previewGenerationRequests);
  }, 20_000);

  it('maps a lost process-local operation to retryable Fidelity without changing Preview Ready', async () => {
    vi.mocked(api.startLabFidelityOperation).mockImplementationOnce(async ({ primaryRequestId }) => ({
      id: 'lab-fidelity-restarted',
      status: 'running',
      phase: 'primary_evidence',
      primaryRequestId,
      primaryArtifactId: `artifact-${primaryRequestId}`,
      primaryGeneration: `generation-${primaryRequestId}:1`,
      materialSignature: `material-${primaryRequestId}`,
      completedWindows: 0,
      totalWindows: 5,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    } as never));
    vi.mocked(api.labFidelityOperation).mockRejectedValueOnce(new ApiRequestError('unknown operation', 404, { code: 'unknown_operation' }));

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);
    await waitFor(() => expect(api.labFidelityOperation).toHaveBeenCalled(), { timeout: 6_000 });
    await userEvent.click(screen.getByRole('tab', { name: 'Frame Fidelity' }));
    expect(await screen.findByText(/after the backend restart/i)).toBeTruthy();
    await userEvent.click(screen.getByRole('tab', { name: 'A/B Previews' }));
    expect(screen.getByText('Video draft')).toBeTruthy();
  }, 20_000);

  it('ignores a late Fidelity result after the material signature changes', async () => {
    let resolvePoll: ((operation: unknown) => void) | undefined;
    vi.mocked(api.startLabFidelityOperation).mockImplementationOnce(async ({ primaryRequestId }) => ({
      id: 'lab-fidelity-signature-a',
      status: 'running',
      phase: 'additional_window_encode',
      primaryRequestId,
      primaryArtifactId: `artifact-${primaryRequestId}`,
      primaryGeneration: `generation-${primaryRequestId}:1`,
      materialSignature: `material-${primaryRequestId}`,
      completedWindows: 1,
      totalWindows: 5,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    } as never));
    vi.mocked(api.labFidelityOperation).mockImplementationOnce(() => new Promise((resolve) => {
      resolvePoll = resolve;
    }) as never);

    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);
    await waitFor(() => expect(api.labFidelityOperation).toHaveBeenCalled(), { timeout: 6_000 });
    await userEvent.click(screen.getByRole('tab', { name: 'Frame Fidelity' }));
    const startInput = screen.getByLabelText('Start');
    await userEvent.clear(startInput);
    await userEvent.type(startInput, '00:00:05');
    expect((startInput as HTMLInputElement).value).toBe('00:00:05');
    await waitFor(() => expect(api.cancelLabFidelityOperation).toHaveBeenCalledWith('lab-fidelity-signature-a'));

    const primaryRequestIdA = vi.mocked(api.startLabFidelityOperation).mock.calls[0]?.[0].primaryRequestId ?? '';
    resolvePoll?.({
      id: 'lab-fidelity-signature-a',
      status: 'completed',
      phase: 'aggregate',
      primaryRequestId: primaryRequestIdA,
      primaryArtifactId: `artifact-${primaryRequestIdA}`,
      primaryGeneration: `generation-${primaryRequestIdA}:1`,
      materialSignature: `material-${primaryRequestIdA}`,
      completedWindows: 1,
      totalWindows: 1,
      result: {
        fidelityWindows: [{
          position: 0.5,
          startSeconds: 0,
          durationSeconds: 20,
          source: previewInspection.source,
          output: previewInspection.output,
          metrics: { comparable: true, ssim: 0.99, psnr: 42 },
          requestId: primaryRequestIdA,
          canonicalFidelity: { status: 'passed' },
        }],
        source: previewInspection.source,
        reference: previewInspection.source,
        conversion: previewInspection.output,
        normalization: previewInspection.normalization,
        referenceRequestId: 'reference-a',
        conversionRequestId: primaryRequestIdA,
        canonicalStatus: 'passed',
        aggregateStatus: 'completed',
        visualAggregate: { comparableCount: 1, totalCount: 1, averageSSIM: 0.99, minimumSSIM: 0.99, metricVerdict: 'Safe' },
        overallVerdict: 'Safe',
      },
    });
    await waitFor(() => expect(screen.queryByText('Overall · Safe')).toBeNull());
    expect(api.startLabFidelityOperation).toHaveBeenCalledTimes(1);
  }, 20_000);

  it('keeps Fidelity results and media evidence read-only when switching tabs', async () => {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const processVideo = await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    await waitFor(() => expect((processVideo as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(processVideo);
    await waitFor(() => expect(api.startLabFidelityOperation).toHaveBeenCalled(), { timeout: 12_000 });
    const counts = {
      previews: vi.mocked(api.createCompatiblePreviewRequest).mock.calls.length,
      probes: vi.mocked(api.inspectCompatibleAssetPreview).mock.calls.length,
      metrics: vi.mocked(api.compatibleAssetFrameMetrics).mock.calls.length,
      fidelity: vi.mocked(api.startLabFidelityOperation).mock.calls.length,
    };

    for (const tab of ['Technical Snapshot', 'Frame Fidelity', 'Characteristics', 'Warnings & Guidance', 'A/B Previews']) {
      await userEvent.click(screen.getByRole('tab', { name: tab }));
    }

    expect(vi.mocked(api.createCompatiblePreviewRequest).mock.calls.length).toBe(counts.previews);
    expect(vi.mocked(api.inspectCompatibleAssetPreview).mock.calls.length).toBe(counts.probes);
    expect(vi.mocked(api.compatibleAssetFrameMetrics).mock.calls.length).toBe(counts.metrics);
    expect(vi.mocked(api.startLabFidelityOperation).mock.calls.length).toBe(counts.fidelity);
  }, 20_000);

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

  it('shows capability-gated RDO beside MBBRC for QSV profiles', async () => {
    vi.mocked(api.profiles).mockResolvedValue([qsvSavedProfile]);
    vi.mocked(api.profilesAdmin).mockResolvedValue([qsvSavedProfile]);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}&videoProfileId=78&section=video`]}>
          <ProfileLabPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    await screen.findByRole('button', { name: 'Process Video' }, { timeout: 12_000 });
    expect(await screen.findByLabelText('MBBRC')).toBeTruthy();
    const rdo = await screen.findByLabelText('RDO');
    expect(rdo.textContent).toContain('Auto');
    expect(screen.getByText(/Rate-distortion optimization.*Auto leaves the driver default unchanged/)).toBeTruthy();
    await userEvent.click(rdo);
    expect(screen.getByRole('option', { name: /Auto/ })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Enabled' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Disabled' })).toBeTruthy();
  }, 15_000);

  it('renders backend GOP candidates and keeps Manual strategy separate from B-frames', async () => {
    const onChangeMany = vi.fn();
    const view = render(
      <FrameStructureControls
        config={{ frameStructureMode: 'auto', frameStructureGopMode: 'auto' }}
        recommendedGop={19}
        recommendedBFrames={2}
        recommendedGopByMode={{ compatible: 15, balanced: 19, maximum_compression: 26 }}
        autoStrategy="balanced"
        analysisDriven
        frameRate={24000 / 1001}
        onChange={vi.fn()}
        onChangeMany={onChangeMany}
      />,
    );

    expect(screen.getByText(/Effective GOP: 19 frames/)).toBeTruthy();
    expect(screen.getByText(/Candidates: Compatible 15 · Balanced 19 · Maximum Compression 26/)).toBeTruthy();
    expect(screen.getByText('Analysis-based GOP recommendation')).toBeTruthy();
    expect(screen.queryByText(/Generic fallback/)).toBeNull();
    expect(screen.queryByLabelText('GOP Strategy')).toBeNull();
    await userEvent.click(screen.getByLabelText('GOP Mode'));
    await userEvent.click(screen.getByRole('option', { name: 'Manual' }));
    expect(onChangeMany).toHaveBeenCalledWith(expect.objectContaining({
      frameStructureGopMode: 'recommended',
      frameStructureGopStrategy: 'balanced',
    }));
    expect(onChangeMany.mock.calls.at(-1)?.[0]).not.toHaveProperty('frameStructureBFrameMode');

    view.rerender(
      <FrameStructureControls
        config={{ frameStructureMode: 'auto', frameStructureGopMode: 'recommended', frameStructureGopStrategy: 'balanced' }}
        recommendedGop={19}
        recommendedBFrames={2}
        recommendedGopByMode={{ compatible: 15, balanced: 19, maximum_compression: 26 }}
        autoStrategy="balanced"
        analysisDriven
        frameRate={24000 / 1001}
        onChange={vi.fn()}
        onChangeMany={onChangeMany}
      />,
    );
    expect(screen.getByLabelText('GOP Strategy')).toBeTruthy();
    expect(screen.queryByLabelText('GOP')).toBeNull();
    await userEvent.click(screen.getByLabelText('GOP Strategy'));
    expect(screen.getByRole('option', { name: 'Compatible · 15' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Balanced · 19' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Maximum Compression · 26' })).toBeTruthy();
    await userEvent.click(screen.getByRole('option', { name: 'Custom' }));
    expect(onChangeMany).toHaveBeenLastCalledWith(expect.objectContaining({
      frameStructureGopMode: 'custom',
      frameStructureGopStrategy: 'custom',
    }));

    view.rerender(
      <FrameStructureControls
        config={{ frameStructureMode: 'auto', frameStructureGopMode: 'custom', frameStructureGopStrategy: 'custom', frameStructureGopFrames: 48 }}
        recommendedGop={19}
        recommendedBFrames={2}
        recommendedGopByMode={{ compatible: 15, balanced: 19, maximum_compression: 26 }}
        autoStrategy="balanced"
        analysisDriven={false}
        frameRate={24000 / 1001}
        onChange={vi.fn()}
        onChangeMany={onChangeMany}
      />,
    );
    expect((screen.getByLabelText('GOP') as HTMLInputElement).value).toBe('48');

    view.rerender(
      <FrameStructureControls
        config={{ frameStructureMode: 'auto', frameStructureGopMode: 'auto' }}
        recommendedGop={19}
        recommendedBFrames={2}
        recommendedGopByMode={{ compatible: 15, balanced: 19, maximum_compression: 26 }}
        autoStrategy="balanced"
        analysisDriven={false}
        frameRate={24000 / 1001}
        onChange={vi.fn()}
        onChangeMany={onChangeMany}
      />,
    );
    expect(screen.getByText(/Generic fallback/)).toBeTruthy();
    expect(screen.queryByText('Analysis-based GOP recommendation')).toBeNull();
  });
});
