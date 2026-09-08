// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type {
  Asset,
  AssetInventory,
  ProfileSampleEstimateOperation,
  ScanResult,
} from '../api/types';

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
      recommendEncoderQuality: vi.fn(),
      startProfileSampleEstimateOperation: vi.fn(),
      profileSampleEstimateOperation: vi.fn(),
      cancelProfileSampleEstimateOperation: vi.fn(),
    },
  };
});

import { ApiRequestError, api } from '../api/client';
import { ProfileLabPage } from './ProfileLabPage';

const assetPath = '/media/raw/movies/Test/Test.mkv';
const storageKey = 'mvforge.profileLab.sampleEstimateOperation';

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

function operation(
  patch: Partial<ProfileSampleEstimateOperation>,
): ProfileSampleEstimateOperation {
  return {
    id: 'estimate-1',
    status: 'queued',
    phase: 'waiting_for_capacity',
    progress: 0,
    currentSample: 0,
    sampleCount: 0,
    currentSampleProgress: 0,
    encodedSeconds: 0,
    totalSampleSeconds: 0,
    speed: 0,
    etaSeconds: 0,
    createdAt: '2026-09-08T00:00:00Z',
    updatedAt: '2026-09-08T00:00:00Z',
    ...patch,
  };
}

function renderLab() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter
        initialEntries={[`/lab?assetPath=${encodeURIComponent(assetPath)}`]}
      >
        <ProfileLabPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

async function openSampleEstimate() {
  await userEvent.click(
    await screen.findByRole(
      'tab',
      { name: 'Frame Fidelity' },
      { timeout: 12_000 },
    ),
  );
  return screen.findByRole('button', { name: 'Measure samples' });
}

afterEach(cleanup);

