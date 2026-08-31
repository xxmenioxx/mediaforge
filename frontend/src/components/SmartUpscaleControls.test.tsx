// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SmartUpscaleControls } from './SmartUpscaleControls';

afterEach(cleanup);

describe('SmartUpscaleControls', () => {
  it('shows legacy defaults and every profile mode without a free width control', async () => {
    const user = userEvent.setup();
    render(<SmartUpscaleControls value={{}} onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: 'Upscale' }).textContent).toContain('Disabled');
    expect(screen.getByRole('combobox', { name: 'Sharpen after upscale' }).textContent).toContain('Off');
    await user.click(screen.getByRole('combobox', { name: 'Upscale' }));
    for (const label of ['Disabled', 'Auto / Recommended', '720p', '1080p', 'Custom']) {
      expect(screen.getByRole('option', { name: label })).toBeTruthy();
    }
    expect(screen.queryByLabelText(/width/i)).toBeNull();
  });

  it('shows an even target height only for Custom', () => {
    const { rerender } = render(<SmartUpscaleControls value={{ mode: '720p', sharpen: 'light' }} onChange={vi.fn()} />);
    expect(screen.queryByLabelText(/Target height/)).toBeNull();
    rerender(<SmartUpscaleControls value={{ mode: 'custom', sharpen: 'medium', customHeight: 721 }} onChange={vi.fn()} />);
    expect((screen.getByLabelText(/Target height/) as HTMLInputElement).value).toBe('721');
    expect(screen.getByText('Target height must be even.')).toBeTruthy();
  });

  it('offers inheritance and obeys the parent read-only lock', async () => {
    const user = userEvent.setup();
    render(<SmartUpscaleControls allowInherit disabled value={{}} onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: 'Upscale' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('combobox', { name: 'Sharpen after upscale' }).getAttribute('aria-disabled')).toBe('true');
    await user.click(screen.getByRole('combobox', { name: 'Upscale' }));
    expect(screen.queryByRole('option', { name: 'Inherit' })).toBeNull();
  });
});
