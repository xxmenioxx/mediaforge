// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { Asset, ScanResult, TestEncode } from '../api/types';
import { LabTestEncodeComparison } from './LabTestEncodeComparison';

vi.mock('../api/client', () => ({ api: { assetPreviewUrl: vi.fn((path: string) => `/preview?path=${encodeURIComponent(path)}`) } }));

const asset: Asset = {
  id: 3,
  libraryId: 4,
  libraryName: 'TV',
  path: '/media/raw/episode.mkv',
  relativePath: 'episode.mkv',
  groupPath: '/media/raw',
  fileName: 'episode.mkv',
  extension: 'mkv',
  sizeBytes: 1_000_000_000,
  modifiedAt: '',
  status: 'unprocessed',
  missing: false,
  review: {} as Asset['review'],
  metadata: {} as Asset['metadata'],
  conversion: {} as Asset['conversion'],
};

const scan = {
  path: asset.path,
  fileName: asset.fileName,
  container: 'matroska',
  sizeBytes: asset.sizeBytes,
  duration: 120,
  bitrate: 9_000_000,
  videoCodec: 'h264',
  width: 1920,
  height: 1080,
  videoStreams: [{ type: 'video', codec: 'h264', frameRate: '23.976' }],
} as unknown as ScanResult;

const readyTest: TestEncode = {
  id: 11,
  sourceAssetId: 3,
  sourcePath: asset.path,
  sourceFingerprint: 'source-revision',
  sourceSizeBytes: asset.sizeBytes,
  libraryId: 4,
  configurationSource: 'lab_draft',
  requestedConfiguration: {},
  effectiveConfiguration: {},
  configurationHash: 'hash',
  profileId: 0,
  profileVersion: 0,
  workerName: 'worker-1',
  effectiveEncoder: 'libx264',
  startSeconds: 600,
  durationSeconds: 20,
  status: 'ready',
  phase: 'ready',
  progress: 100,
  ffmpegCommand: '',
  outputPath: '/media/library/episode-test.mkv',
  outputSizeBytes: 300_000_000,
  subtitleArtifacts: [],
  validationReport: {
    perceptualEvidence: {
      status: 'ready',
      windows: [{ primary: true, metrics: [
        { identity: { identity: 'psnr-1', metric: { logicalMetric: 'PSNR' } }, status: 'available', value: { kind: 'finite', value: 41.2 } },
        { identity: { identity: 'ssim-1', metric: { logicalMetric: 'SSIM' } }, status: 'available', value: { kind: 'finite', value: 0.985 } },
        { identity: { identity: 'vmaf-1', metric: { logicalMetric: 'VMAF' } }, status: 'available', value: { kind: 'finite', value: 96.4 } },
      ] }],
    },
  },
  keep: false,
  reconciliationStatus: 'healthy',
  outputExists: true,
  artifactCount: 1,
  sidecarArtifactCount: 0,
  knownDiskUsageBytes: 300_000_000,
  unknownSizeArtifactCount: 0,
  errorMessage: '',
  stale: false,
  createdAt: '2026-09-29T10:00:00Z',
  completedAt: '2026-09-29T10:01:00Z',
  updatedAt: '2026-09-29T10:01:00Z',
};

function renderComparison(tests = [readyTest], selectedTest: TestEncode | null = readyTest, retainedReferenceUrl?: string) {
  return render(
    <LabTestEncodeComparison
      asset={asset}
      snapshot={scan}
      tests={tests}
      selectedTest={selectedTest}
      retainedReferenceUrl={retainedReferenceUrl}
      loading={false}
      onSelectTest={vi.fn()}
      onManageTests={vi.fn()}
    />,
  );
}

describe('LabTestEncodeComparison', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('uses the retained result, source metrics, and stored canonical quality values', () => {
    renderComparison();

    expect(screen.getByLabelText('Original source preview').getAttribute('src')).toBe('/preview?path=%2Fmedia%2Fraw%2Fepisode.mkv');
    expect(screen.getByLabelText('Selected Test Encode result preview').getAttribute('src')).toBe('/preview?path=%2Fmedia%2Flibrary%2Fepisode-test.mkv');
    expect(screen.getByText(/Comparing Test Encode T11/)).toBeTruthy();
    expect(screen.getByText('PSNR · available · 41.20 dB')).toBeTruthy();
    expect(screen.getByText('SSIM · available · 0.9850')).toBeTruthy();
    expect(screen.getByText('VMAF · available · 96.4000')).toBeTruthy();
    expect(screen.getByText('Size difference')).toBeTruthy();
    expect(screen.getByText('Unavailable · source sample size is not retained')).toBeTruthy();
    expect(screen.getByText('Test sample size')).toBeTruthy();
    expect(screen.getByRole('button', { name: /save test encode frame/i })).toBeTruthy();
    expect(api.assetPreviewUrl).toHaveBeenCalledTimes(2);
  });

  it('selects persisted ready runs and leaves active or missing outputs available only in history', async () => {
    const onSelectTest = vi.fn();
    const activeRun = { ...readyTest, id: 12, status: 'generating' as const, outputExists: false };
    const missingRun = { ...readyTest, id: 13, status: 'ready' as const, outputExists: false };
    render(
      <LabTestEncodeComparison
        asset={asset}
        snapshot={scan}
        tests={[readyTest, activeRun, missingRun]}
        selectedTest={readyTest}
        loading={false}
        onSelectTest={onSelectTest}
        onManageTests={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /select test encode t11/i }));
    await waitFor(() => expect(onSelectTest).toHaveBeenCalledWith(11));
    expect((screen.getByRole('button', { name: /select test encode t12/i }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: /select test encode t13/i }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('keeps equivalent comparison panes and maps playback position between the source range and result', async () => {
    renderComparison();
    const source = screen.getByLabelText('Original source preview') as HTMLVideoElement;
    const result = screen.getByLabelText('Selected Test Encode result preview') as HTMLVideoElement;
    Object.defineProperty(source, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_METADATA });
    Object.defineProperty(result, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_METADATA });
    result.currentTime = 3;
    fireEvent.timeUpdate(result);

    await waitFor(() => expect(source.currentTime).toBe(603));
    expect(screen.getByText('00:00:03 / 00:00:20')).toBeTruthy();
    expect((screen.getByLabelText('Synchronized comparison position') as HTMLInputElement).disabled).toBe(false);
  });

  it('reuses an already prepared Original reference and aligns it from its sample start', async () => {
    renderComparison([readyTest], readyTest, '/compatible/ready-reference');
    const source = screen.getByLabelText('Original source preview') as HTMLVideoElement;
    const result = screen.getByLabelText('Selected Test Encode result preview') as HTMLVideoElement;
    expect(source.getAttribute('src')).toBe('/compatible/ready-reference');
    expect(api.assetPreviewUrl).toHaveBeenCalledTimes(1);

    Object.defineProperty(source, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_METADATA });
    Object.defineProperty(result, 'readyState', { configurable: true, value: HTMLMediaElement.HAVE_METADATA });
    result.currentTime = 3;
    fireEvent.timeUpdate(result);
    await waitFor(() => expect(source.currentTime).toBe(3));
  });
});
