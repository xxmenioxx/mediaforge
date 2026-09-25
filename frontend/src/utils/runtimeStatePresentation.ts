export function liveRuntimeStateLabel(detectedAt: string, formatDate: (value: string) => string): string {
  return `Live runtime state · detected ${formatDate(detectedAt)}`;
}

export const runtimeRefreshSuccessMessage = 'Runtime detection refreshed successfully.';
