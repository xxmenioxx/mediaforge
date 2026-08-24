import { describe, expect, it } from 'vitest';
import { avTimingViewModel } from '../utils/avTiming';

describe('avTimingViewModel', () => {
  it('reads timing evidence nested in a validation report', () => {
    const value = avTimingViewModel({ avTiming: { status: 'validated', withinTolerance: true, frameRateUsed: '24000/1001', toleranceMs: 41.7, toleranceFrames: 1, driftStatus: 'not_measured', tracks: [{ sourceAudioIndex: 1, outputAudioIndex: 1, sourceOffsetMs: 80, outputOffsetMs: 80, introducedOffsetMs: 0, introducedOffsetFrames: 0, withinTolerance: true, status: 'validated' }] } });
    expect(value?.status).toBe('validated');
    expect(value?.withinTolerance).toBe(true);
    expect(value?.frameRateUsed).toBe('24000/1001');
    expect(value?.toleranceFrames).toBe(1);
    expect(value?.tracks[0].introducedOffsetMs).toBe(0);
  });
});
