// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiRequestError, api } from '../api/client';
import type { QueueJob } from '../api/types';
import { ValidationPage } from './ValidationPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function job(validationStatus: QueueJob['validationStatus'], validationScore = 0): QueueJob {
  return {
    id: 165,
    batchId: 'validation-test',
    batchName: 'Validation test',
    batchPosition: 1,
    mediaPath: '/media/raw/validation-test.mkv',
    publishMode: 'standard',
    libraryId: 1,
    profileId: 1,
    profileVersion: 1,
    profileSnapshot: {},
    audioProfileKey: '',
    trackProfileKey: '',
    processingMode: 'full_encode',
    priority: 5,
    queuePosition: 1,
    status: 'completed',
    stage: 'ready_to_publish',
    stageHistory: [],
    progress: 100,
    workerName: '',
    outputPath: '/media/staging/validation-test.mkv',
    plannedPublishedPath: '',
    errorMessage: '',
    notes: '',
    validationStatus,
    validationScore,
    validationReport: {},
    publishedPath: '',
    replacementTargetPath: '',
    originalArchivedPath: '',
    createdAt: '2026-09-25T10:00:00Z',
    updatedAt: '2026-09-25T10:00:00Z',
  };
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <ValidationPage />
    </QueryClientProvider>,
  );
}

describe('ValidationPage request errors', () => {
  it('shows the backend error message when the validation request fails', async () => {
    vi.spyOn(api, 'queueJobs').mockResolvedValue([job('pending')]);
    vi.spyOn(api, 'validateJob').mockRejectedValue(
      new ApiRequestError('The output could not be inspected by the backend.', 500),
    );
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run Checks' }));

    expect(await screen.findByText('The output could not be inspected by the backend.')).toBeTruthy();
  });

  it('keeps a normal failed Validation result separate from request errors', async () => {
    const jobs = vi.spyOn(api, 'queueJobs')
      .mockResolvedValueOnce([job('pending')])
      .mockResolvedValue([job('failed', 60)]);
    vi.spyOn(api, 'validateJob').mockResolvedValue({
      jobId: 165,
      status: 'failed',
      score: 60,
      checks: [{ key: 'smart_upscale_output', label: 'Smart Upscale output', status: 'failed', message: 'Output geometry mismatch.' }],
      warnings: [],
      report: {},
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run Checks' }));
    await waitFor(() => expect(jobs).toHaveBeenCalledTimes(2));

    expect(screen.queryByText('Validation could not be completed.')).toBeNull();
    await user.click(await screen.findByRole('tab', { name: 'Validated (1)' }));
    expect((await screen.findAllByText('failed')).length).toBeGreaterThan(0);
    expect(await screen.findByText('60/100')).toBeTruthy();
  });
});
