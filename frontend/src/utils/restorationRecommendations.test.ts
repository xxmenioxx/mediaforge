import { describe, expect, it } from 'vitest';
import type { RestorationRecommendationPlan } from '../api/types';
import { restorationRecommendationProvenance } from './restorationRecommendations';

describe('restoration recommendation provenance', () => {
  it('snapshots only selected actionable recommendations and keeps calibration evidence', () => {
    const plan: RestorationRecommendationPlan = {
      version: 1,
      applyLocked: false,
      restorationEvidence: { status: 'available', windows: 3, sampledFrames: 9 } as RestorationRecommendationPlan['restorationEvidence'],
      recommendations: [
        { id: 'frame_structure', domain: 'Frame Structure', state: 'recommended', recommendedValue: 'ivtc', confidence: 'high', reasons: ['telecine'], warnings: [], supportingEvidence: [], patch: { cadenceMode: 'inverse_telecine' } },
        { id: 'upscale', domain: 'Smart Upscale', state: 'recommended', recommendedValue: '720p', confidence: 'high', reasons: ['reliable SD'], warnings: [], supportingEvidence: [], patch: { upscaleMode: 'auto' } },
        { id: 'denoise', domain: 'Denoise', state: 'manual_review', confidence: 'low', reasons: ['ambiguous'], warnings: [], supportingEvidence: [] },
      ],
    };

    const provenance = restorationRecommendationProvenance(plan, ['upscale', 'denoise'], '/media/raw/dvd-a.mkv');

    expect(provenance?.appliedRecommendations.map((item) => item.id)).toEqual(['upscale']);
    expect(provenance?.sourceAssetPath).toBe('/media/raw/dvd-a.mkv');
    expect(provenance?.restorationEvidence).toEqual(plan.restorationEvidence);
  });

  it('does not create provenance for manual review or unavailable selections', () => {
    const plan: RestorationRecommendationPlan = {
      version: 1,
      applyLocked: false,
      restorationEvidence: {} as RestorationRecommendationPlan['restorationEvidence'],
      recommendations: [
        { id: 'deblock', domain: 'Deblock', state: 'manual_review', confidence: 'medium', reasons: [], warnings: [], supportingEvidence: [] },
      ],
    };
    expect(restorationRecommendationProvenance(plan, ['deblock'], '/media/raw/dvd-a.mkv')).toBeUndefined();
  });
});
