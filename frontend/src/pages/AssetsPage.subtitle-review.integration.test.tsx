// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Asset, AssetInventory, AssetPath, ScanResult } from '../api/types';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    api: {
      ...actual.api,
      queueJobs: vi.fn(), assets: vi.fn(), profiles: vi.fn(), libraries: vi.fn(), settings: vi.fn(), snapshotOperations: vi.fn(),
      latestSnapshot: vi.fn(), startSnapshotOperation: vi.fn(), snapshotOperation: vi.fn(), externalAssetSubtitles: vi.fn(), extractAssetSubtitles: vi.fn(), assetMetadataSidecars: vi.fn(), trackMaintenanceInventory: vi.fn(), subtitleEligibility: vi.fn(), subtitleExtractionOperations: vi.fn(), subtitleExtractionOperation: vi.fn(),
      suggestProfile: vi.fn(), runtimeState: vi.fn(), workerNodes: vi.fn(), recommendEncoderQuality: vi.fn(),
      profileAssignments: vi.fn(), updateProfileAssignment: vi.fn(), assetScopeConfigurations: vi.fn(), updateAssetScopeConfiguration: vi.fn(), effectiveAssetConfiguration: vi.fn(), effectiveAssetConfigurations: vi.fn(),
      configureLogicalGroupsBatch: vi.fn(), queueSelectedAssets: vi.fn(), queueRemainingPreview: vi.fn(), queueRemainingCommit: vi.fn(), createQueueBatch: vi.fn(), preflightSelectedAssets: vi.fn(), acceptQueueBatchPreflight: vi.fn(), renameAsset: vi.fn(), evaluateAdvisor: vi.fn(), publishAssetsAsIs: vi.fn(),
      updateAssetConversion: vi.fn(), updateAssetMetadata: vi.fn(), updateAssetEdit: vi.fn(), testEncodes: vi.fn(),
      jobArtifacts: vi.fn(), executionPlans: vi.fn(),
      subtitleTranslationCapability: vi.fn(), startSubtitleTranslation: vi.fn(), subtitleOperation: vi.fn(), analyzeSubtitleOCR: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { AssetsPage } from './AssetsPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const assetPath = '/media/raw/movies/Akira/Akira.mkv';
const reviewPath = '/media/raw/movies/Akira/Akira.eng.forced.srt';
const unrelatedPath = '/media/raw/movies/Akira/Akira.eng.srt';

function testAsset(): Asset {
  return {
    id: 1, libraryId: 0, libraryName: 'Originals', path: assetPath, relativePath: 'movies/Akira/Akira.mkv',
    groupPath: 'movies/Akira', fileName: 'Akira.mkv', extension: '.mkv', sizeBytes: 1024,
    modifiedAt: '2026-08-28T00:00:00Z', status: 'unprocessed', missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' }, conversion: {},
  };
}

function testPath(asset: Asset): AssetPath {
  return {
    id: 'root', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'movies/Akira',
    displayPath: 'Akira', isLogicalGroupRoot: true, fileCount: 1, assetCount: 1, sizeBytes: 1024,
    totalSizeBytes: 1024, assets: [asset],
  };
}

function testInventory(): AssetInventory {
  const asset = testAsset();
  const rootPath = testPath(asset);
  return {
    sourceGroups: [{
      id: 1, name: 'Movies', relativePath: 'movies', sourcePath: '/media/raw/movies',
      fileCount: 1, assetCount: 1, titleCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024,
      logicalGroups: [{
        id: '/media/raw/movies/Akira', name: 'Akira', path: '/media/raw/movies/Akira', relativePath: 'Akira',
        fileCount: 1, assetCount: 1, pathCount: 1, sizeBytes: 1024, totalSizeBytes: 1024, assetPaths: [rootPath],
      }],
    }],
    unprocessed: [asset], unprocessedGroups: [], library: [], converted: [], unverified: [], accepted: [], missing: [],
    libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], reports: {},
    sync: { lastSyncedAt: '2026-08-28T00:00:00Z', totalRecords: 1, missingFiles: 0, missingActionable: 0, missingHistorical: 0 },
  } as unknown as AssetInventory;
}

