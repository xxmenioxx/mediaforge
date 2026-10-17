import { describe, expect, it } from 'vitest';
import type { QueueJob } from '../api/types';
import { hasNonTerminalQueueLifecycle, originalArchiveStatusLabel, queueJobPublicationChanged } from './queueLifecycle';

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

describe('published asset refresh', () => {
  it('refreshes Assets when a previously observed Queue job becomes PUBLISHED', () => {
    const publishing = job({ id: 81, publicationState: 'PUBLISHING', publishedPath: '' });
    const published = job({ id: 81, publicationState: 'PUBLISHED', publishedPath: '/library/show/episode.mkv', publicationId: 'pub-81' });

    expect(queueJobPublicationChanged([publishing], [published])).toBe(true);
    expect(queueJobPublicationChanged([published], [published])).toBe(false);
  });

  it('does not invalidate Assets for the initial Queue snapshot', () => {
    expect(queueJobPublicationChanged(undefined, [job({ id: 81, publishedPath: '/library/show/episode.mkv' })])).toBe(false);
  });

  it('refreshes Assets when a current publication path or identity changes', () => {
    const previous = job({ id: 81, publishedPath: '/library/old.mkv', publicationId: 'pub-old' });
    const current = job({ id: 81, publishedPath: '/library/new.mkv', publicationId: 'pub-new' });

    expect(queueJobPublicationChanged([previous], [current])).toBe(true);
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
