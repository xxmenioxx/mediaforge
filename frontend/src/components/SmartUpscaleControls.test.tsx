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

  it('keeps the requested selection when a Copy-style disabled state is removed', () => {
    const value = { mode: '1080p' as const, sharpen: 'light' as const };
    const { rerender } = render(<SmartUpscaleControls disabled value={value} onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: 'Upscale' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('combobox', { name: 'Upscale' }).textContent).toContain('1080p');
    rerender(<SmartUpscaleControls value={value} onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: 'Upscale' }).getAttribute('aria-disabled')).not.toBe('true');
    expect(screen.getByRole('combobox', { name: 'Upscale' }).textContent).toContain('1080p');
  });

  it('offers Custom CAS with 0.01 precision and preserves 0.16', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(<SmartUpscaleControls value={{ mode: '720p', sharpen: 'light' }} onChange={onChange} />);
    await user.click(screen.getByRole('combobox', { name: 'Sharpen after upscale' }));
    await user.click(screen.getByRole('option', { name: 'Custom' }));
    expect(onChange).toHaveBeenCalledWith({ sharpen: 'custom', customSharpenStrength: 0.16 });
    rerender(<SmartUpscaleControls value={{ mode: '720p', sharpen: 'custom', customSharpenStrength: 0.16 }} onChange={onChange} />);
    const input = screen.getByLabelText(/CAS strength/) as HTMLInputElement;
    expect(input.value).toBe('0.16');
    expect(input.step).toBe('0.01');
  });
});
