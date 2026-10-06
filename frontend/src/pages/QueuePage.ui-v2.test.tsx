// @vitest-environment jsdom

import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import { QueuePage } from './QueuePage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const emptyInventory = { unprocessed: [] } as unknown as Awaited<ReturnType<typeof api.assets>>;

function job(id: number, status: QueueJob['status'], overrides: Partial<QueueJob> = {}): QueueJob {
  const stage = status === 'running' ? 'preparing_workspace' : status;
  return {
    id,
    executionNumber: id,
    batchId: 'batch-1',
    batchName: 'Batch One',
    batchPosition: id,
    mediaPath: `/media/raw/job-${id}.mkv`,
    publishMode: 'standard',
    libraryId: 1,
    profileId: 1,
    profileVersion: 1,
    profileSnapshot: {},
    audioProfileKey: '',
    trackProfileKey: '',
    processingMode: 'full_encode',
    priority: 5,
    queuePosition: id,
    status,
    stage,
    stageHistory: [],
    progress: status === 'running' ? 42 : status === 'completed' ? 100 : 0,
    workerName: status === 'queued' ? '' : 'worker-1',
    outputPath: '',
    plannedPublishedPath: '',
    errorMessage: '',
    notes: '',
    validationStatus: '',
    validationScore: 0,
    validationReport: {},
    publishedPath: '',
    replacementTargetPath: '',
    originalArchivedPath: '',
    createdAt: '2026-10-01T00:00:00.000Z',
    updatedAt: '2026-10-01T00:00:00.000Z',
    ...overrides,
  };
}

function mockQueueApi(jobs: QueueJob[]) {
  vi.spyOn(api, 'queueJobs').mockResolvedValue(jobs);
  vi.spyOn(api, 'libraries').mockResolvedValue([]);
  vi.spyOn(api, 'profiles').mockResolvedValue([]);
  vi.spyOn(api, 'assets').mockResolvedValue(emptyInventory);
  vi.spyOn(api, 'settings').mockResolvedValue([]);
  vi.spyOn(api, 'jobArtifacts').mockResolvedValue({} as Awaited<ReturnType<typeof api.jobArtifacts>>);
  vi.spyOn(api, 'executionPlans').mockResolvedValue([]);
  vi.spyOn(api, 'jobLog').mockResolvedValue({ available: false, content: '', truncated: false } as Awaited<ReturnType<typeof api.jobLog>>);
}

