// @vitest-environment jsdom

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { Asset } from '../api/types';
import { AssetInfoOverview } from './AssetsPage';

function libraryAsset(archivedOriginal: NonNullable<NonNullable<Asset['lifecycle']>['archivedOriginals']>[number]): Asset {
  return {
    id: 55,
    libraryId: 7,
    libraryName: 'Anime',
    path: '/media/library/anime/show/episode.mkv',
    relativePath: 'show/episode.mkv',
    groupPath: 'show',
    fileName: 'episode.mkv',
    extension: '.mkv',
    sizeBytes: 1200,
    modifiedAt: '2026-09-25T00:00:00Z',
    status: 'converted',
    missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' },
    conversion: {},
    lifecycle: {
      location: 'library', lifecycle: 'published', provenance: 'converted', validation: 'passed',
      publication: 'published', original: 'archived', archivedOriginals: [archivedOriginal],
    },
  };
}

describe('contextual archived-original inspection', () => {
  it('shows recorded metadata and retention on the related Library asset', () => {
    const archivedOriginal = {
      assetRecordId: 71,
      queueJobId: 81,
      path: '/media/originals_archive/show/episode.mkv',
      sizeBytes: 2300,
      archivedAt: '2026-09-24T12:00:00Z',
      availability: 'available' as const,
      relationship: 'current' as const,
      provenance: { authority: 'queue_job', recordId: 81, sourceAssetRecordId: 11, current: true },
      retentionState: 'retained' as const,
      retentionDeadline: '2026-10-24T12:00:00Z',
      restore: { allowed: true },
      deleteArchivedOriginal: { allowed: true },
    };

    render(<AssetInfoOverview asset={libraryAsset(archivedOriginal)} libraries={[]} />);

    expect(screen.getByText(archivedOriginal.path)).toBeTruthy();
    expect(screen.getByText(/Size ·/)).toBeTruthy();
    expect(screen.getByText(/Archive time ·/)).toBeTruthy();
    expect(screen.getByText('Provenance · Queue job #81 · Source AssetRecord #11')).toBeTruthy();
    expect(screen.getByText('Relationship · Current · Availability · Available')).toBeTruthy();
    expect(screen.getByText(/Retention · Retained · until/)).toBeTruthy();
    expect(screen.getByText('Restore · Available')).toBeTruthy();
    expect(screen.getByText('Delete · Available')).toBeTruthy();
  });

  it('keeps unresolved missing evidence visible with unknown archive time and disabled actions', () => {
    const archivedOriginal = {
      queueJobId: 82,
      path: '/media/originals_archive/legacy/episode.mkv',
      sizeBytes: 0,
      availability: 'missing' as const,
      relationship: 'unresolved' as const,
      provenance: { authority: 'queue_job', recordId: 82 },
      retentionState: 'unknown' as const,
      restore: { allowed: false, reason: 'identity_unresolved' },
      deleteArchivedOriginal: { allowed: false, reason: 'identity_unresolved' },
    };

    render(<AssetInfoOverview asset={libraryAsset(archivedOriginal)} libraries={[]} />);

    expect(screen.getByText(archivedOriginal.path)).toBeTruthy();
    expect(screen.getByText('Archive time · Unknown')).toBeTruthy();
    expect(screen.getByText('Relationship · Unresolved · Availability · Missing')).toBeTruthy();
    expect(screen.getByText(/Restore · Asset identity is unavailable/)).toBeTruthy();
    expect(screen.getByText(/Delete · Asset identity is unavailable/)).toBeTruthy();
  });
});
