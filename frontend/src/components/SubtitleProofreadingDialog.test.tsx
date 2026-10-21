// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { ExternalSubtitle } from '../api/types';
import { SubtitleProofreadingDialog } from './SubtitleProofreadingDialog';

vi.mock('../api/client', () => ({
  api: {
    startSubtitleProofreading: vi.fn(),
    subtitleOperation: vi.fn(),
    applySubtitleProofreading: vi.fn(),
    restoreSubtitleProofreadingRevision: vi.fn(),
    cancelSubtitleExtractionOperation: vi.fn(),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('SubtitleProofreadingDialog', () => {
  it('previews an exact batch and applies only after explicit confirmation', async () => {
    const user = userEvent.setup();
    const subtitle: ExternalSubtitle = {
      path: '/media/raw/movie.en.srt', fileName: 'movie.en.srt', format: 'srt', language: 'eng',
      default: false, forced: false, sizeBytes: 100, modifiedAt: '',
    };
    vi.mocked(api.startSubtitleProofreading).mockResolvedValue({
      operationId: 'proofread-1', status: 'running', phase: 'preparing', progress: 1, sourceRevision: 'source-revision',
    });
    vi.mocked(api.subtitleOperation).mockResolvedValue({
      id: 'proofread-1', kind: 'proofreading', status: 'completed', phase: 'completed', progress: 100,
      format: 'srt', assetPath: subtitle.path, message: 'Review complete',
      proofreading: {
        sidecarPath: subtitle.path, format: 'srt', mode: 'quick', sourceType: 'imported/external', selfReview: false,
        language: 'en', revision: 'source-revision', provider: 'ollama', model: 'translation', providerState: 'ready',
        totalCount: 4, cleanCount: 3, suggestedCount: 1, reviewCount: 0, unassessedCount: 0,
        findings: [{
          cueId: '2:source-hash', ordinal: 2, sourceTextHash: 'source-hash', originalText: 'I kn0w this.',
          suggestedText: 'I know this.', status: 'suggested', category: 'OCR-like confusion',
          reasonCode: 'confusable-character', reason: 'The zero may be a letter o.',
        }],
      },
    } as never);
    vi.mocked(api.applySubtitleProofreading).mockResolvedValue({
      path: subtitle.path, revisionId: 'revision-1', affectedCueCount: 1, changedCueCount: 1,
      sourceRevision: 'next-source-revision', message: 'Approved text was published.',
    });

    render(<SubtitleProofreadingDialog open path="/media/raw/movie.mkv" subtitle={subtitle} onClose={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Analyze' }));

    expect(await screen.findByText('1 suggested fixes')).toBeTruthy();
    expect(screen.getByText('I kn0w this.')).toBeTruthy();
    expect(screen.getByText('I know this.')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Accept' }));
    expect(screen.getByText('1 cue decisions selected · 1 text changes')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Review selected decisions' }));
    expect(screen.getByText('Confirm 1 cue decisions')).toBeTruthy();
    expect(api.applySubtitleProofreading).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Confirm and apply' }));
    await waitFor(() => expect(api.applySubtitleProofreading).toHaveBeenCalledWith({
      path: '/media/raw/movie.mkv', subtitlePath: subtitle.path, operationId: 'proofread-1',
      revision: 'source-revision', confirmed: true,
      decisions: [{ cueId: '2:source-hash', sourceTextHash: 'source-hash', action: 'accept', text: undefined }],
    }));
  });
});
