// @vitest-environment jsdom

import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { SubtitleCompositionEligibility } from '../api/types';
import { SubtitleCompositionEligibilityPanel } from './SubtitleCompositionEligibilityPanel';

const eligible: SubtitleCompositionEligibility = {
  eligible: true,
  policyVersion: 'subtitle-composition-v1',
  maxSupplementals: 8,
  primaryCandidates: [{ sourceIdentity: 'primary', sourceRevision: 'r1', representation: 'ass', language: 'en', role: 'full', compositionClass: 'dialogue', sourceType: 'embedded', eligible: true }],
  supplementalCandidates: [{ sourceIdentity: 'signs', sourceRevision: 'r2', representation: 'ass', language: 'en', role: 'signs', compositionClass: 'sign', sourceType: 'external', eligible: true }],
  crossLanguageExplicit: false,
};

describe('SubtitleCompositionEligibilityPanel', () => {
  it('uses backend eligibility and does not invent a local candidate decision', () => {
    render(<SubtitleCompositionEligibilityPanel eligibility={{ ...eligible, eligible: false, reason: 'unknown language' }} />);
    expect(screen.getByText('unknown language')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Compose subtitles' })).toBeNull();
  });

  it('only enables the backend-authorized compose action when supplied', () => {
    const onCompose = vi.fn();
    render(<SubtitleCompositionEligibilityPanel eligibility={eligible} onCompose={onCompose} />);
    expect(screen.getByRole('button', { name: 'Compose subtitles' })).toBeTruthy();
  });
});
