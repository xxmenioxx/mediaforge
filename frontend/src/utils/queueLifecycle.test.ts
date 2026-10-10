import { describe, expect, it } from 'vitest';
import type { QueueJob } from '../api/types';
import { hasNonTerminalQueueLifecycle, originalArchiveStatusLabel } from './queueLifecycle';

function job(overrides: Partial<QueueJob> = {}): QueueJob {
  return {
    status: 'completed', publicationState: 'PUBLISHED', originalArchiveStatus: 'complete',
    ...overrides,
  } as QueueJob;
}

describe('Queue lifecycle refresh predicate', () => {
  it.each([
    ['queued Queue work', { status: 'queued' }],
    ['running Queue work', { status: 'running' }],
    ['Publisher work', { publicationState: 'PUBLISHING' }],
    ['pending archive work', { originalArchiveStatus: 'pending' }],
    ['active archive work', { originalArchiveStatus: 'archiving' }],
  ])('refreshes while there is %s', (_label, overrides) => {
    expect(hasNonTerminalQueueLifecycle([job(overrides as Partial<QueueJob>)])).toBe(true);
  });

  it.each(['complete', 'failed', 'not_applicable'])('stops after archive status %s', (originalArchiveStatus) => {
    expect(hasNonTerminalQueueLifecycle([job({ originalArchiveStatus })])).toBe(false);
  });

  it('does not poll unrelated terminal jobs or empty data', () => {
    expect(hasNonTerminalQueueLifecycle([job()])).toBe(false);
    expect(hasNonTerminalQueueLifecycle(undefined)).toBe(false);
  });
});

describe('original archive status labels', () => {
  it.each([
    ['pending', 'Pending'],
    ['archiving', 'Archiving'],
    ['complete', 'Complete'],
    ['failed', 'Failed'],
    ['not_applicable', 'Not applicable'],
    [undefined, 'Not recorded'],
  ])('labels %s as %s', (status, label) => {
    expect(originalArchiveStatusLabel(status)).toBe(label);
  });
});