describe('Profile Lab asynchronous sample estimate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.sessionStorage.clear();
    HTMLElement.prototype.scrollIntoView = vi.fn();
    const inventory = {
      sourceGroups: [],
      unprocessed: [asset],
      library: [],
      converted: [],
      unverified: [],
      accepted: [],
      archive: [],
      missing: [],
      unprocessedGroups: [],
      libraryGroups: [],
      convertedGroups: [],
      unverifiedGroups: [],
      acceptedGroups: [],
      archiveGroups: [],
      reports: {},
      sync: {},
    } as unknown as AssetInventory;
    vi.mocked(api.assets).mockResolvedValue(inventory);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.profilesAdmin).mockResolvedValue([]);
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.runtimeSnapshot).mockResolvedValue({ encoders: {} } as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.latestSnapshot).mockResolvedValue({
      found: true,
      snapshot: scan,
      status: 'current',
      requiresAnalysis: false,
      staleComponents: [],
    });
    vi.mocked(api.recommendEncoderQuality).mockResolvedValue(undefined as never);
  });

  it('polls queued and running operations, renders backend progress, and consumes the completed result', async () => {
    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(
      operation({}),
    );
    vi.mocked(api.profileSampleEstimateOperation)
      .mockResolvedValueOnce(operation({}))
      .mockResolvedValueOnce(operation({
        status: 'running',
        phase: 'encoding',
        progress: 46.7,
        currentSample: 2,
        sampleCount: 3,
        currentSampleProgress: 40,
        encodedSeconds: 28,
        totalSampleSeconds: 60,
        speed: 0.31,
        etaSeconds: 198,
      }))
      .mockResolvedValue(operation({
        status: 'completed',
        phase: 'completed',
        progress: 100,
        currentSample: 3,
        sampleCount: 3,
        currentSampleProgress: 100,
        encodedSeconds: 60,
        totalSampleSeconds: 60,
        result: {
          assetPath,
          durationSeconds: 120,
          sampleSeconds: 20,
          sampleStarts: [10, 50, 90],
          sampleCount: 3,
          measuredVideoBytes: 500,
          estimatedVideoBytes: 1500,
          measuredVideoBitrate: 200,
          confidence: 'high',
          source: 'distributed_profile_samples',
          effectiveEncoder: 'libx265',
          persisted: false,
        },
      }));

    renderLab();
    const start = await openSampleEstimate();
    await userEvent.click(start);

    expect(api.startProfileSampleEstimateOperation).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('46.7%', {}, { timeout: 3_500 })).toBeTruthy();
    expect(api.profileSampleEstimateOperation).toHaveBeenCalledTimes(2);
    expect((start as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByText('66.7%')).toBeNull();
    expect(screen.getByText('Sample 2 of 3')).toBeTruthy();
    expect(screen.getByText('Current sample: 40.0%')).toBeTruthy();
    expect(screen.getByText('28.0 / 60.0 s encoded')).toBeTruthy();
    expect(screen.getByText('Speed: 0.31x')).toBeTruthy();
    expect(screen.getByText('Estimated remaining: ~3m 18s')).toBeTruthy();

    expect(await screen.findByText(/Video estimate 1 KB · libx265/, {}, { timeout: 3_500 })).toBeTruthy();
    const callsAfterCompletion = vi.mocked(api.profileSampleEstimateOperation).mock.calls.length;
    await new Promise((resolve) => window.setTimeout(resolve, 1_100));
    expect(api.profileSampleEstimateOperation).toHaveBeenCalledTimes(callsAfterCompletion);
    fireEvent.change(screen.getByLabelText('New video profile name'), {
      target: { value: 'Changed draft' },
    });
    await waitFor(() =>
      expect(screen.queryByText(/Video estimate .* · libx265/)).toBeNull(),
    );
  }, 20_000);

  it('cancels through the backend and supports a dynamic five-sample operation', async () => {
    const running = operation({
      status: 'running',
      phase: 'encoding',
      progress: 28,
      currentSample: 2,
      sampleCount: 5,
      currentSampleProgress: 40,
      encodedSeconds: 28,
      totalSampleSeconds: 100,
    });
    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(operation({}));
    vi.mocked(api.profileSampleEstimateOperation).mockResolvedValue(running);
    vi.mocked(api.cancelProfileSampleEstimateOperation).mockResolvedValue(
      operation({ status: 'canceled', phase: 'canceled' }),
    );

    renderLab();
    await userEvent.click(await openSampleEstimate());
    expect(await screen.findByText('Sample 2 of 5', {}, { timeout: 2_500 })).toBeTruthy();
    expect(screen.queryByText('Speed: 0.00x')).toBeNull();
    const cancel = screen.getByText('Cancel').closest('button');
    expect(cancel).toBeTruthy();
    await userEvent.click(cancel!);

    await waitFor(() =>
      expect(api.cancelProfileSampleEstimateOperation).toHaveBeenCalledWith('estimate-1'),
    );
    expect(await screen.findByText('Sample estimate canceled.')).toBeTruthy();
    expect((await openSampleEstimate() as HTMLButtonElement).disabled).toBe(false);
  }, 10_000);

  it('keeps ownership of a running estimate when the draft changes', async () => {
    const running = operation({
      status: 'running',
      phase: 'encoding',
      progress: 46.7,
      currentSample: 2,
      sampleCount: 3,
      currentSampleProgress: 40,
      encodedSeconds: 28,
      totalSampleSeconds: 60,
      speed: 0.31,
      etaSeconds: 198,
    });

    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(
      operation({}),
    );
    vi.mocked(api.profileSampleEstimateOperation).mockResolvedValue(running);

    renderLab();

    await userEvent.click(await openSampleEstimate());

    expect(
      await screen.findByText('Sample 2 of 3', {}, { timeout: 2_500 }),
    ).toBeTruthy();

    expect(window.sessionStorage.getItem(storageKey)).toContain('estimate-1');

    fireEvent.change(screen.getByLabelText('New video profile name'), {
      target: { value: 'Changed while estimate is running' },
    });

    expect(
      await screen.findByText(
        /This sample estimate is still running for the previous asset or profile/,
      ),
    ).toBeTruthy();

    await new Promise((resolve) => window.setTimeout(resolve, 1_000));

    expect(window.sessionStorage.getItem(storageKey)).toContain('estimate-1');

    expect(
      screen.getByRole('button', { name: 'Cancel', hidden: true }),
    ).toBeTruthy();

    expect(
      (screen.getByRole('button', {
        name: 'Measure samples',
      }) as HTMLButtonElement).disabled,
    ).toBe(true);

    expect(api.startProfileSampleEstimateOperation).toHaveBeenCalledTimes(1);
  }, 10_000);

  it('reconnects after reload without creating a duplicate operation', async () => {
    const running = operation({
      status: 'running',
      phase: 'encoding',
      progress: 25,
      currentSample: 1,
      sampleCount: 3,
      currentSampleProgress: 75,
      encodedSeconds: 15,
      totalSampleSeconds: 60,
    });
    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(operation({}));
    vi.mocked(api.profileSampleEstimateOperation).mockResolvedValue(running);

    const first = renderLab();
    await userEvent.click(await openSampleEstimate());
    await screen.findByText('Sample 1 of 3');
    expect(window.sessionStorage.getItem(storageKey)).toContain('estimate-1');
    const callsBeforeReload = vi.mocked(api.profileSampleEstimateOperation).mock.calls.length;
    first.unmount();

    renderLab();
    expect(await screen.findByText('Sample 1 of 3')).toBeTruthy();
    expect(api.startProfileSampleEstimateOperation).toHaveBeenCalledTimes(1);
    expect(vi.mocked(api.profileSampleEstimateOperation).mock.calls.length).toBeGreaterThan(callsBeforeReload);
  }, 10_000);

  it('stops polling and displays a backend operation failure', async () => {
    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(operation({}));
    vi.mocked(api.profileSampleEstimateOperation).mockResolvedValue(
      operation({
        status: 'failed',
        phase: 'failed',
        error: 'encoder unavailable',
      }),
    );

    renderLab();
    await userEvent.click(await openSampleEstimate());
    expect(await screen.findByText('Sample estimate failed: encoder unavailable')).toBeTruthy();
    const callsAfterFailure = vi.mocked(api.profileSampleEstimateOperation).mock.calls.length;
    await new Promise((resolve) => window.setTimeout(resolve, 1_100));
    expect(api.profileSampleEstimateOperation).toHaveBeenCalledTimes(callsAfterFailure);
  }, 10_000);

  it('clears a recovered operation that no longer exists', async () => {
    vi.mocked(api.startProfileSampleEstimateOperation).mockResolvedValue(operation({}));
    vi.mocked(api.profileSampleEstimateOperation).mockResolvedValueOnce(operation({ status: 'running', phase: 'encoding' }));
    const first = renderLab();
    await userEvent.click(await openSampleEstimate());
    await waitFor(() => expect(window.sessionStorage.getItem(storageKey)).toContain('estimate-1'));
    first.unmount();

    vi.mocked(api.profileSampleEstimateOperation).mockRejectedValue(
      new ApiRequestError('profile sample estimate operation not found', 404),
    );
    renderLab();
    await waitFor(() => expect(window.sessionStorage.getItem(storageKey)).toBeNull());
    expect(api.startProfileSampleEstimateOperation).toHaveBeenCalledTimes(1);
  }, 10_000);
});
