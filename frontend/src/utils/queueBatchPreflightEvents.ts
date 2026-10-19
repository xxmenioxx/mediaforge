import type { QueueBatchPreflightResponse } from '../api/types';

export const QUEUE_BATCH_PREFLIGHT_OPEN_EVENT = 'mediaforge:queue-batch-preflight-open';
export const QUEUE_BATCH_PREFLIGHT_STORAGE_KEY = 'mvforge.queueBatchPreflightCandidateId';

export function createSingleQueueBatchID() {
  return `single-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

const discardedCandidateIDs = new Set<string>();
const maxRememberedDiscardedCandidates = 128;

export function getSavedQueueBatchPreflightCandidateID(): string | null {
  try {
    return window.localStorage.getItem(QUEUE_BATCH_PREFLIGHT_STORAGE_KEY);
  } catch {
    return null;
  }
}

export function clearSavedQueueBatchPreflightCandidate(candidateID: string) {
  try {
    if (window.localStorage.getItem(QUEUE_BATCH_PREFLIGHT_STORAGE_KEY) === candidateID) {
      window.localStorage.removeItem(QUEUE_BATCH_PREFLIGHT_STORAGE_KEY);
    }
  } catch {
    // The current page remains usable when browser storage is unavailable.
  }
}

export function invalidateQueueBatchPreflightReference(candidateID: string) {
  if (discardedCandidateIDs.size >= maxRememberedDiscardedCandidates) {
    const oldest = discardedCandidateIDs.values().next().value;
    if (oldest) discardedCandidateIDs.delete(oldest);
  }
  discardedCandidateIDs.add(candidateID);
  clearSavedQueueBatchPreflightCandidate(candidateID);
}

export function openQueueBatchPreflight(candidate: QueueBatchPreflightResponse) {
  if ((candidate.status && candidate.status !== 'active') || discardedCandidateIDs.has(candidate.id)) return;
  try {
    window.localStorage.setItem(QUEUE_BATCH_PREFLIGHT_STORAGE_KEY, candidate.id);
  } catch {
    // The persisted server candidate remains addressable from the current session.
  }
  window.dispatchEvent(new CustomEvent<QueueBatchPreflightResponse>(QUEUE_BATCH_PREFLIGHT_OPEN_EVENT, { detail: candidate }));
}
