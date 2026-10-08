// @vitest-environment jsdom

import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { CanonicalOCRReviewTarget, SubtitleExtractionOperation } from '../api/types';

vi.mock('../api/client', () => ({
  api: {
    analyzeSubtitleOCR: vi.fn(),
    subtitleExtractionOperation: vi.fn(),
    cancelSubtitleOperation: vi.fn(),
    correctSubtitleOCR: vi.fn(),
  },
}));

import { api } from '../api/client';
import { SubtitleOCRReviewDialog } from './SubtitleOCRReviewDialog';

const exactTarget: CanonicalOCRReviewTarget = {
  state: 'exact',
  identity: {
    version: 1, sourcePath: '/media/movie.mkv', sourceSize: 20, sourceModTimeUnixNano: 10,
    sourceSnapshotVersion: 'stat-v1', streamIndex: 4, streamId: 'pgs-4', streamCodec: 'hdmv_pgs_subtitle',
    requestedLanguage: 'eng', ocrMode: 'accurate', preparationScope: 'full-asset', segmentStart: '0.000000',
    segmentDuration: 0, preparedStreamMapping: 'subtitle-stream:4', preparedRepresentation: 'prepared-subtitle-v1',
    pipelineProvenance: 'canonical-pipeline', runtimeProvenance: 'test-runtime',
  },
  logicalTransformKey: 'logical-review',
  format: 'srt',
  sidecarPaths: ['/media/movie.eng.srt'],
  baselineRevision: 'baseline-review',
};

function operation(status: SubtitleExtractionOperation['status'], phase: SubtitleExtractionOperation['phase']): SubtitleExtractionOperation {
  return {
    id: 'ocr-review-operation', assetPath: '/media/movie.mkv', status, phase, progress: status === 'running' ? 20 : 100,
    processed: 0, total: 1, streamIndex: 4, format: 'srt', kind: 'ocr_analysis',
    createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:00:01Z',
  };
}

function renderReview(target: CanonicalOCRReviewTarget, onClose = vi.fn()) {
  render(
    <SubtitleOCRReviewDialog
      open
      mediaPath="/media/movie.mkv"
      target={target}
      onClose={onClose}
      onSaved={() => undefined}
    />,
  );
  return onClose;
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => cleanup());

describe('SubtitleOCRReviewDialog', () => {
  it('keeps a missing exact target visible and returns to Translation', async () => {
    const user = userEvent.setup();
    const onClose = renderReview({ state: 'missing', message: 'No compatible canonical OCR sidecar is available.' });
    const dialog = await screen.findByRole('dialog', { name: 'OCR review for Translation' });
    expect(within(dialog).getByText('No compatible canonical OCR sidecar is available.')).toBeTruthy();
    expect(api.analyzeSubtitleOCR).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Return to Translation' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('shows a stale analysis error and keeps a return action available', async () => {
    const user = userEvent.setup();
    vi.mocked(api.analyzeSubtitleOCR).mockRejectedValue(new Error('stale OCR review target; refresh the source revision'));
    renderReview(exactTarget);
    const dialog = await screen.findByRole('dialog', { name: 'OCR review for Translation' });
    expect(await within(dialog).findByText(/stale OCR review target/)).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Return to Translation' })).toBeTruthy();
    await user.click(within(dialog).getByRole('button', { name: 'Return to Translation' }));
  });

  it('shows the no-findings state after exact analysis completes', async () => {
    vi.mocked(api.analyzeSubtitleOCR).mockResolvedValue({ operationId: 'ocr-review-operation', status: 'running', phase: 'analyzing', progress: 20 });
    vi.mocked(api.subtitleExtractionOperation).mockResolvedValue({
      ...operation('completed', 'completed'),
      qualityReport: {
        artifactPath: '/media/movie.eng.srt', format: 'srt', logicalTransformKey: 'logical-review',
        revision: { artifactPath: '/media/movie.eng.srt', artifactIdentity: 'srt', sizeBytes: 10, modifiedAtUnixNano: 10, contentHash: 'hash', logicalTransform: 'logical-review' },
        revisionSet: [], baselineRevision: 'baseline-review', analyzerVersion: 'quality-v1', repairVersion: 'repair-v1',
        cleanCount: 1, suspiciousCount: 0, reviewCount: 0, canCorrect: true,
        findings: [{ cueId: 1, originalText: 'Hello', status: 'clean', baselineRevision: 'baseline-review', startSeconds: 1, endSeconds: 2 }],
      },
    });
    renderReview(exactTarget);
    const dialog = await screen.findByRole('dialog', { name: 'OCR review for Translation' });
    expect(await within(dialog).findByText('No OCR findings require a decision for this revision.')).toBeTruthy();
  });

  it('keeps cancellation visible and offers retry and return actions', async () => {
    const user = userEvent.setup();
    let cancelled = false;
    vi.mocked(api.analyzeSubtitleOCR).mockResolvedValue({ operationId: 'ocr-review-operation', status: 'running', phase: 'analyzing', progress: 20 });
    vi.mocked(api.subtitleExtractionOperation).mockImplementation(async () => cancelled ? operation('cancelled', 'cancelled') : operation('running', 'analyzing'));
    vi.mocked(api.cancelSubtitleOperation).mockImplementation(async () => {
      cancelled = true;
      return operation('cancelled', 'cancelled');
    });
    renderReview(exactTarget);
    const dialog = await screen.findByRole('dialog', { name: 'OCR review for Translation' });
    const cancelButton = await within(dialog).findByRole('button', { name: 'Cancel analysis' });
    await waitFor(() => expect(cancelButton).toHaveProperty('disabled', false));
    await user.click(cancelButton);
    expect(await within(dialog).findByText(/OCR analysis was cancelled/)).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Retry analysis' })).toBeTruthy();
    expect(within(dialog).getByRole('button', { name: 'Return to Translation' })).toBeTruthy();
  }, 10000);
});
