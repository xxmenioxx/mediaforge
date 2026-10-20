import type { QueueJob } from '../api/types';

type ValidationDismissalCandidate = Pick<QueueJob,
  | 'status' | 'stage' | 'validationRunId' | 'validationOutcome' | 'publishedPath' | 'publishedAt' | 'publicationId' | 'publicationState'
  | 'publicationRetiredAt' | 'originalArchivedPath' | 'originalArchiveStatus'
>;

// This is presentation-only eligibility. The backend revalidates the canonical
// ValidationRun, Result, frozen plan, Publisher state, and filesystem revision.
export function canDismissValidationJob(job: ValidationDismissalCandidate): boolean {
  if (job.status !== 'completed' || !job.validationRunId || (job.validationOutcome !== 'FAIL' && job.validationOutcome !== 'REVIEW_REQUIRED')) return false;
  if (job.publishedAt || job.publishedPath || job.publicationId || job.publicationState || job.publicationRetiredAt || job.originalArchivedPath) return false;
  if (job.originalArchiveStatus === 'archiving' || job.originalArchiveStatus === 'complete' || job.originalArchiveStatus === 'failed') return false;
  return !['ready_to_publish', 'publishing', 'archiving_original', 'cleaning_workspace'].includes(job.stage);
}
