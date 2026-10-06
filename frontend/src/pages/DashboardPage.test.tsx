// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      libraries: vi.fn(),
      profiles: vi.fn(),
      queueJobs: vi.fn(),
      assets: vi.fn(),
      runtimeState: vi.fn(),
      refreshRuntime: vi.fn(),
      evaluateAdvisor: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import type { AssetInventory, QueueJob, RuntimeState } from '../api/types';
import { theme } from '../theme/theme';
import { DashboardPage } from './DashboardPage';

afterEach(cleanup);

function renderDashboard(queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(
    <ThemeProvider theme={theme}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <QueryClientProvider client={queryClient}><DashboardPage /></QueryClientProvider>
      </MemoryRouter>
    </ThemeProvider>,
  );
}

function job(id: number, status: QueueJob['status'], progress: number, updatedAt: string): QueueJob {
  return {
    id,
    batchId: 'batch-42',
    batchName: 'Evening series',
    batchPosition: id,
    mediaPath: '/raw/series/episode-' + id + '.mkv',
    publishMode: 'standard',
    libraryId: 1,
    profileId: 7,
    profileVersion: 1,
    profileSnapshot: {},
    audioProfileKey: '',
    trackProfileKey: '',
    processingMode: 'full_encode',
    priority: 0,
    queuePosition: id,
    status,
    stage: status === 'running' ? 'encoding' : status,
    stageHistory: [],
    progress,
    workerName: status === 'running' ? 'worker-1' : '',
    outputPath: '/library/series/episode-' + id + '.mkv',
    plannedPublishedPath: '',
    errorMessage: '',
    notes: '',
    validationStatus: '',
    validationScore: 0,
    validationReport: {},
    publishedPath: '',
    replacementTargetPath: '',
    originalArchivedPath: '',
    createdAt: updatedAt,
    updatedAt,
    startedAt: status === 'running' ? '2026-10-05T10:00:00Z' : undefined,
  } as unknown as QueueJob;
}

const jobs = [
  job(1, 'running', 45, '2026-10-05T10:04:00Z'),
  job(2, 'queued', 0, '2026-10-05T10:03:00Z'),
  job(3, 'completed', 100, '2026-10-05T10:02:00Z'),
  job(4, 'failed', 100, '2026-10-05T10:01:00Z'),
  job(5, 'canceled', 0, '2026-10-05T10:00:00Z'),
];

const runtimeState = {
  id: 3,
  stateKind: 'live',
  detectedAt: '2026-10-05T10:05:00Z',
  os: 'linux',
  architecture: 'x86_64',
  container: true,
  cpuCores: 8,
  cpuLoad1: 4,
  gpuDetected: true,
  gpuUsageAvailable: true,
  gpuUsagePercent: 35,
  gpuMediaUsagePercent: 30,
  gpuRenderUsagePercent: 10,
  gpuMetricSource: 'runtime',
  totalMemoryBytes: 16 * 1024 ** 3,
  availableMemoryBytes: 8 * 1024 ** 3,
  batteryPresent: false,
  batteryPercent: 0,
  powerSource: 'AC',
  onBattery: false,
  disks: {},
  encoders: { h264_nvenc: { listed: true, usable: true, reason: '' } },
  recommendedProfile: 'nvenc',
  selectedProfile: 'nvenc',
  preferredProfile: 'auto',
  fallbackProfile: 'software',
  appliedOverrides: [],
  effectivePolicy: {},
  selectionReasons: [],
  warnings: [],
  createdAt: '2026-10-05T10:05:00Z',
} as unknown as RuntimeState;

const assets = {
  sourceGroups: [],
  raw: [],
  unprocessed: [],
  library: [],
  converted: [],
  unverified: [{}, {}],
  accepted: [],
  missing: [],
  unprocessedGroups: [],
  rawGroups: [],
  libraryGroups: [],
  convertedGroups: [],
  unverifiedGroups: [],
  acceptedGroups: [],
  reports: {},
  sync: {},
  counts: {
    raw: 2,
    library: 7,
    converted: 5,
    processing: 1,
    needsReview: 2,
    validationReview: 1,
    publicationBlocked: 1,
    originalsArchived: 3,
  },
} as unknown as AssetInventory;

function resolveDashboardQueries() {
  vi.mocked(api.libraries).mockResolvedValue([{ id: 1, name: 'Series', defaultProfileId: 7 }] as never);
  vi.mocked(api.profiles).mockResolvedValue([{ id: 7, name: 'Balanced' }] as never);
  vi.mocked(api.queueJobs).mockResolvedValue(jobs);
  vi.mocked(api.assets).mockResolvedValue(assets);
  vi.mocked(api.runtimeState).mockResolvedValue(runtimeState);
  vi.mocked(api.refreshRuntime).mockResolvedValue(runtimeState);
}

