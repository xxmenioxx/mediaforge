// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { HEVCLevelControls } from './HEVCLevelControls';

const recommendation = {
  version: 1,
  recommendedLevel: '4.0',
  levelIdc: 120,
  tier: 'main' as const,
  width: 1920,
  height: 1080,
  fps: 24,
  bitrate: 6_418_000,
  lumaPictureSize: 2_073_600,
  lumaSampleRate: 49_766_400,
  sourceLevel: '5.0',
  sourceLevelIdc: 150,
  confidence: 'high',
  limitingFactor: 'picture_size',
  reasons: [],
};

afterEach(cleanup);

describe('HEVCLevelControls', () => {
  it('commits the recommended mode and calculated Level atomically', async () => {
    const user = userEvent.setup();
    const onChangeMany = vi.fn();
    render(<HEVCLevelControls config={{}} recommendation={recommendation} encoder="hevc_qsv" onChange={vi.fn()} onChangeMany={onChangeMany} />);

    await user.click(screen.getByRole('combobox', { name: /level mode/i }));
    await user.click(await screen.findByRole('option', { name: /recommended/i }));

    expect(onChangeMany).toHaveBeenCalledWith({ hevcLevelMode: 'recommended', hevcLevel: '4.0' });
    expect(screen.getByDisplayValue(/1920×1080 · 24.000 fps/i)).toBeTruthy();
  });

  it('allows a custom Level for x265', () => {
    render(<HEVCLevelControls config={{ hevcLevelMode: 'custom', hevcLevel: '4.1' }} recommendation={recommendation} encoder="libx265" onChange={vi.fn()} />);
    const level = screen.getByRole('combobox', { name: /^hevc level$/i });
    expect(level.getAttribute('aria-disabled')).not.toBe('true');
    expect(screen.getByText(/custom level is an explicit constraint/i)).toBeTruthy();
  });

  it('keeps explicit Level disabled for an encoder without a validated mapping', () => {
    render(<HEVCLevelControls config={{ hevcLevelMode: 'recommended', hevcLevel: '4.0' }} recommendation={recommendation} encoder="hevc_videotoolbox" onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: /level mode/i }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByText(/does not have a validated explicit-level mapping/i)).toBeTruthy();
  });
});
