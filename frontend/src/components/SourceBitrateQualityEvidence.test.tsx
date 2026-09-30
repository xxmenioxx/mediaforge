// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { SourceBitrateQualityEvidence } from './SourceBitrateQualityEvidence';

afterEach(cleanup);

describe('SourceBitrateQualityEvidence', () => {
  it('explains source provenance and keeps the BPPPF rail contextual', () => {
    render(<SourceBitrateQualityEvidence
      source={{
        schemaVersion: 1,
        samplingPolicyVersion: 1,
        bitrate: 1_100_000,
        method: 'packet_sampling',
        confidence: 'low',
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
        explanation: 'Estimated target 1.10 Mbps is high-risk at the effective output settings.',
        heuristicOnly: true,
      }}
    />);

    expect(screen.getByText('Source video bitrate: 1.10 Mbps')).toBeTruthy();
    expect(screen.getByText('Evidence: packet sampling · low confidence')).toBeTruthy();
    expect(screen.getByText('Packet windows succeeded: 4/5')).toBeTruthy();
    expect(screen.getByText(/BPPPF planning rail: high-risk/)).toBeTruthy();
    expect(screen.getByText(/Measured LAB evidence remains authoritative/)).toBeTruthy();
  });
});
