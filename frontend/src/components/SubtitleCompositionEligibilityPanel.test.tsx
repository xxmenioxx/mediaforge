// @vitest-environment jsdom

import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { SubtitleCompositionEligibility } from '../api/types';
import { SubtitleCompositionEligibilityPanel } from './SubtitleCompositionEligibilityPanel';

const eligible: SubtitleCompositionEligibility = {
  eligible: true,
  policyVersion: 'subtitle-composition-v1',
  maxSupplementals: 8,
  primaryCandidates: [{ sourceIdentity: 'primary', sourceRevision: 'r1', representation: 'ass', language: 'en', role: 'full', compositionClass: 'dialogue', sourceType: 'embedded', sourcePath: '', streamIndex: 4, eligible: true }],
  supplementalCandidates: [{ sourceIdentity: 'signs', sourceRevision: 'r2', representation: 'ass', language: 'en', role: 'signs', compositionClass: 'sign', sourceType: 'external', sourcePath: '/asset.signs.ass', eligible: true }],
  crossLanguageExplicit: false,
};

describe('SubtitleCompositionEligibilityPanel', () => {
  it('uses backend eligibility and does not invent a local candidate decision', () => {
    render(<SubtitleCompositionEligibilityPanel eligibility={{ ...eligible, eligible: false, reason: 'unknown language' }} />);
    expect(screen.getByText('unknown language')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Compose subtitles' })).toBeNull();
  });

  it('starts an eligible composition with backend source fields and observed revisions', () => {
    const onCompose = vi.fn();
    render(<SubtitleCompositionEligibilityPanel assetPath="/asset.mkv" eligibility={eligible} onCompose={onCompose} />);
    fireEvent.click(screen.getByLabelText(/signs/));
    fireEvent.click(screen.getByRole('button', { name: 'Compose subtitles' }));
    expect(onCompose).toHaveBeenCalledWith(expect.objectContaining({
      path: '/asset.mkv',
      observedRevisions: { primary: 'r1', signs: 'r2' },
      primary: expect.objectContaining({ identity: 'primary', revision: 'r1', role: 'full', cueClass: 'dialogue' }),
      supplementals: [expect.objectContaining({ identity: 'signs', revision: 'r2', sourcePath: '/asset.signs.ass' })],
    }));
  });

  it('does not allow an ineligible backend candidate to be selected', () => {
    render(<SubtitleCompositionEligibilityPanel eligibility={{ ...eligible, supplementalCandidates: [...eligible.supplementalCandidates, { ...eligible.supplementalCandidates[0], sourceIdentity: 'unknown', eligible: false, reason: 'unknown language' }] }} onCompose={vi.fn()} />);
    const invalidCandidateText = screen.getAllByText(/unknown language/).find((element) => element.tagName === 'P');
    const invalidLabel = invalidCandidateText?.closest('label');
    expect(invalidLabel?.querySelector('input')?.hasAttribute('disabled')).toBe(true);
  });

  it('surfaces backend conflicts and consumes the existing operation state', () => {
    const onCancel = vi.fn();
    const onRetry = vi.fn();
    const { rerender } = render(<SubtitleCompositionEligibilityPanel eligibility={eligible} error="stale_source" operation={{ id: 'compose-1', assetPath: '/asset.mkv', status: 'running', phase: 'reconciliation', progress: 42, processed: 0, total: 0, streamIndex: -1, format: 'ass', kind: 'compose', createdAt: '', updatedAt: '' }} onCancel={onCancel} />);
    expect(screen.getByText('stale_source')).toBeTruthy();
    expect(screen.getByRole('progressbar', { name: 'Subtitle composition progress' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    rerender(<SubtitleCompositionEligibilityPanel eligibility={eligible} operation={{ id: 'compose-1', assetPath: '/asset.mkv', status: 'error', phase: 'error', progress: 42, processed: 0, total: 0, streamIndex: -1, format: 'ass', kind: 'compose', error: 'conflict', retryable: true, createdAt: '', updatedAt: '' }} onRetry={onRetry} />);
    expect(screen.getByText('conflict')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
