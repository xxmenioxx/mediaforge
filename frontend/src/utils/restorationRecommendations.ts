import type { RestorationRecommendation, RestorationRecommendationPlan, RestorationRecommendationProvenance } from '../api/types';

export const restorationRecommendationSelectionID = (id: string) => `restoration:${id}`;

export function isActionableRestorationRecommendation(item: RestorationRecommendation) {
  return item.state === 'recommended' && Boolean(item.patch && Object.keys(item.patch).length > 0);
}

export function restorationRecommendationProvenance(plan: RestorationRecommendationPlan, selectedIDs: string[]): RestorationRecommendationProvenance | undefined {
  const selected = new Set(selectedIDs);
  const appliedRecommendations = plan.recommendations
    .filter((item) => selected.has(item.id) && isActionableRestorationRecommendation(item))
    .map(({ id, domain, state, recommendedValue, confidence, reasons, warnings }) => ({ id, domain, state, recommendedValue, confidence, reasons, warnings }));
  if (appliedRecommendations.length === 0) return undefined;
  return { version: plan.version, appliedRecommendations, restorationEvidence: plan.restorationEvidence };
}
