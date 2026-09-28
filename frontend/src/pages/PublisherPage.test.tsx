// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { PublisherPublicationSummary } from '../api/types';
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
    vi.spyOn(api, 'queueJobs').mockResolvedValue([]);
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
});
