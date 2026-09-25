import type { QueueJob } from '../api/types';

export function canRemoveQueueBatch(jobs: QueueJob[]) {
  return jobs.length > 0 && jobs.every((job) => job.status === 'canceled');
}
