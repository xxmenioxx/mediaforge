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

  it('polls READY publications only when automatic publishing is explicitly enabled', () => {
    const ready = [job({ publicationState: 'READY' })];
    expect(hasNonTerminalQueueLifecycle(ready, true)).toBe(true);
    expect(hasNonTerminalQueueLifecycle(ready, false)).toBe(false);
    expect(hasNonTerminalQueueLifecycle(ready)).toBe(false);
  });

  it.each(['PUBLISHED', 'FAILED', 'BLOCKED', 'HELD'] as const)(
    'does not poll terminal or non-actionable publication state %s',
    (publicationState) => {
      expect(hasNonTerminalQueueLifecycle([job({ publicationState })], true)).toBe(false);
    },
  );
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
