// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RestorationControls } from './RestorationControls';

afterEach(cleanup);

describe('RestorationControls', () => {
  it('offers Off, Light, Medium, Strong and Custom for every structured control', async () => {
    const user = userEvent.setup();
    render(<RestorationControls config={{}} onChange={vi.fn()} />);
    for (const label of ['Deblock', 'HQDN3D', 'Chroma NR', 'Deband']) {
      await user.click(screen.getByRole('combobox', { name: label }));
      for (const option of ['Off', 'Light', 'Medium', 'Strong']) {
        expect(screen.getByRole('option', { name: option })).toBeTruthy();
      }
      expect(screen.getByRole('option', { name: /Custom/ })).toBeTruthy();
      await user.keyboard('{Escape}');
    }
  });

  it('shows only the custom fields requested by each mode', () => {
    const { rerender } = render(<RestorationControls config={{ deblockFilter: 'custom' }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Deblock filter')).toBeTruthy();
    expect(screen.getByLabelText('Block size')).toBeTruthy();
    expect(screen.queryByLabelText('Luma spatial')).toBeNull();

    rerender(<RestorationControls config={{ denoise: 'custom' }} onChange={vi.fn()} />);
    for (const label of ['Luma spatial', 'Chroma spatial', 'Luma temporal', 'Chroma temporal']) expect(screen.getByLabelText(label)).toBeTruthy();

    rerender(<RestorationControls config={{ chromaNR: 'custom' }} onChange={vi.fn()} />);
    for (const label of ['Chroma threshold', 'Window width', 'Window height']) expect(screen.getByLabelText(label)).toBeTruthy();

    rerender(<RestorationControls config={{ deband: 'custom' }} onChange={vi.fn()} />);
    expect((screen.getByLabelText('Deband threshold') as HTMLInputElement).value).toBe('0.024');
  });

  it('keeps configured values visible while disabled', () => {
    render(<RestorationControls disabled config={{ deblockFilter: 'custom', deblockCustomBlockSize: 8 }} onChange={vi.fn()} />);
    expect(screen.getByRole('combobox', { name: 'Deblock' }).textContent).toContain('Custom');
    expect((screen.getByLabelText('Block size') as HTMLInputElement).disabled).toBe(true);
  });

  it('persists the exact defaults when Custom is selected', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<RestorationControls config={{}} onChange={onChange} />);
    await user.click(screen.getByRole('combobox', { name: 'HQDN3D' }));
    await user.click(screen.getByRole('option', { name: 'Custom HQDN3D' }));
    expect(onChange).toHaveBeenCalledWith({
      denoise: 'custom',
      hqdn3dLumaSpatial: 4,
      hqdn3dChromaSpatial: 3,
      hqdn3dLumaTemporal: 6,
      hqdn3dChromaTemporal: 4.5,
    });
  });
});
