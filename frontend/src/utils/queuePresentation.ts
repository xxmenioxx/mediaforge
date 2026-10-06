import type { QueueJob } from '../api/types';

export type QueueSemanticColor = 'default' | 'error' | 'info' | 'success' | 'warning';

export function hasDeterminateQueueProgress(progress: number) {
  return Number.isFinite(progress) && progress >= 0 && progress <= 100;
}

export function queueStatusLabel(status: QueueJob['status']) {
  switch (status) {
    case 'queued':
      return 'Queued';
    case 'running':
      return 'Running';
    case 'completed':
      return 'Completed';
    case 'failed':
      return 'Failed';
    case 'canceled':
      return 'Canceled';
  }
}

export function queueStatusColor(status: QueueJob['status']): QueueSemanticColor {
  switch (status) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'running':
      return 'info';
    case 'queued':
    case 'canceled':
      return 'default';
  }
}

export function queueStagePresentation(job: QueueJob): { label: string; color: QueueSemanticColor } {
  if (job.stage) {
    const color = job.stage === 'completed'
      ? 'success'
      : job.stage === 'failed'
        ? 'error'
        : job.stage === 'canceled' || job.stage === 'queued'
          ? 'default'
          : 'info';
    return { label: job.stage.replaceAll('_', ' '), color };
  }

  if (job.publishedAt) {
    return { label: 'Publisher: published', color: 'success' };
  }
  if (job.validationStatus === 'passed' || job.validationStatus === 'warning') {
    return { label: 'Publisher: ready', color: 'info' };
  }
  if (job.status === 'completed') {
    return { label: 'Analysis/Validation: waiting', color: 'info' };
  }
  if (job.status === 'running') {
    return { label: 'Workers: running', color: 'info' };
  }
  if (job.status === 'queued') {
    return { label: 'Queue: waiting', color: 'default' };
  }
  if (job.status === 'failed') {
    return { label: 'Review: failed', color: 'error' };
  }
  if (job.status === 'canceled') {
    return { label: 'Queue: canceled', color: 'default' };
  }
  return { label: 'Assets: queued', color: 'default' };
}
