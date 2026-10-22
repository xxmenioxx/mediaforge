// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { ExternalSubtitle, MediaStreamInfo, SubtitleSyncProposal } from '../api/types';
import { SubtitleSyncDialog } from './SubtitleSyncDialog';

vi.mock('../api/client', () => ({
  api: {
    subtitleSyncCapability: vi.fn(),
    previewSubtitleSync: vi.fn(),
  },
}));

afterEach(cleanup);

const subtitle: ExternalSubtitle = {
  path: '/media/episode.es.srt',
  fileName: 'episode.es.srt',
  format: 'srt',
  language: 'spa',
  default: false,
  forced: false,
  sizeBytes: 128,
  modifiedAt: '2026-10-22T00:00:00Z',
};

const audio: MediaStreamInfo = {
  index: 2,
  type: 'audio',
  codec: 'ac3',
  codecLong: 'ATSC A/52A',
  profile: 'Dolby Digital',
  language: 'spa',
  title: 'Spanish 5.1',
  duration: 1200,
  bitrate: 640_000,
  default: false,
  forced: false,
  comment: false,
  hearingImpaired: false,
  channels: 6,
  channelLayout: '5.1',
};

const proposal: SubtitleSyncProposal = {
  proposalId: 'proposal-1',
  subtitlePath: subtitle.path,
  sourceIdentity: 'external:/media/episode.es.srt',
  sourceRevision: 'revision-1',
  representation: 'srt',
  referenceAudio: { streamIndex: audio.index, streamId: '0x2', codec: audio.codec, language: audio.language, title: audio.title },
  engine: 'manual-fixed-offset',
  engineVersion: '1',
  model: 'fixed_offset',
  offsetMilliseconds: 1000,
  scale: 1,
  segments: [],
  qualityState: 'review_recommended',
  analyzedAt: '2026-10-22T00:00:00Z',
  diagnostics: ['Manual fixed offset; review the preview before applying.'],
  preview: [{ cueIndex: 1, currentStartMs: 1000, currentEndMs: 2500, proposedStartMs: 2000, proposedEndMs: 3500 }],
};

function renderDialog(onApply = vi.fn().mockResolvedValue(undefined)) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <SubtitleSyncDialog
        open
        path="/media/episode.mkv"
        subtitle={subtitle}
        audioStreams={[audio]}
        onClose={() => undefined}
        onApply={onApply}
      />
    </QueryClientProvider>,
  );
  return { onApply, queryClient };
}

describe('SubtitleSyncDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.subtitleSyncCapability).mockResolvedValue({
      enabled: true,
      fixedOffset: true,
      automaticAlignment: false,
      segmentedAlignment: false,
      engines: [],
    });
    vi.mocked(api.previewSubtitleSync).mockResolvedValue(proposal);
  });

  it('requires explicit audio selection and review before applying a fixed offset', async () => {
    const user = userEvent.setup();
    const { onApply } = renderDialog();

    const analyzeButton = await screen.findByRole('button', { name: 'Preview offset' });
    expect(analyzeButton).toHaveProperty('disabled', true);
    expect(screen.queryByRole('button', { name: 'Apply Sync' })).toBeNull();
    expect(screen.getByText(/Automatic alignment is unavailable/)).toBeTruthy();

    await user.click(screen.getByRole('combobox', { name: 'Reference audio' }));
    await user.click(await screen.findByRole('option', { name: '#2 · spa · AC3 · Spanish 5.1' }));
    await user.type(screen.getByRole('spinbutton', { name: 'Offset (milliseconds)' }), '1000');
    await waitFor(() => expect(analyzeButton).toHaveProperty('disabled', false));

    await user.click(analyzeButton);
    expect(api.previewSubtitleSync).toHaveBeenCalledWith({
      path: '/media/episode.mkv',
      subtitlePath: subtitle.path,
      referenceAudioStreamIndex: audio.index,
      offsetMilliseconds: 1000,
    }, expect.anything());
    expect(await screen.findByRole('table', { name: 'Subtitle timing preview' })).toBeTruthy();
    expect(screen.getByText('00:00:01.000 – 00:00:02.500')).toBeTruthy();
    expect(screen.getByText('00:00:02.000 – 00:00:03.500')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Apply Sync' }));
    await waitFor(() => expect(onApply).toHaveBeenCalledWith(proposal, expect.anything()));
  });
});
