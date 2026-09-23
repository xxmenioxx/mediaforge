import { ApiRequestError } from './client';

export function isCompatiblePreviewRecoveryError(error: unknown) {
  return error instanceof ApiRequestError && ['interrupted', 'stale_artifact', 'expired', 'unknown_request', 'failed', 'canceled'].includes(error.code ?? error.state ?? '');
}

export function compatiblePreviewRecoveryMessage(error: unknown) {
  if (!(error instanceof ApiRequestError)) {
    return `Fidelity validation failed: ${error instanceof Error ? error.message : 'unknown backend error'}`;
  }
  switch (error.code ?? error.state) {
    case 'interrupted':
      return 'The preview was interrupted by a backend restart. Recreate it to continue validation.';
    case 'stale_artifact':
      return 'The preview artifact is stale. Recreate it to validate the current source and profile.';
    case 'expired':
      return 'The preview request expired. Recreate it to continue validation.';
    case 'unknown_request':
      return 'The preview request is no longer available. Recreate it to continue validation.';
    case 'failed':
    case 'canceled':
      return `The preview generation is ${error.code}. Recreate it to try again.`;
    default:
      return `Fidelity validation failed: ${error.message}`;
  }
}
