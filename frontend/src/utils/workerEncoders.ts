import type { WorkerNode } from '../api/types';

export function encoderNamesForWorker(worker?: WorkerNode) {
  return new Set((worker?.encoders ?? []).filter((value): value is string => typeof value === 'string' && value.trim() !== ''));
}

export function selectedWorker(workers: WorkerNode[] | undefined, requestedName: string) {
  const online = (workers ?? []).filter((worker) => worker.status === 'online');
  return online.find((worker) => worker.name === requestedName) ?? online[0];
}

export function workerSupportsEncoder(worker: WorkerNode | undefined, encoder: string) {
  return Boolean(worker && encoderNamesForWorker(worker).has(encoder));
}
