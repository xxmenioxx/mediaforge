// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { PublisherEligibilityResponse, PublisherPublicationSummary } from '../api/types';
import { PublisherPage } from './PublisherPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <PublisherPage />
    </QueryClientProvider>,
  );
}

describe('PublisherPage persisted delivery controls', () => {
  it('shows a stored publication and sends a reasoned hold action without actor input', async () => {
    const publication: PublisherPublicationSummary = {
      publication: {
        publicationId: 'publication-54-test',
        queueJobId: 54,
        executionNumber: 2,
        executionPlanId: 8,
        libraryId: 3,
        destinationRoot: '/media/library',
        destinationPath: '/media/library/episode.mkv',
        publishMode: 'standard',
        resultArtifactId: 'result:54:2',
        validationRunId: 'validation-run:54:2',
        state: 'READY',
        attemptCount: 0,
        fencingGeneration: 0,
        originalArchiveStatus: 'pending',
        createdAt: '2026-09-27T10:00:00Z',
        updatedAt: '2026-09-27T10:00:00Z',
      },
      attempts: [],
      evidence: [],
      audit: [],
    };
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [], limit: 100 });
    const summaries = vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [publication], limit: 100 });
    const action = vi.spyOn(api, 'publisherAction').mockResolvedValue({ publicationId: 'publication-54-test', state: 'HELD' });
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText('/media/library/episode.mkv')).toBeTruthy();
    expect(screen.queryByText(/overwrite/i)).toBeNull();
    await user.type(screen.getByRole('textbox', { name: 'Reason for Publisher action' }), 'Pause for review');
    await user.click(screen.getByRole('button', { name: 'Hold' }));

	await waitFor(() => expect(action).toHaveBeenNthCalledWith(1, {
		publicationId: 'publication-54-test',
		action: 'hold',
		reason: 'Pause for review',
	}, expect.anything()));
    expect(summaries).toHaveBeenCalled();
  });

  it('shows override-backed eligibility and requires an operator reason to confirm publication', async () => {
    const candidate: PublisherEligibilityResponse['items'][number] = {
      job: {
        id: 1542,
        executionNumber: 4,
        mediaPath: '/raw/episode.mkv',
        outputPath: '/staging/episode.mkv',
        destinationPath: '/library/episode.mkv',
        publishedPath: '',
        validationStatus: 'failed',
        validationScore: 82,
        validationOutcome: 'REVIEW_REQUIRED',
        validationRunId: 'validation-run:1542:4',
      },
      status: 'ALLOWED_BY_OVERRIDE',
    };
    const eligibility = vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [candidate], limit: 100 });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [], limit: 100 });
    const publish = vi.spyOn(api, 'publishJob').mockResolvedValue({
      jobId: 1542,
      status: 'published',
      sourcePath: '/staging/episode.mkv',
      publishedPath: '/library/episode.mkv',
      message: 'Published',
    });
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText('ALLOWED_BY_OVERRIDE')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Publish' }));
    expect(await screen.findByText(/Publisher-scoped Validation overrides/i)).toBeTruthy();
    const confirm = screen.getByRole('button', { name: 'Confirm publish' });
    expect(confirm).toHaveProperty('disabled', true);
    await user.type(screen.getByRole('textbox', { name: 'Operator reason' }), 'Reviewed the findings for this Result');
    await user.click(confirm);

    await waitFor(() => expect(publish).toHaveBeenCalledWith({
      jobId: 1542,
      reason: 'Reviewed the findings for this Result',
    }, expect.anything()));
    await waitFor(() => expect(eligibility).toHaveBeenCalledTimes(2));
    expect(await screen.findByText('Job published.')).toBeTruthy();
  });

  it('shows backend blocking reasons and disables publication for blocked candidates', async () => {
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({
      items: [{
        job: {
          id: 1543,
          mediaPath: '/raw/blocked.mkv',
          outputPath: '/staging/blocked.mkv',
          publishedPath: '',
          validationStatus: 'passed',
          validationScore: 99,
          validationOutcome: 'REVIEW_REQUIRED',
        },
        status: 'BLOCKED',
        reason: 'blocking Validation finding has no eligible override: duration.timeline',
      }],
      limit: 100,
    });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [], limit: 100 });
    renderPage();

    expect(await screen.findByText(/no eligible override: duration\.timeline/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Publish' })).toHaveProperty('disabled', true);
  });
});
