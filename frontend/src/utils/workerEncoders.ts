import type { WorkerNode } from '../api/types';

export function encoderNamesForWorker(worker?: WorkerNode) {
  return new Set((worker?.encoders ?? []).filter((value): value is string => typeof value === 'string' && value.trim() !== ''));
}

export function selectedWorker(workers: WorkerNode[] | undefined, requestedName: string) {
  const online = (workers ?? []).filter((worker) => worker.status === 'online');
  const requested = requestedName.trim();
  // An explicit worker is part of the requested execution contract. Falling
  // back here made the UI display another host's encoders while preserving the
  // unavailable worker name in the profile.
  return requested ? online.find((worker) => worker.name === requested) : online[0];
}

export function workerSupportsEncoder(worker: WorkerNode | undefined, encoder: string) {
  return Boolean(worker && encoderNamesForWorker(worker).has(encoder));
}
