// @vitest-environment jsdom

import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import { QueuePage } from './QueuePage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise; });
  return { promise, resolve };
}

const emptyInventory = {
  sourceGroups: [], unprocessed: [], library: [], converted: [], unverified: [], accepted: [], missing: [],
  unprocessedGroups: [], libraryGroups: [], convertedGroups: [], unverifiedGroups: [], acceptedGroups: [], reports: {}, sync: {},
} as never;

const library = {
  id: 1, name: 'Synthetic Movies', sourcePath: '/synthetic/raw', destinationPath: '/synthetic/library',
  type: 'movie', validationRules: {}, createdAt: '', updatedAt: '',
};

const profile = {
  id: 1, name: 'Synthetic H264', videoCodec: 'h264', description: '', container: 'mkv', audioCodec: 'copy',
};

function queueJob(): QueueJob {
  return {
    id: 42, executionNumber: 42, batchId: 'batch-42', batchName: 'Synthetic', batchPosition: 1,
    mediaPath: '/synthetic/raw/input.mkv', publishMode: 'standard', libraryId: 1, profileId: 1,
    profileVersion: 1, profileSnapshot: {}, audioProfileKey: '', trackProfileKey: '', processingMode: 'full_encode',
    priority: 5, queuePosition: 1, status: 'queued', stage: 'queued', stageHistory: [], progress: 0,
    workerName: '', outputPath: '', plannedPublishedPath: '', errorMessage: '', notes: '', validationStatus: '',
    validationScore: 0, validationReport: {}, publishedPath: '', replacementTargetPath: '', originalArchivedPath: '',
    createdAt: '2026-10-01T00:00:00.000Z', updatedAt: '2026-10-01T00:00:00.000Z',
  };
}

function renderQueue() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter><QueuePage /></MemoryRouter>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

function mockBase(jobs: QueueJob[], assets = vi.fn().mockResolvedValue(emptyInventory)) {
  vi.spyOn(api, 'queueJobs').mockResolvedValue(jobs);
  vi.spyOn(api, 'libraries').mockResolvedValue([library] as never);
  vi.spyOn(api, 'profiles').mockResolvedValue([profile] as never);
  vi.spyOn(api, 'assets').mockImplementation(assets as typeof api.assets);
  vi.spyOn(api, 'settings').mockResolvedValue([]);
  vi.spyOn(api, 'updateAssetConversion').mockResolvedValue({} as never);
  vi.spyOn(api, 'jobArtifacts').mockResolvedValue({} as never);
  vi.spyOn(api, 'executionPlans').mockResolvedValue([]);
  vi.spyOn(api, 'jobLog').mockResolvedValue({ available: false, content: '', truncated: false } as never);
  return assets;
}

describe('Queue mutation feedback timing', () => {
  it('shows the authoritative preflight candidate while the asset refetch continues', async () => {
    const assetsRefresh = deferred<Awaited<ReturnType<typeof api.assets>>>();
    let holdAssetsRefresh = false;
    const assets = mockBase([], vi.fn(() => holdAssetsRefresh ? assetsRefresh.promise : Promise.resolve(emptyInventory)));
    const preflightResponse = deferred<Awaited<ReturnType<typeof api.createQueueBatchPreflight>>>();
    const preflight = vi.spyOn(api, 'createQueueBatchPreflight').mockReturnValue(preflightResponse.promise);
    const user = userEvent.setup();
    const { queryClient } = renderQueue();

    await user.click(await screen.findByRole('button', { name: 'Queue Job' }));
    await user.type(screen.getByRole('combobox', { name: 'Media path' }), '/synthetic/raw/input.mkv');
    await user.click(screen.getByRole('combobox', { name: 'Library' }));
    await user.click(await screen.findByRole('option', { name: /Synthetic Movies/ }));
    await user.click(screen.getByRole('combobox', { name: 'Video profile' }));
    await user.click(await screen.findByRole('option', { name: /Synthetic H264/ }));
    await user.click(screen.getByRole('button', { name: /^Queue$/ }));
    await waitFor(() => expect(preflight).toHaveBeenCalledTimes(1));

    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    holdAssetsRefresh = true;
    await act(async () => {
      syntheticNowMs += 120;
      preflightResponse.resolve({
        id: 'candidate-1', revision: 1, resolved: true, status: 'active', batchId: 'batch-1',
        batchName: 'Single asset Queue review', assets: [],
      } as never);
      await Promise.resolve();
    });
    await waitFor(() => expect(assets).toHaveBeenCalledTimes(2));
    const responseAtMs = syntheticNowMs;
    expect(await screen.findByRole('heading', { name: 'Batch Preflight · Single asset Queue review' })).toBeTruthy();
    expect(queryClient.isFetching({ queryKey: ['assets'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;
    const refreshedInventory = { ...(emptyInventory as Record<string, unknown>), reports: { refreshed: true } } as never;

    await act(async () => {
      syntheticNowMs += 240;
      assetsRefresh.resolve(refreshedInventory);
      await assetsRefresh.promise;
    });
    await waitFor(() => expect(queryClient.getQueryData(['assets'])).toEqual(refreshedInventory));
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
  }, 15000);

  it('closes an edited job form on the authoritative response while Queue and asset refetches continue', async () => {
    const queueRefresh = deferred<Awaited<ReturnType<typeof api.queueJobs>>>();
    const assetsRefresh = deferred<Awaited<ReturnType<typeof api.assets>>>();
    let holdQueueRefresh = false;
    let holdAssetsRefresh = false;
    const assets = mockBase([queueJob()], vi.fn(() => holdAssetsRefresh ? assetsRefresh.promise : Promise.resolve(emptyInventory)));
    const queueJobs = vi.mocked(api.queueJobs).mockImplementation(() => holdQueueRefresh ? queueRefresh.promise : Promise.resolve([queueJob()]));
    const updateResponse = deferred<Awaited<ReturnType<typeof api.updateQueueJob>>>();
    const update = vi.spyOn(api, 'updateQueueJob').mockReturnValue(updateResponse.promise);
    const user = userEvent.setup();
    const { queryClient } = renderQueue();

    await user.click(await screen.findByRole('button', { name: 'Edit Job #42' }));
    await user.click(screen.getByRole('button', { name: /^Save$/ }));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));

    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    holdQueueRefresh = true;
    holdAssetsRefresh = true;
    await act(async () => {
      syntheticNowMs += 120;
      updateResponse.resolve(queueJob() as never);
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(queueJobs).toHaveBeenCalledTimes(2);
      expect(assets).toHaveBeenCalledTimes(2);
    });
    const responseAtMs = syntheticNowMs;
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Edit Job #42' })).toBeNull());
    expect(queryClient.isFetching({ queryKey: ['queueJobs'] })).toBe(1);
    expect(queryClient.isFetching({ queryKey: ['assets'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;
    const refreshedJob = { ...queueJob(), notes: 'server-refreshed' };
    const refreshedInventory = { ...(emptyInventory as Record<string, unknown>), reports: { refreshed: true } } as never;

    await act(async () => {
      syntheticNowMs += 240;
      queueRefresh.resolve([refreshedJob]);
      assetsRefresh.resolve(refreshedInventory);
      await Promise.all([queueRefresh.promise, assetsRefresh.promise]);
    });
    await waitFor(() => {
      expect(queryClient.getQueryData(['queueJobs'])).toEqual([refreshedJob]);
      expect(queryClient.getQueryData(['assets'])).toEqual(refreshedInventory);
    });
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
  });
});
