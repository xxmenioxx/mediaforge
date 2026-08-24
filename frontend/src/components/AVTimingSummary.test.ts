import { describe, expect, it } from 'vitest';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { avTimingViewModel } from '../utils/avTiming';
import { AVTimingSummary } from './AVTimingSummary';

describe('avTimingViewModel', () => {
  it('reads timing evidence nested in a validation report', () => {
    const value = avTimingViewModel({ avTiming: { status: 'validated', withinTolerance: true, frameRateUsed: '24000/1001', toleranceMs: 41.7, toleranceFrames: 1, driftStatus: 'not_measured', tracks: [{ sourceAudioIndex: 1, outputAudioIndex: 1, sourceOffsetMs: 80, outputOffsetMs: 80, introducedOffsetMs: 0, introducedOffsetFrames: 0, withinTolerance: true, status: 'validated' }] } });
    expect(value?.status).toBe('validated');
    expect(value?.withinTolerance).toBe(true);
    expect(value?.frameRateUsed).toBe('24000/1001');
    expect(value?.toleranceFrames).toBe(1);
    expect(value?.tracks[0].introducedOffsetMs).toBe(0);
  });

  it('does not manufacture zero tolerance or offsets for unverified evidence', () => {
	const report = { avTiming: { status: 'unverified', driftStatus: 'not_measured', tracks: [{ sourceAudioIndex: 1, outputAudioIndex: 1, status: 'unverified' }] } };
	const value = avTimingViewModel(report);
	expect(value?.toleranceFrames).toBeNull();
	expect(value?.toleranceMs).toBeNull();
	expect(value?.tracks[0].introducedOffsetMs).toBeNull();
	expect(value?.tracks[0].introducedOffsetFrames).toBeNull();
	const markup = renderToStaticMarkup(createElement(AVTimingSummary, { report }));
	expect(markup).toContain('Tolerance unavailable');
	expect(markup).toContain('Output FPS could not be verified');
	expect(markup).not.toContain('±0 frame');
	expect(markup).not.toContain('0.0 ms at unknown FPS');
  });
});
