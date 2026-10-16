// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { SourceBitrateQualityEvidence } from './SourceBitrateQualityEvidence';

afterEach(cleanup);

describe('SourceBitrateQualityEvidence', () => {
  it('explains source provenance and keeps the BPPPF rail contextual', () => {
    const technicalExplanation = 'Estimated target 1.10 Mbps is high-risk against the versioned hevc/software/sdr BPPPF reference for 1920×1080 at 24 fps and the balanced goal.';
    render(<SourceBitrateQualityEvidence
      source={{
        schemaVersion: 1,
        samplingPolicyVersion: 1,
        bitrate: 1_100_000,
        minimumBitrate: 800_000,
        maximumBitrate: 1_300_000,
        method: 'packet_sampling',
        confidence: 'low',
        streamIndex: 0,
        requestedWindows: 5,
        successfulWindows: 4,
        sampledWindows: [],
      }}
      assessment={{
        policyVersion: 1,
        encoderModelVersion: 1,
        status: 'high-risk',
        bpppf: 0.02,
        referenceBpppf: 0.045,
        targetBitrate: 1_100_000,
        bitrateBasis: 'recommendation_estimate',
        outputWidth: 1920,
        outputHeight: 1080,
        effectiveFps: 24,
        codecFamily: 'hevc',
        encoder: 'libx265',
        encoderClass: 'software',
        dynamicRange: 'sdr',
        optimizationIntent: 'balanced',
        explanation: `${technicalExplanation} This is heuristic planning context; measured LAB evidence remains authoritative.`,
        heuristicOnly: true,
      }}
    />);

    expect(screen.getByText('Source video bitrate: 1.10 Mbps')).toBeTruthy();
    expect(screen.getByText('This source bitrate estimate is uncertain; treat it as a guide.')).toBeTruthy();
    expect(screen.getByText('Measured range: 0.80 Mbps–1.30 Mbps')).toBeTruthy();
    expect(screen.getByText('Samples analyzed: 4 of 5')).toBeTruthy();
    expect(screen.getByText('Confidence: Low')).toBeTruthy();
    expect(screen.getByText('Video stream: 0')).toBeTruthy();
    expect(screen.getByText('Evidence method: Packet sampling')).toBeTruthy();
    expect(screen.getByText('Sampling policy version: 1')).toBeTruthy();
    expect(screen.getByText('Encoding estimate: High risk')).toBeTruthy();
    expect(screen.getByText(
      'For this 1920×1080 video at 24 fps, MVForge estimates around 1.10 Mbps as a starting point for the selected Balanced quality goal.',
    )).toBeTruthy();
    expect(screen.getByText('Estimated density: 0.0200 BPPPF')).toBeTruthy();
    expect(screen.getByText('Expected reference: 0.0450 BPPPF')).toBeTruthy();
    expect(screen.getByText('BPPPF policy version: 1 · encoder model version: 1')).toBeTruthy();
    expect(screen.getByText(`Technical explanation: ${technicalExplanation}`)).toBeTruthy();
    expect(screen.getByText('Final quality should be confirmed with Test Encode / LAB results.')).toBeTruthy();
    expect(screen.queryByText(/heuristic planning context|planning heuristic only|measured LAB evidence remains authoritative/i)).toBeNull();
  });

  it('visually distinguishes high source bitrate confidence', () => {
    render(<SourceBitrateQualityEvidence source={{
      schemaVersion: 1,
      samplingPolicyVersion: 1,
      bitrate: 5_910_000,
      method: 'stream_metadata',
      confidence: 'high',
      requestedWindows: 0,
      successfulWindows: 0,
      sampledWindows: [],
    }} />);

    expect(screen.getByText('Confidence: High')).toBeTruthy();
    expect(screen.getAllByRole('alert')[0].className).toContain('MuiAlert-colorSuccess');
  });
});
