import type { AssetInventory, Library, QueueJob } from '../api/types';

export function buildDashboardSummary(libraries: Library[], jobs: QueueJob[], assets?: AssetInventory) {
  const counts = assets?.counts;
  const unprocessedAssets = counts?.raw ?? assets?.raw?.length ?? assets?.unprocessed.length ?? 0;
  const libraryAssets = counts?.library ?? assets?.library?.length ?? assets?.converted.length ?? 0;
  const convertedAssets = counts?.converted ?? assets?.converted.length ?? 0;
  const unverifiedAssets = assets?.unverified?.length ?? Math.max(0, libraryAssets - convertedAssets);
  const processingAssets = counts?.processing ?? 0;
  const needsReviewAssets = counts?.needsReview ?? 0;
  const validationReviewAssets = counts?.validationReview ?? 0;
  const publicationBlockedAssets = counts?.publicationBlocked ?? 0;
  const originalsArchived = counts?.originalsArchived ?? 0;
  const totalAssets = unprocessedAssets + libraryAssets;
  const queuedJobs = jobs.filter((job) => job.status === 'queued').length;
  const runningJobs = jobs.filter((job) => job.status === 'running').length;
  const completedJobs = jobs.filter((job) => job.status === 'completed').length;
  const failedJobs = jobs.filter((job) => job.status === 'failed').length;
  const canceledJobs = jobs.filter((job) => job.status === 'canceled').length;
  const activeJobs = queuedJobs + runningJobs;
  const terminalJobs = completedJobs + failedJobs + jobs.filter((job) => job.status === 'canceled').length;
  const completedPercent = jobs.length > 0 ? Math.round((completedJobs / jobs.length) * 100) : 0;
  const activeWorkers = new Set(jobs.filter((job) => job.status === 'running' && job.workerName).map((job) => job.workerName)).size;
  const recentJobs = [...jobs]
    .sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
    .slice(0, 5);
  const batchSummaries = buildBatchSummaries(jobs);
  const librariesWithoutProfile = libraries.filter((library) => !library.defaultProfileId).length;
  const attentionItems = [
    libraries.length === 0 ? { message: 'No libraries configured yet.', to: '/libraries' } : null,
    librariesWithoutProfile > 0 ? { message: `${librariesWithoutProfile} libraries do not have a default profile.`, to: '/libraries' } : null,
    unprocessedAssets > 0 && queuedJobs === 0 ? { message: `${unprocessedAssets} unprocessed assets are not queued yet.`, to: '/assets' } : null,
    failedJobs > 0 ? { message: `${failedJobs} jobs have failed and need review.`, to: '/queue?status=failed' } : null,
    unverifiedAssets > 0 ? { message: `${unverifiedAssets} library assets have not been verified by MVForge.`, to: '/assets' } : null,
    activeJobs > 0 && activeWorkers === 0 ? { message: 'Queued jobs are waiting for a worker.', to: '/workers' } : null,
    jobs.length > 0 && terminalJobs === jobs.length && failedJobs === 0 ? null : null,
  ].filter((item): item is { message: string; to: string } => Boolean(item));

  return {
    unprocessedAssets, libraryAssets, convertedAssets, unverifiedAssets, processingAssets,
    needsReviewAssets, validationReviewAssets, publicationBlockedAssets, originalsArchived,
    totalAssets, queuedJobs, runningJobs, completedJobs, failedJobs, canceledJobs, activeJobs, activeWorkers,
    completedPercent, recentJobs, batchSummaries, attentionItems,
  };
}

function buildBatchSummaries(jobs: QueueJob[]) {
  const batches = new Map<string, QueueJob[]>();
  jobs.forEach((job) => {
    if (!job.batchId) return;
    batches.set(job.batchId, [...(batches.get(job.batchId) ?? []), job]);
  });
  return [...batches.entries()]
    .map(([id, batchJobs]) => {
      const completed = batchJobs.filter((job) => job.status === 'completed').length;
      const remaining = batchJobs.filter((job) => job.status === 'queued' || job.status === 'running').length;
      const name = batchJobs.find((job) => job.batchName)?.batchName || `Batch ${id}`;
      return { id, name, total: batchJobs.length, completed, remaining, percent: batchJobs.length > 0 ? Math.round((completed / batchJobs.length) * 100) : 0 };
    })
    .filter((batch) => batch.remaining > 0)
    .sort((a, b) => b.remaining - a.remaining || a.name.localeCompare(b.name))
    .slice(0, 6);
}
