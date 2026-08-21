import type { AssetConversionOverrideState, MediaStreamInfo } from '../api/types';

export function withStreamSelection(
  value: AssetConversionOverrideState,
  type: MediaStreamInfo['type'],
  indexes: number[] | undefined,
) {
  if (type === 'video') return { ...value, keepVideoStreams: indexes };
  if (type === 'audio') return { ...value, keepAudioStreams: indexes };
  return { ...value, keepSubtitleStreams: indexes };
}
