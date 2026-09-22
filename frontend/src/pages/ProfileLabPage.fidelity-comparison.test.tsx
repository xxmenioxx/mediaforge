// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PreviewVideoCharacteristics } from '../api/types';
import { api } from '../api/client';
import { LAB_COMPARISON_VIEWPORT_POLICY, labComparisonImageSx, labComparisonLayerSx } from '../components/LabComparisonViewport';
import { FrameFidelityComparison, VideoPreviewRequest } from './ProfileLabPage';

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

  it('uses credentialless fetches for compatible media while retaining streaming-capable media elements', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['preview']),
      headers: new Headers(),
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:preview'), revokeObjectURL: vi.fn() });

    render(<VideoPreviewRequest label="Preview" src="/preview.mp4" />);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/preview.mp4', expect.objectContaining({ credentials: 'omit' })));
    expect(screen.getByTestId('lab-comparison-video').getAttribute('crossorigin')).toBe('anonymous');
  });
});
