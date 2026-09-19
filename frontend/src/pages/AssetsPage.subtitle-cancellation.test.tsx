// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { EmbeddedSubtitleActions, subtitleOperationStopsPolling } from './AssetsPage';

afterEach(cleanup);

const bitmapStream = {
  index: 4,
  codec: 'hdmv_pgs_subtitle',
  language: 'eng',
  title: 'English',
} as never;

function runningGeneration() {
  return {
    '4:srt': {
      status: 'running',
      streamIndex: 4,
      format: 'srt',
      operationId: 'subtitle-operation-1',
      cancelPending: false,
      progress: 42,
      phase: 'ocr',
      processed: 2,
      total: 5,
      message: 'Running OCR…',
    },
  } as never;
}

describe('Asset Info subtitle cancellation controls', () => {
  it('treats cancellation as a terminal polling state', () => {
    expect(subtitleOperationStopsPolling('running')).toBe(false);
    expect(subtitleOperationStopsPolling('completed')).toBe(true);
    expect(subtitleOperationStopsPolling('cancelled')).toBe(true);
    expect(subtitleOperationStopsPolling('error')).toBe(true);
  });

  it('maps Cancel to the current operation identity and disables it while pending', async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();

    const view = render(
      <EmbeddedSubtitleActions
        streams={[bitmapStream]}
        generations={runningGeneration()}
        onGenerate={vi.fn()}
        onCancel={onCancel}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledWith(4, 'srt');

    view.rerender(
      <EmbeddedSubtitleActions
        streams={[bitmapStream]}
        generations={{
          '4:srt': {
            ...((runningGeneration() as Record<string, Record<string, unknown>>)['4:srt']),
            cancelPending: true,
          },
        } as never}
        onGenerate={vi.fn()}
        onCancel={onCancel}
      />,
    );
    expect(screen.getByRole('button', { name: 'Cancelling…' })).toHaveProperty('disabled', true);
  });

  it('shows cancellation as terminal and leaves the generation action available for retry', async () => {
    const user = userEvent.setup();
    const onGenerate = vi.fn();
    const onCancel = vi.fn();

    render(
      <EmbeddedSubtitleActions
        streams={[bitmapStream]}
        generations={{
          '4:srt': {
            status: 'cancelled',
            streamIndex: 4,
            format: 'srt',
            operationId: 'subtitle-operation-1',
            cancelPending: false,
            progress: 42,
            phase: 'cancelled',
            message: 'Subtitle generation cancelled.',
          },
        } as never}
        onGenerate={onGenerate}
        onCancel={onCancel}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
    expect(screen.getByText('Subtitle generation cancelled.')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Generate SRT' }));
    expect(onGenerate).toHaveBeenCalledWith(4, 'srt', 'eng', 'accurate');
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('exposes Analyze OCR for bitmap subtitles and routes the stream/format', async () => {
    const user = userEvent.setup();
    const onAnalyze = vi.fn();

    render(
      <EmbeddedSubtitleActions
        streams={[bitmapStream]}
        generations={{}}
        onGenerate={vi.fn()}
        onCancel={vi.fn()}
        onAnalyze={onAnalyze}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Analyze OCR' }));
    expect(onAnalyze).toHaveBeenCalledWith(4, 'srt');
  });
});
