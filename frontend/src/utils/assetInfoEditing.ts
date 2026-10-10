import type {
  AssetConversionOverrideState,
  AssetEditResponse,
  AssetMetadataState,
  AssetTracksOverrideState,
} from '../api/types';

export type AssetInfoEditDocument = {
  metadata: {
    categories: string[];
    tags: string[];
  };
  conversion: AssetConversionOverrideState;
};

export function assetInfoEditDocument(
  metadata: Pick<AssetMetadataState, 'categories' | 'tags'>,
  conversion: AssetConversionOverrideState,
): AssetInfoEditDocument {
  return {
    metadata: {
      categories: normalizeStringList(metadata.categories),
      tags: normalizeStringList(metadata.tags),
    },
    conversion: normalizeConversion(conversion),
  };
}

export function assetInfoDocumentsEqual(left: AssetInfoEditDocument, right: AssetInfoEditDocument): boolean {
  return stableStringify(normalizeDocument(left)) === stableStringify(normalizeDocument(right));
}

export function assetInfoSaveReconciliation(
  currentDraft: AssetInfoEditDocument,
  submittedDraft: AssetInfoEditDocument,
  response: AssetEditResponse,
): { baseline: AssetInfoEditDocument; draft: AssetInfoEditDocument; dirty: boolean } {
  const baseline = assetInfoEditDocument(response.metadata, response.conversion);
  if (assetInfoDocumentsEqual(currentDraft, submittedDraft)) {
    return { baseline, draft: baseline, dirty: false };
  }
  return { baseline, draft: currentDraft, dirty: true };
}

export function assetInfoSaveFailureReconciliation(
  baseline: AssetInfoEditDocument,
  draft: AssetInfoEditDocument,
): { baseline: AssetInfoEditDocument; draft: AssetInfoEditDocument; dirty: boolean } {
  return { baseline, draft, dirty: !assetInfoDocumentsEqual(baseline, draft) };
}

export function assetInfoRefreshReconciliation(
  baseline: AssetInfoEditDocument,
  draft: AssetInfoEditDocument,
  authoritative: AssetInfoEditDocument,
): { baseline: AssetInfoEditDocument; draft: AssetInfoEditDocument; dirty: boolean } {
  return assetInfoHydrationReconciliation(baseline, draft, authoritative);
}

export function assetInfoHydrationReconciliation(
  baseline: AssetInfoEditDocument,
  draft: AssetInfoEditDocument,
  hydrated: AssetInfoEditDocument,
): { baseline: AssetInfoEditDocument; draft: AssetInfoEditDocument; dirty: boolean } {
  if (!assetInfoDocumentsEqual(baseline, draft)) {
    return { baseline, draft, dirty: true };
  }
  const next = assetInfoEditDocument(hydrated.metadata, hydrated.conversion);
  return { baseline: next, draft: next, dirty: false };
}

export function assetInfoDiscardDraft(baseline: AssetInfoEditDocument): AssetInfoEditDocument {
  return assetInfoEditDocument(baseline.metadata, baseline.conversion);
}

export function assetTracksPatch(
  persisted: AssetConversionOverrideState,
  draft: AssetConversionOverrideState,
): Partial<AssetTracksOverrideState> {
  const before = normalizeAssetTracks(persisted);
  const after = normalizeAssetTracks(draft);
  const patch: Partial<AssetTracksOverrideState> = {};
  const selections = ['keepVideoStreams', 'keepAudioStreams', 'keepSubtitleStreams'] as const;
  const metadata = ['videoMetadata', 'audioMetadata', 'subtitleMetadata'] as const;

  selections.forEach((key) => {
    if (stableStringify(before[key]) === stableStringify(after[key])) return;
    patch[key] = after[key] ?? null;
  });
  metadata.forEach((key) => {
    if (stableStringify(before[key]) === stableStringify(after[key])) return;
    patch[key] = after[key] ?? {};
  });
  if (stableStringify(before.subtitleRoleOverrides) !== stableStringify(after.subtitleRoleOverrides)) {
    patch.subtitleRoleOverrides = after.subtitleRoleOverrides ?? {};
  }
  if (stableStringify(before.subtitleSidecarFormatsByStream) !== stableStringify(after.subtitleSidecarFormatsByStream)) {
    patch.subtitleSidecarFormatsByStream = after.subtitleSidecarFormatsByStream ?? {};
  }
  return patch;
}

export function mergeAssetTracks(
  conversion: AssetConversionOverrideState,
  tracks: AssetTracksOverrideState,
): AssetConversionOverrideState {
  const next = { ...conversion };
  const keys: Array<keyof AssetTracksOverrideState> = [
    'keepVideoStreams', 'keepAudioStreams', 'keepSubtitleStreams',
    'videoMetadata', 'audioMetadata', 'subtitleMetadata',
    'subtitleRoleOverrides', 'subtitleSidecarFormatsByStream',
  ];
  keys.forEach((key) => { delete next[key]; });
  Object.assign(next, normalizeAssetTracks(tracks));
  return next;
}

