// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProfileTemplate } from '../api/types';
import { GuidedProfileTemplateChooser } from './GuidedProfileTemplateChooser';

const template: ProfileTemplate = {
  key: 'archive-quality',
  version: 2,
  name: 'Archive Quality',
  category: 'Quality',
  description: 'Preserve source quality.',
  definition: {
    container: 'mkv',
    videoCodec: 'x265',
    audioCodec: 'copy',
    qualityMode: 'crf',
    qualityValue: 20,
    preserveHdr: true,
    preserveSubtitles: true,
    preserveChapters: true,
  },
};

afterEach(() => cleanup());

describe('GuidedProfileTemplateChooser', () => {
  it('renders templates from the catalog and offers Blank creation', () => {
    const onCreateTemplate = vi.fn();
    const onCreateBlank = vi.fn();
    render(
      <GuidedProfileTemplateChooser
        templates={[template]}
        loading={false}
        creating={false}
        onCreateTemplate={onCreateTemplate}
        onCreateBlank={onCreateBlank}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: /Archive Quality/ }));
    expect(onCreateTemplate).toHaveBeenCalledWith(template);
    fireEvent.click(screen.getByRole('button', { name: /Blank Profile/ }));
    expect(onCreateBlank).toHaveBeenCalledOnce();
  });

  it('shows a catalog error without rendering fallback templates', () => {
    render(
      <GuidedProfileTemplateChooser
        templates={[]}
        loading={false}
        error="service unavailable"
        creating={false}
        onCreateTemplate={vi.fn()}
        onCreateBlank={vi.fn()}
      />,
    );

    expect(screen.getByText(/Could not load profile templates: service unavailable/)).toBeDefined();
    expect(screen.queryByText('Archive Quality')).toBeNull();
    expect(screen.getByRole('button', { name: /Blank Profile/ })).toBeDefined();
  });
});