beforeEach(() => {
  vi.mocked(api.libraries).mockReset();
  vi.mocked(api.profiles).mockReset();
  vi.mocked(api.queueJobs).mockReset();
  vi.mocked(api.assets).mockReset();
  vi.mocked(api.runtimeState).mockReset();
  vi.mocked(api.refreshRuntime).mockReset();
  vi.mocked(api.evaluateAdvisor).mockReset();
});

describe('DashboardPage', () => {
  it('presents a compact responsive operational overview and reuses bounded queries and actions', async () => {
    resolveDashboardQueries();

    renderDashboard();

    expect(await screen.findByRole('heading', { level: 1, name: 'Dashboard' })).toBeTruthy();
    expect(screen.getByTestId('page-content-workspace')).toBeTruthy();
    expect(screen.getByTestId('dashboard-overview-grid').className).toContain('MuiGrid-container');
    expect(screen.getByRole('region', { name: 'System & runtime' })).toBeTruthy();
    expect(screen.getByRole('region', { name: 'Queue' })).toBeTruthy();
    expect(screen.getByRole('region', { name: 'Assets & libraries' })).toBeTruthy();
    expect(screen.getByRole('region', { name: 'Storage summary' })).toBeTruthy();

    expect(await screen.findByText('4.00 / 8 cores')).toBeTruthy();
    expect(screen.getByText(/8\.00 GB/)).toBeTruthy();
    expect(screen.getByText('h264_nvenc · usable')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Workers active on jobs: 1' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Queued: 1 jobs' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Canceled: 1 jobs' })).toBeTruthy();
    expect(screen.getByText('Publication blocked · 1')).toBeTruthy();
    expect(screen.getByText('No controlled destinations detected.')).toBeTruthy();
    expect(screen.getByText(/dashboard-wide aggregate is not available/i)).toBeTruthy();

    const mediaLink = screen.getByRole('link', { name: 'episode-1.mkv' });
    expect(mediaLink.getAttribute('href')).toBe('/queue?status=running&job=1');
    expect(screen.getByRole('link', { name: 'Evening series' }).getAttribute('href')).toBe('/queue?batch=batch-42');
    expect(screen.getByRole('link', { name: 'Runtime settings' }).getAttribute('href')).toBe('/settings?section=runtime');
    expect(screen.getAllByRole('link', { name: 'Open Assets' })).toHaveLength(2);

    await waitFor(() => {
      expect(api.libraries).toHaveBeenCalledTimes(1);
      expect(api.profiles).toHaveBeenCalledTimes(1);
      expect(api.queueJobs).toHaveBeenCalledTimes(1);
      expect(api.assets).toHaveBeenCalledTimes(1);
      expect(api.runtimeState).toHaveBeenCalledTimes(1);
    });
    expect(api.evaluateAdvisor).not.toHaveBeenCalled();
  });

  it('keeps available summaries visible when an inventory query fails', async () => {
    resolveDashboardQueries();
    vi.mocked(api.assets).mockRejectedValue(new Error('inventory unavailable'));

    renderDashboard();

    expect(await screen.findByText('Some Dashboard data is temporarily unavailable. Available summaries are still shown.')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Queued: 1 jobs' })).toBeTruthy();
    expect(screen.getByText('Asset inventory is unavailable.')).toBeTruthy();
    expect(screen.getByText('Attention summary is incomplete while source data is unavailable.')).toBeTruthy();
  });

  it('keeps cached inventory visible when a background refresh fails', async () => {
    resolveDashboardQueries();
    vi.mocked(api.assets).mockRejectedValue(new Error('inventory refresh failed'));
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    queryClient.setQueryData(['assets'], assets);

    renderDashboard(queryClient);

    expect(await screen.findByText(/assets in the current inventory/)).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Unprocessed: 2' })).toBeTruthy();
    expect(screen.queryByText('Asset inventory is unavailable.')).toBeNull();
    expect(await screen.findByText('Some Dashboard data is temporarily unavailable. Available summaries are still shown.')).toBeTruthy();
  });

  it('shows compact loading feedback while dashboard sources are pending', async () => {
    vi.mocked(api.libraries).mockImplementation(() => new Promise(() => {}));
    vi.mocked(api.profiles).mockImplementation(() => new Promise(() => {}));
    vi.mocked(api.queueJobs).mockImplementation(() => new Promise(() => {}));
    vi.mocked(api.assets).mockImplementation(() => new Promise(() => {}));
    vi.mocked(api.runtimeState).mockImplementation(() => new Promise(() => {}));

    renderDashboard();

    expect(await screen.findByText('Loading queue summary…')).toBeTruthy();
    expect(screen.getByText('Loading runtime diagnostics…')).toBeTruthy();
    expect(screen.getByText('Loading asset inventory…')).toBeTruthy();
  });
});