export function assetInfoTracksSaveReconciliation(
  baseline: AssetInfoEditDocument,
  draft: AssetInfoEditDocument,
  responseConversion: AssetConversionOverrideState,
): { baseline: AssetInfoEditDocument; draft: AssetInfoEditDocument; dirty: boolean } {
  const tracks = normalizeAssetTracks(responseConversion);
  const nextBaseline = assetInfoEditDocument(baseline.metadata, mergeAssetTracks(baseline.conversion, tracks));
  const nextDraft = assetInfoEditDocument(draft.metadata, mergeAssetTracks(draft.conversion, tracks));
  return {
    baseline: nextBaseline,
    draft: nextDraft,
    dirty: !assetInfoDocumentsEqual(nextBaseline, nextDraft),
  };
}

function normalizeStringList(values: string[] | undefined): string[] {
  const seen = new Set<string>();
  return (values ?? []).reduce<string[]>((result, value) => {
    const normalized = value.trim();
    if (normalized && !seen.has(normalized)) {
      seen.add(normalized);
      result.push(normalized);
    }
    return result;
  }, []);
}

function normalizeDocument(document: AssetInfoEditDocument): AssetInfoEditDocument {
  return assetInfoEditDocument(document.metadata, document.conversion);
}

function normalizeConversion(conversion: AssetConversionOverrideState): AssetConversionOverrideState {
  const normalized = { ...conversion };
  (['keepVideoStreams', 'keepAudioStreams', 'keepSubtitleStreams'] as const).forEach((key) => {
    const values = normalized[key];
    const unique = Array.from(new Set((values ?? []).filter((value) => Number.isInteger(value) && value >= 0))).sort((left, right) => left - right);
    if (Array.isArray(values)) normalized[key] = unique;
    else delete normalized[key];
  });
  if (!normalized.subtitleRoleOverrides || !Object.keys(normalized.subtitleRoleOverrides).length) delete normalized.subtitleRoleOverrides;
  if (!normalized.videoMetadata || !Object.keys(normalized.videoMetadata).length) delete normalized.videoMetadata;
  if (!normalized.audioMetadata || !Object.keys(normalized.audioMetadata).length) delete normalized.audioMetadata;
  if (!normalized.subtitleMetadata || !Object.keys(normalized.subtitleMetadata).length) delete normalized.subtitleMetadata;
  if (normalized.subtitleSidecarFormatsByStream) {
    const sidecars = Object.fromEntries(Object.entries(normalized.subtitleSidecarFormatsByStream).flatMap(([index, formats]) => {
      const values = Array.from(new Set(formats)).sort();
      return values.length ? [[index, values]] : [];
    }));
    if (Object.keys(sidecars).length) normalized.subtitleSidecarFormatsByStream = sidecars;
    else delete normalized.subtitleSidecarFormatsByStream;
  }
  if (!normalized.subtitleTransforms?.length) delete normalized.subtitleTransforms;
  return normalized;
}

function normalizeAssetTracks(tracks: AssetTracksOverrideState): AssetTracksOverrideState {
  const normalized: AssetTracksOverrideState = {};
  (['keepVideoStreams', 'keepAudioStreams', 'keepSubtitleStreams'] as const).forEach((key) => {
    const values = tracks[key];
    if (Array.isArray(values)) {
      normalized[key] = Array.from(new Set(values.filter((value) => Number.isInteger(value) && value >= 0)))
        .sort((left, right) => left - right);
    }
  });
  (['videoMetadata', 'audioMetadata', 'subtitleMetadata'] as const).forEach((key) => {
    const values = tracks[key];
    if (values && Object.keys(values).length) normalized[key] = values;
  });
  if (tracks.subtitleRoleOverrides && Object.keys(tracks.subtitleRoleOverrides).length) {
    normalized.subtitleRoleOverrides = tracks.subtitleRoleOverrides;
  }
  if (tracks.subtitleSidecarFormatsByStream) {
    const sidecars = Object.fromEntries(Object.entries(tracks.subtitleSidecarFormatsByStream).flatMap(([index, formats]) => {
      const values = Array.from(new Set(formats)).sort();
      return values.length ? [[index, values]] : [];
    }));
    if (Object.keys(sidecars).length) normalized.subtitleSidecarFormatsByStream = sidecars;
  }
  return normalized;
}

function stableStringify(value: unknown): string {
  return JSON.stringify(stableValue(value));
}

function stableValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableValue);
  if (!value || typeof value !== 'object') return value;
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .filter(([key]) => key !== 'updatedAt')
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, child]) => [key, stableValue(child)]),
  );
}
