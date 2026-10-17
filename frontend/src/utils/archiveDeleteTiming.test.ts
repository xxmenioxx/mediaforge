import { describe, expect, it } from 'vitest';
import { measureArchiveDeletePhase } from './archiveDeleteTiming';

describe('archive deletion timing', () => {
  it('records bounded user timing measures for request, refresh, and feedback phases', () => {
    for (const phase of ['delete-request', 'assets-refresh', 'feedback-render'] as const) {
      const prefix = `mvforge.archive-delete.${phase}.`;
      const existing = performance.getEntriesByType('measure')
        .filter((entry) => entry.name.startsWith(prefix));
      for (const entry of existing) performance.clearMeasures(entry.name);
      measureArchiveDeletePhase(phase, performance.now() - 1);
      const [entry] = performance.getEntriesByType('measure')
        .filter((candidate) => candidate.name.startsWith(prefix));
      expect(entry).toBeTruthy();
      expect(entry.duration).toBeGreaterThanOrEqual(0);
    }
  });
});
