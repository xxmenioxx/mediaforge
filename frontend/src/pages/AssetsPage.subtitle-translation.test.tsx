// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EmbeddedSubtitleActions, ExternalSubtitleList } from './AssetsPage';

afterEach(cleanup);

const stream = {
  index: 4,
  codec: 'ass',
  language: 'eng',
  title: 'English forced',
} as never;

const sidecar = {
  path: '/media/episode.eng.forced.ass',
  fileName: 'episode.eng.forced.ass',
  format: 'ass',
  language: 'eng',
  default: false,
  forced: true,
  sizeBytes: 100,
  modifiedAt: '',
} as never;

describe('Asset Info translation source controls', () => {
  it('passes only the exact embedded stream index to Translate...', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <EmbeddedSubtitleActions
        streams={[stream]}
        generations={{}}
        onGenerate={vi.fn()}
        onCancel={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate...' }));
    expect(onTranslate).toHaveBeenCalledWith(4);
  });

  it('passes the exact clicked external sidecar to Translate...', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <ExternalSubtitleList
        values={[sidecar]}
        loading={false}
        deleting={false}
        renaming={false}
        onRename={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate...' }));
    expect(onTranslate).toHaveBeenCalledWith(sidecar);
  });
});
