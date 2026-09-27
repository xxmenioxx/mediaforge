// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiRequestError, api } from '../api/client';
import type { ValidationSummary } from '../api/types';
import { ValidationPage } from './ValidationPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function job(validationStatus: ValidationSummary['validationStatus'], validationScore = 0): ValidationSummary {
  return {
    id: 165,
    mediaPath: '/media/raw/validation-test.mkv',
    status: 'completed',
    outputPath: '/media/staging/validation-test.mkv',
    validationStatus,
    validationScore,
    validationRunId: '',
    validationOutcome: '',
    validationOverridePresent: false,
    validationHasActionableWarnings: false,
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
    vi.spyOn(api, 'validationReview').mockResolvedValue({ items: [job('pending')], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'validateJob').mockRejectedValue(
      new ApiRequestError('The output could not be inspected by the backend.', 500),
    );
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run validation' }));

    expect(await screen.findByText('The output could not be inspected by the backend.')).toBeTruthy();
  });

  it('keeps a normal failed Validation result separate from request errors', async () => {
    const jobs = vi.spyOn(api, 'validationReview')
      .mockResolvedValueOnce({ items: [job('pending')], total: 1, limit: 25, offset: 0 })
      .mockResolvedValue({ items: [{ ...job('failed', 60), validationOutcome: 'FAIL', validationRunId: 'validation-run:test' }], total: 1, limit: 25, offset: 0 });
    vi.spyOn(api, 'validateJob').mockResolvedValue({
      jobId: 165,
      status: 'failed',
      outcome: 'FAIL',
      runId: 'validation-run:test',
      score: 60,
      checks: [{ key: 'video.properties', label: 'Video properties', status: 'failed', message: 'Output geometry mismatch.' }],
      warnings: [],
      report: {},
      findings: [],
    });
    const user = userEvent.setup();
    renderPage();

    await user.click(await screen.findByRole('button', { name: 'Run validation' }));
    await waitFor(() => expect(jobs).toHaveBeenCalledTimes(2));

    expect(screen.queryByText('Validation could not be completed.')).toBeNull();
    expect(await screen.findByText('FAIL')).toBeTruthy();
    expect(await screen.findByText('60/100')).toBeTruthy();
  });
});
