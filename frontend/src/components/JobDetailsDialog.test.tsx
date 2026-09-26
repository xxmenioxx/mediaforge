// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { QueueJob } from '../api/types';

vi.mock('../api/client', () => ({
  api: {
    jobArtifacts: vi.fn().mockResolvedValue({ warnings: [] }),
    executionPlans: vi.fn().mockResolvedValue([]),
    jobLog: vi.fn().mockResolvedValue({ jobId: 42, available: true, content: 'job log line', truncated: false }),
    reviewExecutionPlan: vi.fn(),
  },
}));

import { JobDetailsDialog } from './JobDetailsDialog';

function jobWithSidecars(): QueueJob {
  return {
    id: 42, batchId: 'batch', batchName: 'Movie', batchPosition: 0, mediaPath: '/raw/Movie.mkv',
    publishMode: 'standard', libraryId: 1, profileId: 1, profileVersion: 1, profileSnapshot: {},
    audioProfileKey: '', trackProfileKey: 'tracks', processingMode: 'full_encode', priority: 5, queuePosition: 1,
    status: 'running', stage: 'preparing_subtitles', stageHistory: [{ stage: 'preparing_subtitles', at: '2026-08-30T12:00:00Z' }],
    progress: 3, workerName: 'worker-1', outputPath: '', plannedPublishedPath: '', errorMessage: '', notes: '',
    validationStatus: 'pending', validationScore: 0, validationReport: {}, publishedPath: '', replacementTargetPath: '',
    originalArchivedPath: '', createdAt: '2026-08-30T12:00:00Z', updatedAt: '2026-08-30T12:00:00Z',
    subtitleArtifacts: [
      { artifactId: 'subtitle:4:original:ass', streamIndex: 4, sourceCodec: 'ass', format: 'ass', mode: 'original', language: 'spa', default: false, forced: false, displayName: 'Movie.spa.ass', stagedPath: '', sizeBytes: 0, status: 'ready' },
      { artifactId: 'subtitle:4:converted:srt', streamIndex: 4, sourceCodec: 'ass', format: 'srt', mode: 'converted', language: 'spa', default: false, forced: false, displayName: 'Movie.spa.srt', stagedPath: '', sizeBytes: 0, status: 'generating' },
      { artifactId: 'subtitle:7:original:srt', streamIndex: 7, sourceCodec: 'subrip', format: 'srt', mode: 'original', language: 'eng', default: false, forced: true, displayName: 'Movie.eng.forced.srt', stagedPath: '', sizeBytes: 0, status: 'planned' },
      { artifactId: 'subtitle:9:converted:srt', streamIndex: 9, sourceCodec: 'ass', format: 'srt', mode: 'converted', language: 'jpn', default: false, forced: false, displayName: 'Movie.jpn.srt', stagedPath: '', sizeBytes: 0, status: 'failed', error: 'conversion failed' },
	  { artifactId: 'font-attachment:11', type: 'font_attachment', streamIndex: 11, attachmentOrdinal: 1, sourceCodec: 'ttf', fontFormat: 'TTF', safeFilename: 'CustomFont.ttf', displayName: 'CustomFont.ttf', stagedPath: '', sizeBytes: 0, status: 'planned' },
	  { artifactId: 'font-attachment:12', type: 'font_attachment', streamIndex: 12, attachmentOrdinal: 2, sourceCodec: 'otf', fontFormat: 'OTF', safeFilename: 'Other.otf', displayName: 'Other.otf', stagedPath: '', sizeBytes: 0, status: 'generating' },
    ],
  } as QueueJob;
}

describe('JobDetailsDialog subtitle stage', () => {
  it('shows canonical sidecar identities and truthful generation states inside the subtitle stage', async () => {
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><JobDetailsDialog job={jobWithSidecars()} onClose={() => undefined} /></QueryClientProvider>);

    expect(await screen.findByText('Movie.spa.ass')).toBeTruthy();
    expect(screen.getByText('ASS · spa · Original')).toBeTruthy();
    expect(screen.getByText('SRT · spa · Compatibility')).toBeTruthy();
    expect(screen.getByText('Generated')).toBeTruthy();
	 expect(screen.getAllByText('Generating')).toHaveLength(2);
	 expect(screen.getAllByText('Planned')).toHaveLength(2);
    expect(screen.getByText('Failed')).toBeTruthy();
	 expect(screen.getByText('CustomFont.ttf')).toBeTruthy();
	 expect(screen.getByText('FONT · TTF')).toBeTruthy();
	 expect(screen.getByText('FONT · OTF')).toBeTruthy();
  });

  it('loads the selected job log only after the Job Log tab opens and supports refresh and copy', async () => {
    const { api } = await import('../api/client');
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><JobDetailsDialog job={jobWithSidecars()} onClose={() => undefined} /></QueryClientProvider>);

    expect(api.jobLog).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('tab', { name: 'Job Log' }));
    expect(await screen.findByText('job log line')).toBeTruthy();
    expect(api.jobLog).toHaveBeenCalledWith(42);
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(api.jobLog).toHaveBeenCalledTimes(2));
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('job log line'));
  });

  it('keeps an explicit unavailable state for missing logs', async () => {
    const { api } = await import('../api/client');
    vi.mocked(api.jobLog).mockResolvedValueOnce({ jobId: 42, available: false, content: '', truncated: false });
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><JobDetailsDialog job={jobWithSidecars()} onClose={() => undefined} /></QueryClientProvider>);
    fireEvent.click(screen.getByRole('tab', { name: 'Job Log' }));
    expect(await screen.findByText('No persisted log is available for this job yet.')).toBeTruthy();
  });

  it('pauses auto-scroll when the reader scrolls away from the bottom', async () => {
    const { api } = await import('../api/client');
    vi.mocked(api.jobLog).mockResolvedValueOnce({ jobId: 42, available: true, content: 'line', truncated: false });
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><JobDetailsDialog job={jobWithSidecars()} onClose={() => undefined} /></QueryClientProvider>);
    fireEvent.click(screen.getByRole('tab', { name: 'Job Log' }));
    await screen.findByText('line');
    const viewer = screen.getAllByLabelText('Job log').at(-1)!;
    Object.defineProperty(viewer, 'scrollHeight', { configurable: true, value: 200 });
    Object.defineProperty(viewer, 'clientHeight', { configurable: true, value: 100 });
    Object.defineProperty(viewer, 'scrollTop', { configurable: true, value: 0 });
    fireEvent.scroll(viewer);
    await waitFor(() => expect(screen.getByRole('checkbox', { name: 'Auto-scroll' })).toHaveProperty('checked', false));
  });

  it('polls a running job log only while the tab is active', async () => {
    const { api } = await import('../api/client');
    vi.mocked(api.jobLog).mockClear();
    vi.useFakeTimers();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
    const view = render(<QueryClientProvider client={client}><JobDetailsDialog job={jobWithSidecars()} onClose={() => undefined} /></QueryClientProvider>);
    fireEvent.click(screen.getByRole('tab', { name: 'Job Log' }));
    await act(async () => { await Promise.resolve(); });
    expect(api.jobLog).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
    expect(api.jobLog).toHaveBeenCalledTimes(2);
    fireEvent.click(screen.getByRole('tab', { name: 'Pipeline' }));
    await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
    expect(api.jobLog).toHaveBeenCalledTimes(2);
    view.unmount();
    vi.useRealTimers();
  });
});
