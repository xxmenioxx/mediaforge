// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return { ...actual, api: { ...actual.api, validationHistory: vi.fn(), jellyfinOutcomes: vi.fn() } };
});

import { api } from '../api/client';
import type { ValidationHistoryPage, ValidationHistorySummary } from '../api/types';
import { HistoryPage } from './HistoryPage';

afterEach(cleanup);

function renderHistory() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}><HistoryPage /></QueryClientProvider>
    </MemoryRouter>,
  );
}

function page(item: ValidationHistorySummary): ValidationHistoryPage {
  return { items: [item], total: 1, limit: 25, offset: 0 };
}

const baseJob = {
  id: 54,
  executionNumber: 3,
  mediaPath: '/raw/series/episode.mkv',
  publishedPath: '',
  status: 'completed' as const,
  createdAt: '2026-09-27T10:00:00Z',
  updatedAt: '2026-09-27T11:00:00Z',
  validationStatus: 'passed',
  validationScore: 100,
  validationRunId: 'validation-run:queue-projection',
  validationOutcome: 'PASS',
  validationOverridePresent: true,
  validationHasActionableWarnings: false,
  originalArchiveStatus: 'failed',
  originalArchivedPath: '/archive/original.mkv',
} as const;

describe('HistoryPage Publisher projection', () => {
  beforeEach(() => {
    vi.mocked(api.validationHistory).mockReset();
    vi.mocked(api.jellyfinOutcomes).mockReset().mockResolvedValue([]);
  });

  it('shows canonical Publisher facts and keeps archive status separate', async () => {
    vi.mocked(api.validationHistory).mockResolvedValue(page({
      ...baseJob,
      publishedPath: '/queue/projection-only.mkv',
      publishedAt: '2026-09-27T10:30:00Z',
      publisher: {
        publicationId: 'publication:canonical-54',
        state: 'PUBLISHED',
        destinationPath: '/library/frozen/episode.mkv',
        publishMode: 'replace_library',
        attemptCount: 2,
        latestAttempt: {
          phase: 'commit',
          outcome: 'published',
          summary: 'Required files and provenance committed.',
          createdAt: '2026-09-27T10:30:00Z',
        },
        validationRunId: 'validation-run:frozen-54',
        validationOverridePresent: true,
        validationOverrideEventIds: ['validation-override:54'],
      },
    }));

    renderHistory();

    expect(await screen.findByText('PUBLISHED')).toBeTruthy();
    expect(screen.getByText('Publication publication:canonical-54')).toBeTruthy();
    expect(screen.getByText('/library/frozen/episode.mkv')).toBeTruthy();
    expect(screen.getByText('Mode: replace_library')).toBeTruthy();
    expect(screen.getByText(/Attempts: 2 · latest published/)).toBeTruthy();
    expect(screen.getByText(/Validation Run validation-run:frozen-54 · override recorded \(validation-override:54\)/)).toBeTruthy();
    expect(screen.getByText('failed')).toBeTruthy();
    expect(screen.getByText('/archive/original.mkv')).toBeTruthy();
    expect(screen.queryByText('/queue/projection-only.mkv')).toBeNull();
  });

  it('labels a historical PublishedAt row as legacy without a canonical Publisher identity or state', async () => {
    vi.mocked(api.validationHistory).mockResolvedValue(page({
      ...baseJob,
      publishedPath: '/library/legacy/episode.mkv',
      publishedAt: '2026-09-26T10:30:00Z',
    }));

    renderHistory();

    expect(await screen.findByText('Legacy publication')).toBeTruthy();
    expect(screen.getByText('/library/legacy/episode.mkv')).toBeTruthy();
    expect(screen.queryByText('PUBLISHED')).toBeNull();
    expect(screen.queryByText(/Publication publication:/)).toBeNull();
  });

  it('shows bounded Jellyfin status and retries only eligible failures', async () => {
    vi.mocked(api.validationHistory).mockResolvedValue(page({
      ...baseJob,
      publisher: {
        publicationId: 'publication:jellyfin', state: 'PUBLISHED', destinationPath: '/library/episode.mkv',
        publishMode: 'replace_library', attemptCount: 1, validationRunId: 'validation-run:1', validationOverridePresent: false,
      },
    }));
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([{
      id: 81, reason: 'asset_published', status: 'retryable_failure', failureClass: 'timeout', attemptCount: 5,
      durationMillis: 10000, updatedAt: '2026-09-27T11:01:00Z', publicationId: 'publication:jellyfin', retryEligible: true,
    }]);

    renderHistory();

    expect(await screen.findByText(/Jellyfin refresh: retryable failure/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Retry Jellyfin refresh' })).toBeTruthy();
    expect(screen.getByText('PUBLISHED')).toBeTruthy();
  });
});
