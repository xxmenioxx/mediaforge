import { describe, expect, it } from 'vitest';
import type { AssetConversionOverrideState, AssetEditResponse, AssetMetadataState } from '../api/types';
import {
  assetInfoDiscardDraft,
  assetInfoDocumentsEqual,
  assetInfoEditDocument,
  assetInfoHydrationReconciliation,
  assetInfoRefreshReconciliation,
  assetInfoTracksSaveReconciliation,
  assetInfoSaveFailureReconciliation,
  assetInfoSaveReconciliation,
  assetTracksPatch,
  mergeAssetTracks,
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
  it('keeps explicit empty track selections distinct from inherited selections', () => {
    const baseline = assetInfoEditDocument(
      metadata({ categories: [' Drama ', 'Drama'], tags: [' action ', 'action'] }),
      conversion({ keepAudioStreams: null, keepSubtitleStreams: [], keepVideoStreams: [2, 1, 2], subtitleRoleOverrides: {}, videoMetadata: {} }),
    );
    const hydrated = assetInfoEditDocument(
      metadata({ categories: ['Drama'], tags: ['action'], updatedAt: '2026-09-23T00:00:00Z' }),
      conversion({ keepAudioStreams: undefined, keepSubtitleStreams: [], keepVideoStreams: [1, 2] }),
    );

    expect(assetInfoDocumentsEqual(baseline, hydrated)).toBe(true);
    expect(assetInfoDocumentsEqual(
      assetInfoEditDocument(metadata(), { keepAudioStreams: null }),
      assetInfoEditDocument(metadata(), { keepAudioStreams: [] }),
    )).toBe(false);
  });

  it('marks persisted edits dirty and returns to clean when the user reverts them', () => {
    const baseline = assetInfoEditDocument(metadata({ categories: ['Drama'] }), conversion({ videoCodec: 'x265' }));
    const edited = assetInfoEditDocument(metadata({ categories: ['Drama'] }), conversion({ videoCodec: 'x264' }));
    const reverted = assetInfoEditDocument(metadata({ categories: ['Drama'] }), conversion({ videoCodec: 'x265' }));

    expect(assetInfoDocumentsEqual(baseline, edited)).toBe(false);
    expect(assetInfoDocumentsEqual(baseline, reverted)).toBe(true);
  });

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

  it('uses resolved defaults as the clean hydration baseline and preserves a conflicting edit', () => {
    const baseline = assetInfoEditDocument(metadata(), conversion());
    const hydrated = assetInfoEditDocument(metadata(), conversion({ optimizationIntent: 'balanced' }));
    const edited = assetInfoEditDocument(metadata(), conversion({ videoCodec: 'x264' }));

    const cleanResult = assetInfoHydrationReconciliation(baseline, baseline, hydrated);
    const dirtyResult = assetInfoHydrationReconciliation(baseline, edited, hydrated);

    expect(cleanResult.baseline).toEqual(hydrated);
    expect(cleanResult.draft).toEqual(hydrated);
    expect(cleanResult.dirty).toBe(false);
    expect(dirtyResult.baseline).toEqual(baseline);
    expect(dirtyResult.draft).toEqual(edited);
    expect(dirtyResult.dirty).toBe(true);
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

  it('builds a scoped Tracks patch with only changed fields', () => {
    const persisted = conversion({
      keepAudioStreams: [1, 2],
      keepSubtitleStreams: [3],
      videoCodec: 'x265',
      qualityValue: 18,
      subtitleRoleOverrides: { '3': { role: 'forced' } },
      subtitleSidecarFormatsByStream: { '3': ['srt'] },
    });
    const draft = {
      ...persisted,
      keepAudioStreams: [1],
      videoMetadata: { '0': { title: 'Main' } },
    };

    expect(assetTracksPatch(persisted, draft)).toEqual({
      keepAudioStreams: [1],
      videoMetadata: { '0': { title: 'Main' } },
    });
  });

  it('preserves an explicit empty selection in the Tracks patch', () => {
    expect(assetTracksPatch(conversion({ keepAudioStreams: [1] }), conversion({ keepAudioStreams: [] })))
      .toEqual({ keepAudioStreams: [] });
  });

  it('uses explicit empty maps to clear only changed role and sidecar overrides', () => {
    const persisted = conversion({
      subtitleRoleOverrides: { '3': { role: 'forced' } },
      subtitleSidecarFormatsByStream: { '3': ['srt'] },
    });
    const draft = conversion({ subtitleRoleOverrides: {}, subtitleSidecarFormatsByStream: {} });

    expect(assetTracksPatch(persisted, draft)).toEqual({
      subtitleRoleOverrides: {},
      subtitleSidecarFormatsByStream: {},
    });
  });

  it('reconciles saved Tracks values while retaining an unrelated conversion draft', () => {
    const baseline = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1, 2], videoCodec: 'x265', qualityValue: 18 }));
    const draft = assetInfoEditDocument(metadata(), conversion({ keepAudioStreams: [1], videoCodec: 'x264', qualityValue: 22 }));
    const reconciled = assetInfoTracksSaveReconciliation(baseline, draft, conversion({ keepAudioStreams: [1], videoCodec: 'x265', qualityValue: 18 }));

    expect(reconciled.baseline.conversion).toMatchObject({ keepAudioStreams: [1], videoCodec: 'x265', qualityValue: 18 });
    expect(reconciled.draft.conversion).toMatchObject({ keepAudioStreams: [1], videoCodec: 'x264', qualityValue: 22 });
    expect(reconciled.dirty).toBe(true);
  });

  it('merges a Tracks reset without changing unrelated Asset Override values', () => {
    const conversionDraft = conversion({ keepAudioStreams: [1], videoCodec: 'x264', qualityValue: 22 });
    const persisted = conversion({ keepAudioStreams: [1, 2], videoCodec: 'x265', qualityValue: 18 });
    const reset = mergeAssetTracks(conversionDraft, persisted);

    expect(reset).toMatchObject({ keepAudioStreams: [1, 2], videoCodec: 'x264', qualityValue: 22 });
  });
});
