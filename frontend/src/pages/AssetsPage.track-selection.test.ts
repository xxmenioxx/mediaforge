import { describe, expect, it } from 'vitest';
import { withStreamSelection } from '../utils/assetTrackSelection';

describe('withStreamSelection', () => {
  it('preserves an exact audio selection for folder queue persistence', () => {
    expect(withStreamSelection({ keepVideoStreams: [0] }, 'audio', [2, 4])).toEqual({
      keepVideoStreams: [0],
      keepAudioStreams: [2, 4],
    });
  });

  it('preserves an empty selection as remove all instead of inherit all', () => {
    expect(withStreamSelection({}, 'audio', [])).toEqual({ keepAudioStreams: [] });
  });
});
