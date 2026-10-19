// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FrameCadenceControls } from './FrameCadenceControls';

afterEach(cleanup);

describe('FrameCadenceControls deinterlace parity', () => {
  it('edits parity independently without changing frame structure', async () => {
    const onFieldStructureChange = vi.fn();
    const onParityChange = vi.fn();
    const user = userEvent.setup();
    render(<FrameCadenceControls
      fieldStructureMode="preserve"
      cadenceMode="preserve"
      deinterlaceFieldOrder="auto"
      onFieldStructureChange={onFieldStructureChange}
      onCadenceChange={vi.fn()}
      onDeinterlaceFieldOrderChange={onParityChange}
    />);
    await user.click(screen.getByRole('combobox', { name: 'Deinterlace field order' }));
    await user.click(screen.getByRole('option', { name: 'BFF' }));
    expect(onParityChange).toHaveBeenCalledWith('bff');
    expect(onFieldStructureChange).not.toHaveBeenCalled();
  });

  it('locks parity with the parent asset override read-only state', () => {
    render(<FrameCadenceControls
      fieldStructureMode="deinterlace"
      cadenceMode="preserve"
      deinterlaceFieldOrder="bff"
      onFieldStructureChange={vi.fn()}
      onCadenceChange={vi.fn()}
      onDeinterlaceFieldOrderChange={vi.fn()}
      disabled
    />);
    expect(screen.getByRole('combobox', { name: 'Deinterlace field order' }).getAttribute('aria-disabled')).toBe('true');
  });

  it('offers field order for asset-scoped auto IVTC when analysis recommends IVTC', async () => {
    const onCadenceFieldOrderChange = vi.fn();
    const user = userEvent.setup();
    render(<FrameCadenceControls
      fieldStructureMode="preserve"
      cadenceMode="auto"
      cadenceFieldOrder="auto"
      scan={{ cadenceAnalysis: { type: 'hard_telecine' }, cadenceRecommendation: { operation: 'inverse_telecine' } } as never}
      allowAutomaticIVTCFieldOrder
      onFieldStructureChange={vi.fn()}
      onCadenceChange={vi.fn()}
      onCadenceFieldOrderChange={onCadenceFieldOrderChange}
    />);

    await user.click(screen.getByRole('combobox', { name: 'Inverse telecine field order' }));
    await user.click(screen.getByRole('option', { name: 'BFF' }));

    expect(onCadenceFieldOrderChange).toHaveBeenCalledWith('bff');
  });

  it('does not expose auto IVTC field order in profile controls without asset opt-in', () => {
    render(<FrameCadenceControls
      fieldStructureMode="preserve"
      cadenceMode="auto"
      scan={{ cadenceRecommendation: { operation: 'inverse_telecine' } } as never}
      onFieldStructureChange={vi.fn()}
      onCadenceChange={vi.fn()}
      onCadenceFieldOrderChange={vi.fn()}
    />);

    expect(screen.queryByRole('combobox', { name: 'Inverse telecine field order' })).toBeNull();
  });
});
