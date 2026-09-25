// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PreviewVideoCharacteristics } from '../api/types';
import { api } from '../api/client';
import { LAB_COMPARISON_VIEWPORT_POLICY, labComparisonImageSx, labComparisonLayerSx } from '../utils/labComparisonViewport';
import { FrameFidelityComparison, PerceptualEvidencePanel, VideoPreviewRequest } from './ProfileLabPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function characteristics(width: number, height: number, displayAspectRatio: string, sampleAspectRatio = '1:1'): PreviewVideoCharacteristics {
  return {
    codec: 'h264',
    width,
    height,
    sampleAspectRatio,
    displayAspectRatio,
  };
}

function inspection(source: PreviewVideoCharacteristics, output: PreviewVideoCharacteristics) {
  return {
    completedAt: 1,
    visualAggregate: {
      comparableCount: 1,
      totalCount: 1,
      averageSSIM: 0.99,
      minimumSSIM: 0.99,
      averagePSNR: 40,
      minimumPSNR: 40,
      worstWindowIndex: 0,
      worstPosition: 0.5,
      metricVerdict: 'Safe',
    },
    overallVerdict: 'Safe',
    fidelityWindows: [{
      position: 0.5,
      startSeconds: 0,
      durationSeconds: 1,
      source,
      output,
      metrics: { comparable: true, ssim: 0.99, psnr: 40, reason: 'Measured' },
      options: {},
      requestId: 'request-1',
    }],
  } as unknown as Parameters<typeof FrameFidelityComparison>[0]['inspection'];
}

function canonicalInspection(status: 'passed' | 'mismatch' | 'unverified', aggregateStatus = 'completed') {
  return {
    ...inspection(characteristics(1920, 1080, '16:9'), characteristics(1920, 1080, '16:9')),
    canonicalStatus: status,
    aggregateStatus,
    overallVerdict: aggregateStatus === 'completed' ? status === 'mismatch' ? 'Reject' : status === 'unverified' ? 'Review' : 'Safe' : undefined,
    canonicalFidelity: {
      fields: {
        frameRate: { status },
      },
    },
  } as unknown as Parameters<typeof FrameFidelityComparison>[0]['inspection'];
}

