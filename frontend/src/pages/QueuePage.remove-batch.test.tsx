// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import { QueueGroupCard } from './QueuePage';
import { canRemoveQueueBatch } from '../utils/queueBatch';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function job(id: number, status: QueueJob['status']): QueueJob {
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
  };
}

function group(jobs: QueueJob[]) {
  return {
    id: 'batch:batch-1',
    name: 'Batch One',
    jobs,
    isBatch: true,
    batchId: 'batch-1',
    progress: Math.round(jobs.reduce((total, item) => total + item.progress, 0) / jobs.length),
    completed: jobs.filter((item) => item.status === 'completed').length,
    failed: jobs.filter((item) => item.status === 'failed').length,
    running: jobs.filter((item) => item.status === 'running').length,
    queued: jobs.filter((item) => item.status === 'queued').length,
  };
}

function renderGroup(jobs: QueueJob[]) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <QueueGroupCard group={group(jobs)} onEditJob={() => undefined} />
    </QueryClientProvider>,
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
});
