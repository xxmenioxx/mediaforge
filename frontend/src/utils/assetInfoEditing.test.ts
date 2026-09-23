import { describe, expect, it } from 'vitest';
import type { AssetConversionOverrideState, AssetEditResponse, AssetMetadataState } from '../api/types';
import {
  assetInfoDiscardDraft,
  assetInfoDocumentsEqual,
  assetInfoEditDocument,
  assetInfoRefreshReconciliation,
  assetInfoSaveFailureReconciliation,
  assetInfoSaveReconciliation,
} from './assetInfoEditing';

const conversion = (patch: Partial<AssetConversionOverrideState> = {}): AssetConversionOverrideState => ({
  keepAudioStreams: [],
  keepSubtitleStreams: [],
  keepVideoStreams: [],
  ...patch,
});

const metadata = (patch: Partial<AssetMetadataState> = {}): AssetMetadataState => ({
  categories: [],
  tags: [],
  updatedAt: '2026-09-22T00:00:00Z',
  ...patch,
});

describe('asset info editing state', () => {
  it('uses semantic equality for normalized lists and server timestamps', () => {
    const left = assetInfoEditDocument(metadata({ categories: [' Drama ', 'Drama'] }), conversion({ keepAudioStreams: [2, 1, 2], subtitleRoleOverrides: {}, videoCodec: 'hevc' }));
    const right = assetInfoEditDocument(metadata({ categories: ['Drama'], updatedAt: '2026-09-23T00:00:00Z' }), conversion({ keepAudioStreams: [1, 2], videoCodec: 'hevc' }));

    expect(assetInfoDocumentsEqual(left, right)).toBe(true);
    expect(assetInfoDocumentsEqual(left, { ...right, conversion: conversion({ videoCodec: 'copy' }) })).toBe(false);
  });

  it('keeps a newer local draft when an older save response arrives', () => {
    const submitted = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1] }));
    const newer = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1], subtitleSidecarFormatsByStream: { '3': ['srt'] } }));
    const response: AssetEditResponse = {
      path: '/media/raw/movie.mkv',
      metadata: metadata(),
      conversion: conversion({ keepAudioStreams: [1] }),
    };

    const result = assetInfoSaveReconciliation(newer, submitted, response);

    expect(result.baseline).toEqual(assetInfoEditDocument(response.metadata, response.conversion));
    expect(result.draft).toEqual(newer);
    expect(result.dirty).toBe(true);
  });

  it('cleans the draft only when no edit was made during save', () => {
    const submitted = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1] }));
    const response: AssetEditResponse = {
      path: '/media/raw/movie.mkv',
      metadata: metadata({ categories: ['Drama'] }),
      conversion: conversion({ keepAudioStreams: [1] }),
    };

    const result = assetInfoSaveReconciliation(submitted, submitted, response);

    expect(result.baseline).toEqual(assetInfoEditDocument(response.metadata, response.conversion));
    expect(result.draft).toEqual(result.baseline);
    expect(result.dirty).toBe(false);
  });

  it('retains dirty drafts during background refreshes and follows clean authoritative state', () => {
    const baseline = assetInfoEditDocument(metadata(), conversion());
    const draft = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1] }));
    const authoritative = assetInfoEditDocument(metadata({ categories: ['Drama'] }), conversion({ keepAudioStreams: [2] }));

    const dirtyResult = assetInfoRefreshReconciliation(baseline, draft, authoritative);
    const cleanResult = assetInfoRefreshReconciliation(baseline, baseline, authoritative);

    expect(dirtyResult.draft).toEqual(draft);
    expect(dirtyResult.baseline).toEqual(baseline);
    expect(dirtyResult.dirty).toBe(true);
    expect(cleanResult.draft).toEqual(authoritative);
    expect(cleanResult.baseline).toEqual(authoritative);
    expect(cleanResult.dirty).toBe(false);
  });

  it('retains the draft and baseline after a failed save and can discard locally', () => {
    const baseline = assetInfoEditDocument(metadata(), conversion());
    const draft = assetInfoEditDocument(metadata({ categories: ['Drama'] }), conversion({ keepAudioStreams: [1] }));
    const failed = assetInfoSaveFailureReconciliation(baseline, draft);

    expect(failed.baseline).toEqual(baseline);
    expect(failed.draft).toEqual(draft);
    expect(failed.dirty).toBe(true);
    expect(assetInfoDiscardDraft(draft)).toEqual(draft);
  });
});
