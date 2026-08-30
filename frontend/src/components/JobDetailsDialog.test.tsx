// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { QueueJob } from '../api/types';

vi.mock('../api/client', () => ({
  api: {
    jobArtifacts: vi.fn().mockResolvedValue({ warnings: [] }),
    executionPlans: vi.fn().mockResolvedValue([]),
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
    expect(screen.getByText('Generating')).toBeTruthy();
    expect(screen.getByText('Planned')).toBeTruthy();
    expect(screen.getByText('Failed')).toBeTruthy();
  });
});
