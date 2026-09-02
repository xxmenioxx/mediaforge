// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { RestorationRecommendationPlan } from '../api/types';
import { isActionableRestorationRecommendation } from '../utils/restorationRecommendations';
import { RestorationRecommendationPanel } from './RestorationRecommendationPanel';

afterEach(cleanup);

const plan: RestorationRecommendationPlan = {
  version: 1,
  applyLocked: false,
  recommendations: [
    { id: 'frame_structure', domain: 'Frame Structure', state: 'recommended', recommendedValue: 'ivtc', confidence: 'high', reasons: ['Authoritative cadence resolver selected IVTC.'], warnings: [], supportingEvidence: [], patch: { cadenceMode: 'inverse_telecine' } },
    { id: 'deblock', domain: 'Deblock', state: 'manual_review', confidence: 'medium', reasons: ['Severity is unclassified.'], warnings: [], supportingEvidence: ['lavfi.block mean=0.123456'] },
    { id: 'deband', domain: 'Deband', state: 'no_recommendation', confidence: 'unavailable', reasons: ['Banding analysis is unavailable.'], warnings: [], supportingEvidence: [] },
  ],
  restorationEvidence: {} as RestorationRecommendationPlan['restorationEvidence'],
};

describe('RestorationRecommendationPanel', () => {
  it('renders legacy recommendations whose collection fields are null', () => {
    const legacyPlan = {
      ...plan,
      recommendations: [{
        ...plan.recommendations[0],
        reasons: null,
        warnings: null,
        supportingEvidence: null,
      }],
    } as unknown as RestorationRecommendationPlan;

    render(<RestorationRecommendationPanel plan={legacyPlan} selected={[]} onToggle={vi.fn()} />);

    expect(screen.getByText('Frame Structure')).toBeTruthy();
    expect(screen.getByText('Recommended: Ivtc')).toBeTruthy();
    expect(screen.getByText('confidence High')).toBeTruthy();
  });

  it('renders canonical empty recommendation arrays', () => {
    render(<RestorationRecommendationPanel plan={{
      ...plan,
      recommendations: [{ ...plan.recommendations[0], reasons: [], warnings: [], supportingEvidence: [] }],
    }} selected={[]} onToggle={vi.fn()} />);

    expect(screen.getByText('Frame Structure')).toBeTruthy();
  });

  it('makes only actionable recommendations selectable and keeps readiness states distinct', async () => {
    const onToggle = vi.fn();
    render(<RestorationRecommendationPanel plan={plan} selected={[]} onToggle={onToggle} />);
    expect(screen.getAllByText('Manual review')).toHaveLength(2);
    expect(screen.getByText('Analysis unavailable · no recommendation')).toBeTruthy();
    expect(screen.getAllByRole('checkbox')).toHaveLength(1);
    await userEvent.click(screen.getByRole('checkbox'));
    expect(onToggle).toHaveBeenCalledWith('restoration:frame_structure', true);
    expect(isActionableRestorationRecommendation(plan.recommendations[1])).toBe(false);
  });

  it('keeps recommendations visible but disables applying when Queue is active', () => {
    render(<RestorationRecommendationPanel plan={{ ...plan, applyLocked: true, applyLockReason: 'Active Queue job' }} selected={['restoration:frame_structure']} onToggle={vi.fn()} />);
    expect(screen.getByText('Active Queue job')).toBeTruthy();
    expect(screen.getByText('Ivtc')).toBeTruthy();
    expect((screen.getByRole('checkbox') as HTMLInputElement).disabled).toBe(true);
  });
});
