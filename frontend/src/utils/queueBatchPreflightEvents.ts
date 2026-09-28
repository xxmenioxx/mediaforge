import type { QueueBatchPreflightResponse } from '../api/types';

export const QUEUE_BATCH_PREFLIGHT_OPEN_EVENT = 'mediaforge:queue-batch-preflight-open';
export const QUEUE_BATCH_PREFLIGHT_STORAGE_KEY = 'mvforge.queueBatchPreflightCandidateId';

export function openQueueBatchPreflight(candidate: QueueBatchPreflightResponse) {
  try {
    window.localStorage.setItem(QUEUE_BATCH_PREFLIGHT_STORAGE_KEY, candidate.id);
  } catch {
    // The persisted server candidate remains addressable from the current session.
  }
  window.dispatchEvent(new CustomEvent<QueueBatchPreflightResponse>(QUEUE_BATCH_PREFLIGHT_OPEN_EVENT, { detail: candidate }));
}