function LocationSearch() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function renderQueue(initialEntry = '/queue') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <Routes>
          <Route path="/queue" element={<><QueuePage /><LocationSearch /></>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

describe('Queue V2 workspace', () => {
  it('shows compact status totals and groups each batch without changing job order within it', async () => {
    const jobs = [
      job(1, 'queued'),
      job(2, 'running'),
      job(3, 'completed'),
      job(4, 'failed'),
      job(5, 'canceled'),
    ];
    mockQueueApi(jobs);
    renderQueue();

    expect(await screen.findByRole('heading', { name: 'Queue', level: 1 })).toBeTruthy();
    await screen.findByTestId('queue-job-1');
    expect(screen.getByTestId('queue-summary-all-jobs').textContent).toContain('5');
    expect(screen.getByTestId('queue-summary-queued').textContent).toContain('1');
    expect(screen.getByTestId('queue-summary-running').textContent).toContain('1');
    expect(screen.getByTestId('queue-summary-completed').textContent).toContain('1');
    expect(screen.getByTestId('queue-summary-failed').textContent).toContain('1');
    expect(screen.getByTestId('queue-summary-canceled').textContent).toContain('1');

    const batch = screen.getByRole('region', { name: 'Batch: Batch One' });
    const rows = within(batch).getAllByRole('listitem');
    expect(rows).toHaveLength(5);
    expect(rows.map((row) => row.getAttribute('data-testid'))).toEqual([
      'queue-job-2', 'queue-job-1', 'queue-job-3', 'queue-job-4', 'queue-job-5',
    ]);
    expect(screen.getByText('Auto worker enabled')).toBeTruthy();
  });

  it('uses neutral queued and info active pills, while keeping running and selected rows distinct', async () => {
    const jobs = [job(1, 'queued'), job(2, 'running')];
    mockQueueApi(jobs);
    renderQueue('/queue?job=1');

    const selected = await screen.findByTestId('queue-job-1');
    const running = screen.getByTestId('queue-job-2');
    expect(selected.getAttribute('aria-current')).toBe('true');
    expect(selected.getAttribute('data-selected')).toBe('true');
    expect(selected.getAttribute('data-running')).toBe('false');
    expect(running.hasAttribute('aria-current')).toBe(false);
    expect(running.getAttribute('data-running')).toBe('true');

    expect(within(selected).getByLabelText('Queue status: Queued').classList.contains('MuiChip-colorDefault')).toBe(true);
    expect(within(running).getByLabelText('Queue status: Running').classList.contains('MuiChip-colorInfo')).toBe(true);
    expect(within(running).getByLabelText('Queue stage: preparing workspace').classList.contains('MuiChip-colorInfo')).toBe(true);
  });

  it('keeps status and worker filter query behavior and opens the selected job details', async () => {
    const user = userEvent.setup();
    mockQueueApi([job(1, 'queued'), job(2, 'running')]);
    renderQueue('/queue?status=queued');

    await screen.findByTestId('queue-job-1');
    await user.click(screen.getByRole('button', { name: 'Running' }));
    expect(screen.getByTestId('location-search').textContent).toBe('?status=running');

    await user.click(screen.getByRole('combobox', { name: 'Worker' }));
    await user.click(screen.getByRole('option', { name: 'worker-1' }));
    expect(screen.getByTestId('location-search').textContent).toBe('?status=running&worker=worker-1');

    await user.click(screen.getByRole('button', { name: 'Details for Job #2' }));
    expect(screen.getByTestId('location-search').textContent).toBe('?status=running&worker=worker-1&job=2');
    expect(await screen.findByRole('heading', { name: 'Job #2' })).toBeTruthy();
    expect(within(screen.getByRole('dialog')).getByLabelText('Queue stage: preparing workspace')).toBeTruthy();
  });

  it('keeps current per-job actions and does not request job details while they are closed', async () => {
    const queued = job(1, 'queued');
    mockQueueApi([queued]);
    const update = vi.spyOn(api, 'updateQueueJob').mockResolvedValue(job(1, 'canceled'));
    const dismiss = vi.spyOn(api, 'dismissQueueJob').mockResolvedValue(job(1, 'canceled'));
    const user = userEvent.setup();
    renderQueue();

    await screen.findByTestId('queue-job-1');
    expect((screen.getByRole('button', { name: 'Details for Job #1' }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole('button', { name: 'Edit Job #1' }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole('button', { name: 'Cancel Job #1' }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole('button', { name: 'Remove Job #1' }) as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole('button', { name: 'Requeue Job #1' }) as HTMLButtonElement).disabled).toBe(true);

    await user.click(screen.getByRole('button', { name: 'Cancel Job #1' }));
    await waitFor(() => expect(update.mock.calls[0]?.[0]).toEqual({ jobId: 1, status: 'canceled' }));
    expect(api.jobArtifacts).not.toHaveBeenCalled();
    expect(api.executionPlans).not.toHaveBeenCalled();
    expect(api.jobLog).not.toHaveBeenCalled();
    expect(dismiss).not.toHaveBeenCalled();
  });

  it('distinguishes an empty Queue from a filter with no matches', async () => {
    mockQueueApi([]);
    const empty = renderQueue();
    expect(await screen.findByText('No conversion jobs have been queued yet.')).toBeTruthy();
    empty.unmount();
    vi.restoreAllMocks();

    mockQueueApi([job(1, 'queued')]);
    renderQueue('/queue?status=failed');
    expect(await screen.findByText('No jobs match the current filter.')).toBeTruthy();
    expect(screen.queryByText('No conversion jobs have been queued yet.')).toBeNull();
  });

  it('shows loading and recoverable API error feedback', async () => {
    vi.spyOn(api, 'queueJobs').mockReturnValue(new Promise<QueueJob[]>(() => undefined));
    vi.spyOn(api, 'libraries').mockResolvedValue([]);
    vi.spyOn(api, 'profiles').mockResolvedValue([]);
    vi.spyOn(api, 'assets').mockResolvedValue(emptyInventory);
    vi.spyOn(api, 'settings').mockResolvedValue([]);
    renderQueue();
    expect(await screen.findByText('Loading Queue…')).toBeTruthy();
    cleanup();
    vi.restoreAllMocks();

    vi.spyOn(api, 'queueJobs').mockRejectedValue(new Error('offline'));
    vi.spyOn(api, 'libraries').mockResolvedValue([]);
    vi.spyOn(api, 'profiles').mockResolvedValue([]);
    vi.spyOn(api, 'assets').mockResolvedValue(emptyInventory);
    vi.spyOn(api, 'settings').mockResolvedValue([]);
    renderQueue();
    expect(await screen.findByText('Unable to load Queue.')).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Retry' }) as HTMLButtonElement).disabled).toBe(false);
  });
});