function testSnapshot(): ScanResult {
  return {
    id: 1, path: assetPath, fileName: 'Akira.mkv', container: 'matroska', sizeBytes: 1024, duration: 60, bitrate: 1000,
    videoCodec: 'h264', width: 1920, height: 1080, hdr: false, audioTracks: 1, subtitleTracks: 1, chapters: 0,
    videoStreams: [], audioStreams: [], subtitleStreams: [{ index: 4, codec: 'hdmv_pgs_subtitle', language: 'eng' }],
    attachmentStreams: [], compatibilityAnalysis: {}, interlaceAnalysis: {}, rawProbe: {},
    createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
  } as unknown as ScanResult;
}

function completedSnapshot() {
  return {
    id: 'snapshot-operation-1', assetPath, status: 'completed', phase: 'complete', progress: 100,
    message: 'Complete', durationMs: 10, cacheHit: false, incrementalRefresh: false, result: testSnapshot(),
    createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
  };
}

function renderAssetsPage() {
  return render(
    <MemoryRouter>
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <AssetsPage />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

async function openAssetInfo(user: ReturnType<typeof userEvent.setup>) {
  const details = await screen.findByTestId('selected-asset-detail');
  await user.click(within(details).getByRole('button', { name: 'More actions for Akira.mkv' }));
  await user.click(await screen.findByRole('menuitem', { name: 'Asset Info' }));
  return screen.findByRole('dialog');
}

function setupPageData(reviewSidecar: Record<string, unknown>, unrelatedSidecar: Record<string, unknown>) {
  vi.clearAllMocks();
  vi.mocked(api.queueJobs).mockResolvedValue([]);
  vi.mocked(api.assets).mockResolvedValue(testInventory());
  vi.mocked(api.profiles).mockResolvedValue([]);
  vi.mocked(api.libraries).mockResolvedValue([]);
  vi.mocked(api.settings).mockResolvedValue([]);
  vi.mocked(api.snapshotOperations).mockResolvedValue({ operations: [] });
  vi.mocked(api.latestSnapshot).mockResolvedValue({ found: true, snapshot: testSnapshot(), status: 'current', requiresAnalysis: false, staleComponents: [] });
  vi.mocked(api.startSnapshotOperation).mockResolvedValue(completedSnapshot() as never);
  vi.mocked(api.snapshotOperation).mockResolvedValue(completedSnapshot() as never);
  vi.mocked(api.assetMetadataSidecars).mockResolvedValue([]);
  vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({ path: assetPath, fingerprint: '', streams: [], chapters: 0, durationSeconds: 60, maintenanceAllowed: true });
  vi.mocked(api.suggestProfile).mockResolvedValue({} as never);
  vi.mocked(api.runtimeState).mockResolvedValue({ encoders: {} } as never);
  vi.mocked(api.workerNodes).mockResolvedValue([]);
  vi.mocked(api.recommendEncoderQuality).mockResolvedValue(undefined as never);
  vi.mocked(api.externalAssetSubtitles).mockResolvedValue([unrelatedSidecar, reviewSidecar] as never);
  vi.mocked(api.subtitleEligibility).mockResolvedValue({
    embedded: [{
      streamIndex: 4, sourceIdentity: 'embedded:4', sourceRevision: 'source-revision', representation: 'bitmap',
      role: 'full', roleProvenance: 'snapshot', policyVersion: 'test', regenerate: { eligible: true },
      canonicalOcrArtifactPath: reviewPath,
    }],
    external: [unrelatedSidecar, reviewSidecar],
    composition: { policyVersion: 'test', primaryCandidates: [], supplementalCandidates: [], eligible: false, maxSupplementals: 0, reason: 'test' },
  } as never);
  vi.mocked(api.subtitleExtractionOperations).mockResolvedValue({ operations: [] });
  vi.mocked(api.profileAssignments).mockResolvedValue([]);
  vi.mocked(api.assetScopeConfigurations).mockResolvedValue([]);
  vi.mocked(api.effectiveAssetConfigurations).mockResolvedValue({ configurations: {}, missingAssetIds: [] });
  vi.mocked(api.effectiveAssetConfiguration).mockResolvedValue({ assetPath, video: { selection: 'inherit' }, audio: { selection: 'inherit' }, tracks: { selection: 'inherit' }, category: { selection: 'inherit' }, destination: { selection: 'inherit' } } as never);
  vi.mocked(api.evaluateAdvisor).mockResolvedValue({} as never);
}

describe('Asset Info OCR review routing', () => {
  it('opens the canonical sidecar when translation requires OCR review', async () => {
    const user = userEvent.setup();
    const reviewSidecar = {
      path: reviewPath, fileName: 'Akira.eng.forced.srt', format: 'srt', language: 'eng', default: false, forced: true,
      sizeBytes: 100, modifiedAt: '2026-08-28T00:00:00Z', ocrManaged: true, ocrAnalysisAvailable: true,
      logicalTransformKey: 'canonical-ocr-stream-4',
    };
    const unrelatedSidecar = {
      ...reviewSidecar, path: unrelatedPath, fileName: 'Akira.eng.srt', forced: false,
      logicalTransformKey: 'other-ocr-stream',
    };
    setupPageData(reviewSidecar, unrelatedSidecar);
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue({
      source: { type: 'embedded', embedded: { streamIndex: 4 } }, eligible: true, available: true,
      sourceLanguages: ['eng'], targetLocales: ['es-419'], provider: { name: 'ollama', model: 'translate-test', modelSelectable: false },
      outputRepresentations: ['srt'], context: { status: 'healthy' },
    } as never);
    const failedTranslation = {
      id: 'translation-review-required', assetPath, status: 'error', phase: 'ocr_review', progress: 40,
      processed: 0, total: 0, streamIndex: 4, format: 'srt', kind: 'translation',
      message: 'OCR review is required before translation.',
      translation: { failureClass: 'ocr_review_required', diagnostic: 'OCR review is required before translation.', retryable: true, cancellable: false },
      createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
    };
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: failedTranslation } as never);
    vi.mocked(api.subtitleOperation).mockResolvedValue(failedTranslation as never);
    vi.mocked(api.analyzeSubtitleOCR).mockResolvedValue({ operationId: 'ocr-review-operation', status: 'running', phase: 'analyzing', progress: 0 } as never);
    vi.mocked(api.subtitleExtractionOperation).mockResolvedValue({
      id: 'ocr-review-operation', assetPath, status: 'completed', phase: 'completed', progress: 100,
      processed: 1, total: 1, streamIndex: 4, format: 'srt', kind: 'ocr_analysis',
      qualityReport: { cleanCount: 1, suspiciousCount: 0, reviewCount: 0, canCorrect: true, baselineRevision: 'review-sidecar', findings: [] },
      createdAt: '2026-08-28T00:00:00Z', updatedAt: '2026-08-28T00:00:01Z',
    } as never);

    renderAssetsPage();
    await user.click(await screen.findByRole('button', { name: 'Expand Akira' }));
    const assetInfo = await openAssetInfo(user);
    await user.click(within(assetInfo).getByRole('tab', { name: 'Tracks' }));
    await user.click(await screen.findByRole('button', { name: 'Translate subtitle stream 4' }));
    const translationDialog = await screen.findByRole('dialog', { name: 'Translate subtitle' });
    await user.click(within(translationDialog).getByRole('button', { name: 'Start translation' }));
    expect(await within(translationDialog).findByText('Translation paused')).toBeTruthy();
    await user.click(within(translationDialog).getByRole('button', { name: 'Review OCR' }));

    await waitFor(() => expect(api.analyzeSubtitleOCR).toHaveBeenCalledWith(expect.objectContaining({
      mediaPath: assetPath,
      sidecarPath: reviewPath,
      sidecarPaths: [reviewPath],
      format: 'srt',
      logicalTransformKey: 'canonical-ocr-stream-4',
    })));
    expect(api.analyzeSubtitleOCR).not.toHaveBeenCalledWith(expect.objectContaining({ sidecarPath: unrelatedPath }));
  }, 15000);
});
