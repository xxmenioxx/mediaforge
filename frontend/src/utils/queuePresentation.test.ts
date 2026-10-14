import { describe, expect, it } from 'vitest';
import type { QueueJob } from '../api/types';
import { queueStagePresentation } from './queuePresentation';

describe('queueStagePresentation', () => {
  it('labels Validation review from the persisted stage and uses warning semantics', () => {
    const job = { status: 'completed', stage: 'validation_review' } as QueueJob;

    expect(queueStagePresentation(job)).toEqual({ label: 'validation review', color: 'warning' });
  });
});
