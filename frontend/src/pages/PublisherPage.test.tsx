// @vitest-environment jsdom

import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
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
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter><PublisherPage /></MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise; });
  return { promise, resolve };
}

describe('PublisherPage persisted delivery controls', () => {
  it('shows eligibility first, identifies bounded windows, and keeps publication evidence on demand', async () => {
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [{ job: {
      id: 60, executionNumber: 3, mediaPath: '/raw/first.mkv', outputPath: '/stage/first.mkv',
      publishedPath: '', destinationPath: '/library/first.mkv', validationStatus: 'passed', validationScore: 99, validationRunId: 'v60-3',
    }, status: 'ALLOWED' }], limit: 100 });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [{
      publication: { publicationId: 'pub-old', queueJobId: 59, executionNumber: 2, executionPlanId: 1, libraryId: 1,
        destinationRoot: '/library', destinationPath: '/library/old.mkv', publishMode: 'standard', resultArtifactId: 'result',
        validationRunId: 'validation-old', state: 'PUBLISHED', attemptCount: 1, fencingGeneration: 1,
        originalArchiveStatus: 'complete', createdAt: '', updatedAt: '' },
      attempts: [], evidence: [{ attemptId: 'a', phase: 'delivery', outcome: 'published', createdAt: 'today' }], audit: [],
    }], limit: 100 });
    renderPage();
    const first = await screen.findByText('first.mkv');
    await userEvent.setup().click(screen.getByRole('combobox', { name: 'State' }));
    await userEvent.setup().click(screen.getByRole('option', { name: 'Published' }));
    const old = screen.getByText('old.mkv');
    expect(first.compareDocumentPosition(old) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByText(/Eligibility is limited to 100 jobs/)).toBeTruthy();
    expect(screen.getByText(/Evidence · 1 attempts/).closest('details')).toHaveProperty('open', false);
    await userEvent.setup().click(screen.getByText(/Evidence · 1 attempts/));
    expect(screen.getByText(/delivery: published/)).toBeTruthy();
  });

  it('does not duplicate an eligibility row for an exact same execution and retains separate publication IDs', async () => {
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [{ job: {
      id: 60, executionNumber: 3, mediaPath: '/raw/first.mkv', outputPath: '/stage/first.mkv', publishedPath: '',
      destinationPath: '/library/first.mkv', validationStatus: 'passed', validationScore: 99, validationRunId: 'v60-3',
    }, status: 'ALLOWED' }], limit: 100 });
    const publication = (id: string) => ({ publication: {
      publicationId: id, queueJobId: 60, executionNumber: 3, executionPlanId: 1, libraryId: 1, destinationRoot: '/library',
      destinationPath: '/library/first.mkv', publishMode: 'standard' as const, resultArtifactId: 'result', validationRunId: 'validation',
      state: 'FAILED' as const, attemptCount: 1, fencingGeneration: 1, originalArchiveStatus: 'pending', createdAt: '', updatedAt: '',
    }, attempts: [], evidence: [], audit: [] });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [publication('pub-a'), publication('pub-b')], limit: 100 });
    renderPage();
    expect((await screen.findAllByText(/Job #60 · Run 3/)).length).toBe(2);
    expect(screen.getByText('Publication pub-a')).toBeTruthy();
    expect(screen.getByText('Publication pub-b')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Discard' })).toBeNull();
  });

  it('prioritizes Ready, Publishing, Failed/Blocked, and Held across the unified asset list', async () => {
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [
      { job: { id: 72, executionNumber: 1, mediaPath: '/raw/blocked.mkv', outputPath: '/stage/blocked.mkv', publishedPath: '', destinationPath: '/library/blocked.mkv', validationStatus: 'failed', validationScore: 70 }, status: 'BLOCKED' },
      { job: { id: 71, executionNumber: 1, mediaPath: '/raw/ready.mkv', outputPath: '/stage/ready.mkv', publishedPath: '', destinationPath: '/library/ready.mkv', validationStatus: 'passed', validationScore: 99 }, status: 'ALLOWED' },
    ], limit: 100 });
    const publication = (publicationId: string, queueJobId: number, fileName: string, state: PublisherPublicationSummary['publication']['state']): PublisherPublicationSummary => ({
      publication: {
        publicationId, queueJobId, executionNumber: 1, executionPlanId: 1, libraryId: 1,
        destinationRoot: '/library', destinationPath: `/library/${fileName}`, publishMode: 'standard',
        resultArtifactId: `result-${queueJobId}`, validationRunId: `validation-${queueJobId}`, state,
        attemptCount: 1, fencingGeneration: 1, originalArchiveStatus: 'pending', createdAt: '', updatedAt: '',
      },
      attempts: [], evidence: [], audit: [],
    });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [
      publication('held', 75, 'held.mkv', 'HELD'),
      publication('failed', 74, 'failed.mkv', 'FAILED'),
      publication('publishing', 73, 'publishing.mkv', 'PUBLISHING'),
    ], limit: 100 });
    renderPage();

    const firstAsset = await screen.findByText('ready.mkv');
    const rowOrder = Array.from(firstAsset.closest('tbody')!.querySelectorAll('tr')).map((row) => row.textContent ?? '');
    expect(rowOrder.map((row) => row.match(/(?:ready|publishing|blocked|failed|held)\.mkv/)?.[0])).toEqual([
      'ready.mkv', 'publishing.mkv', 'blocked.mkv', 'failed.mkv', 'held.mkv',
    ]);
  });

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
      requestId: 'manual-request-1542',
      publicationId: 'publication-1542',
      status: 'accepted',
      message: 'Manual publication request was durably accepted; delivery has not completed.',
    });
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText(/Eligibility: ALLOWED_BY_OVERRIDE/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Publish' }));
    expect(await screen.findByText(/Publisher-scoped Validation overrides/i)).toBeTruthy();
    const confirm = screen.getByRole('button', { name: 'Accept manual Publish' });
    expect(confirm).toHaveProperty('disabled', true);
    await user.type(screen.getByRole('textbox', { name: 'Operator reason' }), 'Reviewed the findings for this Result');
    await user.click(confirm);

    await waitFor(() => expect(publish).toHaveBeenCalledWith(expect.objectContaining({
      jobId: 1542,
      reason: 'Reviewed the findings for this Result',
      idempotencyKey: expect.any(String),
    }), expect.anything()));
    await waitFor(() => expect(eligibility).toHaveBeenCalledTimes(2));
    expect(await screen.findByText(/Manual Publish accepted\. Delivery is still in progress/)).toBeTruthy();
    expect(screen.queryByText('Job published.')).toBeNull();
  });

  it('records accepted feedback from the 202 response while Publisher refreshes reconcile in the background', async () => {
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
    const publishResponse = deferred<Awaited<ReturnType<typeof api.publishJob>>>();
    const eligibilityRefresh = deferred<PublisherEligibilityResponse>();
    const publicationsRefresh = deferred<Awaited<ReturnType<typeof api.publisherPublications>>>();
    const eligibility = vi.spyOn(api, 'publisherEligibility')
      .mockResolvedValueOnce({ items: [candidate], limit: 100 })
      .mockReturnValueOnce(eligibilityRefresh.promise);
    const publications = vi.spyOn(api, 'publisherPublications')
      .mockResolvedValueOnce({ items: [], limit: 100 })
      .mockReturnValueOnce(publicationsRefresh.promise);
    const publish = vi.spyOn(api, 'publishJob').mockReturnValue(publishResponse.promise);
    const user = userEvent.setup();
    const { queryClient } = renderPage();

    expect(await screen.findByText(/Eligibility: ALLOWED_BY_OVERRIDE/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Publish' }));
    await user.type(screen.getByRole('textbox', { name: 'Operator reason' }), 'Reviewed the findings for this Result');
    await user.click(screen.getByRole('button', { name: 'Accept manual Publish' }));
    await waitFor(() => expect(publish).toHaveBeenCalled());
    expect(screen.getByRole('button', { name: 'Submitting…' })).toBeTruthy();

    // Deterministic synthetic timing: the request consumes 120 ms; the active
    // eligibility refetch is then held for another 240 ms as a read-model delay.
    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    await act(async () => {
      syntheticNowMs += 120;
      publishResponse.resolve({
        jobId: 1542,
        requestId: 'manual-request-1542',
        publicationId: 'publication-1542',
        status: 'accepted',
        message: 'Manual publication request was durably accepted; delivery has not completed.',
      });
      await Promise.resolve();
    });
    await waitFor(() => expect(eligibility).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(publications).toHaveBeenCalledTimes(2));
    const responseAtMs = syntheticNowMs;
    expect(await screen.findByText(/Manual Publish accepted\. Delivery is still in progress/)).toBeTruthy();
    expect(screen.queryByText('Job published.')).toBeNull();
    expect(queryClient.isFetching({ queryKey: ['publisherEligibility'] })).toBe(1);
    expect(queryClient.isFetching({ queryKey: ['publisherPublications'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;

    await act(async () => {
      syntheticNowMs += 240;
      eligibilityRefresh.resolve({ items: [], limit: 100 });
      publicationsRefresh.resolve({ items: [], limit: 100 });
      await Promise.all([eligibilityRefresh.promise, publicationsRefresh.promise]);
    });
    await waitFor(() => {
      expect(queryClient.getQueryData(['publisherEligibility'])).toEqual({ items: [], limit: 100 });
      expect(queryClient.getQueryData(['publisherPublications'])).toEqual({ items: [], limit: 100 });
    });
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
    expect(screen.queryByText('Job published.')).toBeNull();
  });

  it('does not show accepted or published feedback when the Publisher request fails', async () => {
    const candidate: PublisherEligibilityResponse['items'][number] = {
      job: {
        id: 1544, executionNumber: 4, mediaPath: '/raw/episode.mkv', outputPath: '/staging/episode.mkv',
        destinationPath: '/library/episode.mkv', publishedPath: '', validationStatus: 'failed', validationScore: 82,
        validationOutcome: 'REVIEW_REQUIRED', validationRunId: 'validation-run:1544:4',
      },
      status: 'ALLOWED_BY_OVERRIDE',
    };
    vi.spyOn(api, 'publisherEligibility').mockResolvedValue({ items: [candidate], limit: 100 });
    vi.spyOn(api, 'publisherPublications').mockResolvedValue({ items: [], limit: 100 });
    vi.spyOn(api, 'publishJob').mockRejectedValue(new Error('synthetic HTTP 500'));
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText(/Eligibility: ALLOWED_BY_OVERRIDE/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Publish' }));
    await user.type(screen.getByRole('textbox', { name: 'Operator reason' }), 'Reviewed synthetic evidence');
    await user.click(screen.getByRole('button', { name: 'Accept manual Publish' }));

    expect(await screen.findByText('synthetic HTTP 500')).toBeTruthy();
    expect(screen.queryByText(/Manual Publish accepted\. Delivery is still in progress/)).toBeNull();
    expect(screen.queryByText('Job published.')).toBeNull();
    expect(screen.getByRole('dialog')).toBeTruthy();
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
