// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import { QueueGroupCard } from './QueuePage';
import { canRemoveQueueBatch } from '../utils/queueBatch';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function job(id: number, status: QueueJob['status'], overrides: Partial<QueueJob> = {}): QueueJob {
  return {
    id,
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
    stage: status,
    stageHistory: [],
    progress: status === 'completed' ? 100 : 0,
    workerName: '',
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
    createdAt: '',
    updatedAt: '',
    ...overrides,
  };
}

function group(jobs: QueueJob[]) {
  return {
    id: 'batch:batch-1',
    name: 'Batch One',
    jobs,
    isBatch: true,
    batchId: 'batch-1',
    completed: jobs.filter((item) => item.status === 'completed').length,
    failed: jobs.filter((item) => item.status === 'failed').length,
    running: jobs.filter((item) => item.status === 'running').length,
    queued: jobs.filter((item) => item.status === 'queued').length,
    canceled: jobs.filter((item) => item.status === 'canceled').length,
  };
}

function renderGroup(jobs: QueueJob[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const view = render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <QueueGroupCard group={group(jobs)} onEditJob={() => undefined} />
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return { ...view, queryClient };
}

describe('Queue batch removal', () => {
  it('allows removal only when every job is canceled', () => {
    expect(canRemoveQueueBatch([job(1, 'canceled'), job(2, 'canceled')])).toBe(true);
    expect(canRemoveQueueBatch([job(1, 'canceled'), job(2, 'completed')])).toBe(false);
    expect(canRemoveQueueBatch([job(1, 'canceled'), job(2, 'failed')])).toBe(false);
    expect(canRemoveQueueBatch([job(1, 'failed')])).toBe(false);
    expect(canRemoveQueueBatch([job(1, 'canceled'), job(2, 'queued')])).toBe(false);
    expect(canRemoveQueueBatch([job(1, 'canceled'), job(2, 'running')])).toBe(false);
    expect(canRemoveQueueBatch([job(1, 'completed')])).toBe(false);
  });

  it('hides Remove Batch for every mixed or non-cancelled batch', () => {
    for (const statuses of [
      ['canceled', 'completed'],
      ['canceled', 'failed'],
      ['failed'],
      ['canceled', 'queued'],
      ['canceled', 'running'],
    ] as QueueJob['status'][][]) {
      renderGroup(statuses.map((status, index) => job(index + 1, status)));
      expect(screen.queryByRole('button', { name: 'Remove Batch' })).toBeNull();
      cleanup();
    }
  });

  it('requires confirmation and refreshes Queue after a successful removal', async () => {
    const dismiss = vi.spyOn(api, 'dismissQueueBatch').mockResolvedValue({
      batchId: 'batch-1',
      dismissedJobs: 2,
      cleanedExecutionPlans: 1,
    });
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const { queryClient } = renderGroup([job(1, 'canceled'), job(2, 'canceled')]);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'Remove Batch' }));
    expect(confirm).toHaveBeenCalledTimes(1);
    expect(confirm.mock.calls[0][0]).toContain('cancelled');
    expect(dismiss).not.toHaveBeenCalled();

    confirm.mockReturnValue(true);
    await user.click(screen.getByRole('button', { name: 'Remove Batch' }));
    await waitFor(() => expect(dismiss).toHaveBeenCalledWith('batch-1'));
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['queueJobs'] });
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['assets'] });
  });

  it('previews a path across batches, lets the operator filter candidates, and dismisses selected stable IDs once', async () => {
    const preview = vi.spyOn(api, 'previewCanceledPath').mockResolvedValue({
      pathGroup: '/media/raw',
      selectedJobs: 2,
      dismissibleJobs: 2,
      skippedJobs: 2,
      alreadyDismissedJobs: 0,
      skippedReasons: [{ reason: 'validation_owned', count: 1 }, { reason: 'active', count: 1 }],
      jobs: [
        { id: 1, status: 'canceled', batchId: 'batch-1', mediaPath: '/media/raw/one.mkv', dismissible: true },
        { id: 2, status: 'canceled', batchId: 'batch-2', mediaPath: '/media/raw/two.mkv', dismissible: true },
        { id: 3, status: 'running', batchId: 'batch-3', mediaPath: '/media/raw/three.mkv', dismissible: false, skipReason: 'active' },
        { id: 4, status: 'canceled', batchId: 'batch-4', mediaPath: '/media/raw/four.mkv', validationRunId: 'validation-44', dismissible: false, skipReason: 'validation_owned' },
      ],
      truncated: false,
    });
    const dismiss = vi.spyOn(api, 'dismissCanceledPath').mockResolvedValue({
      pathGroup: '/media/raw',
      selectedJobs: 1,
      dismissedJobs: 1,
      alreadyDismissedJobs: 0,
      skippedJobs: 0,
      conflictedJobs: 0,
      cleanedExecutionPlans: 1,
      skippedReasons: [],
    });
    const { queryClient } = renderGroup([job(1, 'canceled'), job(2, 'canceled')]);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'Remove canceled in path /media/raw' }));
    await waitFor(() => expect(preview.mock.calls[0]?.[0]).toEqual({ pathGroup: '/media/raw' }));
    expect(await screen.findByText('validation_owned (1) · active (1)')).toBeTruthy();
    expect(screen.getByRole('checkbox', { name: 'Include job #3' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('checkbox', { name: 'Include job #4' })).toHaveProperty('disabled', true);
    await user.click(screen.getByRole('checkbox', { name: 'Include job #2' }));
    await user.click(screen.getByRole('button', { name: 'Remove selected (1)' }));

    await waitFor(() => expect(dismiss.mock.calls[0]?.[0]).toEqual({ pathGroup: '/media/raw', jobIds: [1] }));
    await waitFor(() => {
      expect(invalidateQueries.mock.calls.filter(([options]) => options?.queryKey?.[0] === 'queueJobs')).toHaveLength(1);
      expect(invalidateQueries.mock.calls.filter(([options]) => options?.queryKey?.[0] === 'assets')).toHaveLength(1);
    });
  });

  it('refreshes Queue and Assets once when canceling a job and shows cancellation cleanup state', async () => {
    const runningJob = job(9, 'running', {
      cancellationRequestedAt: '2026-10-27T12:00:00Z',
      cleanupStatus: 'failed',
      cleanupError: 'workspace busy',
    });
    vi.spyOn(api, 'updateQueueJob').mockResolvedValue({ ...runningJob, status: 'canceled' });
    const { queryClient } = renderGroup([runningJob]);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'Expand queue group' }));
    expect(screen.getByText('Stopping')).toBeTruthy();
    expect(screen.getByText(/Cancellation requested · 2026-10-27T12:00:00Z/)).toBeTruthy();
    expect(screen.getByText(/Cleanup · failed/)).toBeTruthy();
    expect(screen.getByText(/Cleanup failed: workspace busy/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(invalidateQueries.mock.calls.filter(([options]) => options?.queryKey?.[0] === 'queueJobs')).toHaveLength(1);
      expect(invalidateQueries.mock.calls.filter(([options]) => options?.queryKey?.[0] === 'assets')).toHaveLength(1);
    });
  });

  it('exposes Dismiss for a completed unpublished Validation exception and invalidates preflight views', async () => {
    const candidate = job(7, 'completed', {
      executionNumber: 7,
      stage: 'validation_review',
      validationRunId: 'validation-run:review-required',
      validationOutcome: 'REVIEW_REQUIRED',
    });
    const dismiss = vi.spyOn(api, 'dismissQueueJob').mockResolvedValue(candidate);
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { queryClient } = renderGroup([candidate]);
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    const user = userEvent.setup();

    await user.click(screen.getByRole('button', { name: 'Expand queue group' }));
    await user.click(screen.getByRole('button', { name: 'Dismiss' }));

    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('staged output'));
    await waitFor(() => expect(dismiss.mock.calls[0]?.[0]).toBe(candidate.id));
    for (const key of ['validationReview', 'validationHistory', 'queueJobs', 'assets', 'queueBatchPreflight', 'publisherEligibility']) {
      expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: [key] });
    }
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull();
  });
});
