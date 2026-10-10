import type { QueueJob } from '../api/types';

const activeArchiveStatuses = new Set(['pending', 'archiving']);

export function hasNonTerminalQueueLifecycle(jobs: QueueJob[] | undefined): boolean {
  return Boolean(jobs?.some((job) =>
    job.status === 'queued' ||
    job.status === 'running' ||
    job.publicationState === 'PUBLISHING' ||
    activeArchiveStatuses.has(job.originalArchiveStatus ?? ''),
  ));
}

export function originalArchiveStatusLabel(status: string | undefined): string {
  if (status === 'not_applicable') return 'Not applicable';
  if (status) return status.charAt(0).toUpperCase() + status.slice(1);
  return 'Not recorded';
}
