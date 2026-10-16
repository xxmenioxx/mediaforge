import type { QueueJob } from '../api/types';

const activeArchiveStatuses = new Set(['pending', 'archiving']);

export function queueJobPublicationChanged(previous: QueueJob[] | undefined, current: QueueJob[] | undefined): boolean {
  if (!previous || !current) return false;
  const previousById = new Map(previous.map((job) => [job.id, job]));
  return current.some((job) => {
    if (job.publicationState !== 'PUBLISHED' || !job.publishedPath || job.publicationRetiredAt) return false;
    const before = previousById.get(job.id);
    return !before || before.publicationState !== 'PUBLISHED' || before.publishedPath !== job.publishedPath || before.publicationId !== job.publicationId;
  });
}

export function hasNonTerminalQueueLifecycle(jobs: QueueJob[] | undefined, autoPublisherEnabled = false): boolean {
  return Boolean(jobs?.some((job) =>
    job.status === 'queued' ||
    job.status === 'running' ||
    job.publicationState === 'PUBLISHING' ||
    (autoPublisherEnabled && job.publicationState === 'READY') ||
    activeArchiveStatuses.has(job.originalArchiveStatus ?? ''),
  ));
}

export function originalArchiveStatusLabel(status: string | undefined): string {
  if (status === 'not_applicable') return 'Not applicable';
  if (status) return status.charAt(0).toUpperCase() + status.slice(1);
  return 'Not recorded';
}