describe('LAB fidelity comparison geometry', () => {
  beforeEach(() => {
    vi.spyOn(api, 'compatibleAssetFrameUrl').mockImplementation((requestId, kind) => `/frames/${requestId}/${kind}`);
  });

  it('uses equal shared viewports for same-resolution frames', () => {
    render(<FrameFidelityComparison inspection={inspection(characteristics(1920, 1080, '16:9'), characteristics(1920, 1080, '16:9'))} />);

    const viewports = screen.getAllByTestId('lab-comparison-viewport');
    expect(viewports).toHaveLength(2);
    expect(viewports.map((viewport) => viewport.getAttribute('data-lab-comparison-viewport'))).toEqual([
      LAB_COMPARISON_VIEWPORT_POLICY,
      LAB_COMPARISON_VIEWPORT_POLICY,
    ]);
  });

  it('keeps mixed native dimensions visible while sharing the display viewport', () => {
    render(<FrameFidelityComparison inspection={inspection(characteristics(720, 480, '3:2'), characteristics(1920, 1080, '16:9'))} />);

    expect(screen.getByText('Native 720×480')).toBeTruthy();
    expect(screen.getByText('Native 1920×1080')).toBeTruthy();
    expect(screen.getAllByTestId('lab-comparison-viewport')).toHaveLength(2);
    expect(screen.getAllByRole('img').every((image) => image.getAttribute('data-lab-comparison-geometry') === 'shared-viewport-contain')).toBe(true);
  });

  it('reports encoded native dimensions instead of SAR-adjusted geometry', () => {
    render(<FrameFidelityComparison inspection={inspection(characteristics(720, 480, '4:3', '8:9'), characteristics(1920, 1080, '16:9'))} />);

    expect(screen.getByText('Native 720×480')).toBeTruthy();
    expect(screen.queryByText('Native 640×480')).toBeNull();
  });

  it('uses contain geometry for different aspect ratios and slider/blink layers', async () => {
    expect(labComparisonImageSx).toMatchObject({ width: '100%', height: '100%', objectFit: 'contain', objectPosition: 'center' });
    expect(labComparisonLayerSx).toMatchObject({ width: '100%', height: '100%', objectFit: 'contain', objectPosition: 'center', position: 'absolute', inset: 0 });

    const user = userEvent.setup();
    render(<FrameFidelityComparison inspection={inspection(characteristics(720, 480, '3:2'), characteristics(2560, 1080, '64:27'))} />);

    await user.click(screen.getByRole('button', { name: 'Before / after' }));
    let layers = screen.getAllByRole('img');
    expect(layers).toHaveLength(2);
    expect(layers.every((image) => image.getAttribute('data-lab-comparison-geometry') === 'shared-viewport-contain')).toBe(true);

    await user.click(screen.getByRole('button', { name: 'Blink' }));
    layers = screen.getAllByRole('img');
    expect(layers).toHaveLength(2);
    expect(layers.every((image) => image.getAttribute('data-lab-comparison-geometry') === 'shared-viewport-contain')).toBe(true);
    expect(screen.getByTestId('lab-comparison-viewport').getAttribute('data-lab-comparison-viewport')).toBe(LAB_COMPARISON_VIEWPORT_POLICY);
  });

  it('uses the stable contain video presentation policy', () => {
    render(<VideoPreviewRequest label="Preview" src="/preview.mp4" direct />);

    const video = screen.getByTestId('lab-comparison-video');
    expect(video.getAttribute('data-lab-comparison-video')).toBe(LAB_COMPARISON_VIEWPORT_POLICY);
    expect(video.getAttribute('crossorigin')).toBe('anonymous');
  });

  it('uses native ready media delivery instead of whole-video Blob loading', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    render(<VideoPreviewRequest label="Preview" src="/preview.mp4" />);

    await waitFor(() => expect(screen.getByTestId('lab-comparison-video').getAttribute('src')).toBe('/preview.mp4'));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByTestId('lab-comparison-video').getAttribute('crossorigin')).toBe('anonymous');
  });

  it('keeps an authoritative canonical mismatch above high visual metrics', () => {
    render(<FrameFidelityComparison inspection={canonicalInspection('mismatch')} />);

    expect(screen.getByText('Overall · Reject')).toBeTruthy();
    expect(screen.getByText(/authoritative output mismatch/i)).toBeTruthy();
  });

  it('renders the backend aggregate verdict without deriving a second policy from windows', () => {
    render(<FrameFidelityComparison inspection={{
      ...canonicalInspection('passed'),
      visualAggregate: {
        comparableCount: 1,
        totalCount: 1,
        averageSSIM: 0.99,
        minimumSSIM: 0.99,
        metricVerdict: 'Safe',
      },
      overallVerdict: 'Reject',
    }} />);

    expect(screen.getByText('Overall · Reject')).toBeTruthy();
    expect(screen.queryByText('Overall · Safe')).toBeNull();
  });

  it('does not expose a final aggregate while windows are still partial', () => {
    render(<FrameFidelityComparison inspection={canonicalInspection('passed', 'analysis_in_progress')} />);

    expect(screen.getByText('Overall · Analyzing')).toBeTruthy();
    expect(screen.getByText(/aggregate remains in progress/i)).toBeTruthy();
  });

  it('keeps canonical unverified results out of Safe', () => {
    render(<FrameFidelityComparison inspection={canonicalInspection('unverified')} />);

    expect(screen.getByText('Overall · Review')).toBeTruthy();
    expect(screen.getByText(/cannot be marked Safe/i)).toBeTruthy();
  });

  it('renders explicit partial perceptual evidence without deriving a verdict', () => {
    render(<PerceptualEvidencePanel evidence={{
      schemaVersion: 'perceptual-evidence-v1',
      policyVersion: 'perceptual-quality-policy-v1',
      identity: 'set-1',
      samplingPlanIdentity: 'plan-1',
      sourceRevision: 'source-1',
      runtimeCapabilityIdentity: 'runtime-1',
      requestedMetrics: [],
      capabilities: [{ metric: 'VMAF', availability: 'unavailable', reason: 'libvmaf absent' }],
      aggregates: [{ logicalMetric: 'PSNR', median: { kind: 'positive_infinity' }, lowerPercentileP10: { kind: 'positive_infinity' }, worstValue: { kind: 'positive_infinity' } }],
      windows: [
        { windowIdentity: 'primary', startSeconds: 0, durationSeconds: 20, primary: true, status: 'covered', candidate: { identity: 'candidate-1' }, pairIdentity: { identity: 'pair-1' }, metrics: [] },
        { windowIdentity: 'hard', startSeconds: 100, durationSeconds: 20, primary: false, status: 'not_covered', comparableReason: 'Test Encode segment does not represent this window.' },
      ],
    }} />);

    expect(screen.getByTestId('perceptual-evidence-panel')).toBeTruthy();
    expect(screen.getByText(/SamplingPlan · plan-1/)).toBeTruthy();
    expect(screen.getByText('not covered')).toBeTruthy();
    expect(screen.getByText(/libvmaf absent/)).toBeTruthy();
    expect(screen.getByText(/PSNR · median \+∞/)).toBeTruthy();
    expect(screen.queryByText(/Safe|Review|Reject/)).toBeNull();
  });

  it('renders stale, unavailable, failed, non-comparable, and infinite evidence without starting work', () => {
    const metric = (logicalMetric: string, status: string, value?: { kind: 'finite' | 'positive_infinity'; value?: number }, reason?: string) => ({
      identity: { identity: `${logicalMetric}-result`, pairIdentity: 'pair-a', metric: { logicalMetric } }, status, value, reason,
    });
    render(<PerceptualEvidencePanel evidence={{
      status: 'stale',
      unavailableReason: 'Frozen source revision no longer matches the current source.',
      schemaVersion: 'perceptual-evidence-v1',
      policyVersion: 'perceptual-quality-policy-v1',
      identity: 'set-stale', samplingPlanIdentity: 'plan-stale', sourceRevision: 'source-old', runtimeCapabilityIdentity: 'runtime-2',
      requestedMetrics: [],
      capabilities: [{ metric: 'VMAF', availability: 'unavailable', reason: 'libvmaf is not installed' }],
      aggregates: [{ logicalMetric: 'PSNR', median: { kind: 'positive_infinity' }, lowerPercentileP10: { kind: 'finite', value: 39 }, worstValue: { kind: 'finite', value: 38 }, worstWindowIdentity: 'hard-b' }],
      windows: [
        { windowIdentity: 'primary', startSeconds: 0, durationSeconds: 20, primary: true, status: 'covered', candidate: { identity: 'candidate-a' }, reference: { identity: 'reference-a' }, pairIdentity: { identity: 'pair-a' }, metrics: [metric('PSNR', 'available', { kind: 'positive_infinity' }), metric('VMAF', 'failed_measurement', undefined, 'provider process failed')] },
        { windowIdentity: 'hard-b', startSeconds: 100, durationSeconds: 20, primary: false, status: 'covered', candidate: { identity: 'candidate-b' }, reference: { identity: 'reference-b' }, pairIdentity: { identity: 'pair-b' }, metrics: [metric('SSIM', 'available', { kind: 'finite', value: 0.97 }), metric('XPSNR', 'non_comparable', undefined, 'canonical reference unavailable')] },
        { windowIdentity: 'hard-c', startSeconds: 200, durationSeconds: 20, primary: false, status: 'not_covered', comparableReason: 'Test Encode segment does not represent this frozen window.' },
      ],
    }} />);

    expect(screen.getByText('Evidence · stale')).toBeTruthy();
    expect(screen.getByText(/Frozen source revision no longer matches/)).toBeTruthy();
    expect(screen.getByText(/libvmaf is not installed/)).toBeTruthy();
    expect(screen.getByText(/VMAF · failed_measurement · provider process failed/)).toBeTruthy();
    expect(screen.getByText(/XPSNR · non_comparable · canonical reference unavailable/)).toBeTruthy();
    expect(screen.getByText(/PSNR · median \+∞/)).toBeTruthy();
    expect(screen.getByText(/worst window hard-b/)).toBeTruthy();
    expect(screen.getByText('not covered')).toBeTruthy();
    expect(screen.queryByText(/Overall · (Safe|Review|Reject)/)).toBeNull();
    expect(api.compatibleAssetFrameUrl).not.toHaveBeenCalled();
  });

  it('keeps legacy Fidelity results without perceptual evidence readable', () => {
    const { container } = render(<PerceptualEvidencePanel />);
    expect(container.firstChild).toBeNull();
  });

  it('does not fetch or build a media element before a request ID is available', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(<VideoPreviewRequest label="Preview" />);

    expect(screen.getByText('Waiting for a valid Compatible Preview request.')).toBeTruthy();
    expect(screen.queryByTestId('lab-comparison-video')).toBeNull();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
