import type { RestorationRecommendation } from '../api/types';

export const restorationRecommendationSelectionID = (id: string) => `restoration:${id}`;

export function isActionableRestorationRecommendation(item: RestorationRecommendation) {
  return item.state === 'recommended' && Boolean(item.patch && Object.keys(item.patch).length > 0);
}
