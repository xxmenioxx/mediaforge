// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FrameStructureControls } from './FrameStructureControls';

afterEach(cleanup);

describe('FrameStructureControls', () => {
  it('commits a numeric B-frame edit as an explicit Custom policy', () => {
    const onChangeMany = vi.fn();
    render(
      <FrameStructureControls
        config={{
          frameStructureMode: 'custom',
          frameStructureBFrameMode: 'custom',
          frameStructureMaxBFrames: 1,
        }}
        onChange={vi.fn()}
        onChangeMany={onChangeMany}
        encoder="hevc_qsv"
      />,
    );

    const input = screen.getByRole('spinbutton', { name: /maximum b-frames/i });
    fireEvent.change(input, { target: { value: '3' } });

    expect(onChangeMany).toHaveBeenLastCalledWith({
      frameStructureMode: 'custom',
      frameStructureBFrameMode: 'custom',
      frameStructureMaxBFrames: 3,
    });
  });
});
