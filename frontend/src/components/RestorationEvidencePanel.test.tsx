// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { RestorationEvidencePanel } from './RestorationEvidencePanel';

afterEach(cleanup);

describe('RestorationEvidencePanel', () => {
  it('renders legacy evidence whose supporting evidence is null', () => {
    const unavailable = {
      availability: 'unavailable',
      severity: 'unknown',
      confidence: 'unavailable',
      supportingEvidence: null,
    };
    render(<RestorationEvidencePanel analysis={{
      version: 1,
      status: 'unavailable',
      source: 'sampled_ffmpeg_metrics',
      windows: 0,
      sampledFrames: 0,
      blocking: unavailable,
      noise: unavailable,
      grain: unavailable,
      chromaNoise: unavailable,
      banding: unavailable,
      ringing: unavailable,
      edgeDetailConfidence: unavailable,
    } as unknown as Parameters<typeof RestorationEvidencePanel>[0]['analysis']} />);

    expect(screen.getByText('Restoration evidence')).toBeTruthy();
    expect(screen.getAllByText('Unavailable')).toHaveLength(7);
  });

  it('distinguishes measured, ambiguous and unavailable source evidence', () => {
    render(<RestorationEvidencePanel analysis={{
      version: 1,
      status: 'available',
      source: 'sampled_ffmpeg_metrics',
      windows: 2,
      sampledFrames: 6,
      blocking: { availability: 'available', severity: 'unclassified', value: 11.25, confidence: 'medium', supportingEvidence: ['lavfi.block sampled'] },
      noise: { availability: 'ambiguous', severity: 'unknown', value: 0.03, confidence: 'low', supportingEvidence: ['Cannot separate noise and grain'] },
      grain: { availability: 'ambiguous', severity: 'unknown', value: 0.03, confidence: 'low', supportingEvidence: ['Cannot separate grain and noise'] },
      chromaNoise: { availability: 'ambiguous', severity: 'unknown', value: 0.05, confidence: 'low', supportingEvidence: ['Bit-plane evidence'] },
      banding: { availability: 'unavailable', severity: 'unknown', confidence: 'unavailable', supportingEvidence: ['Black bars are excluded from inference'] },
      ringing: { availability: 'unavailable', severity: 'unknown', confidence: 'unavailable', supportingEvidence: ['No canonical metric'] },
      edgeDetailConfidence: { availability: 'unavailable', severity: 'unknown', confidence: 'unavailable', supportingEvidence: ['Blur is not detail confidence'] },
    }} />);
    expect(screen.getByText('Restoration evidence')).toBeTruthy();
    expect(screen.getAllByText('Unavailable')).toHaveLength(3);
    expect(screen.getAllByText('Ambiguous')).toHaveLength(3);
    expect(screen.getByText('11.250000 · Severity: Unclassified')).toBeTruthy();
    expect(screen.getByText(/do not enable restoration filters/)).toBeTruthy();
  });
});
