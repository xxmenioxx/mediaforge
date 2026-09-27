import type { ExternalSubtitle, AssetGroup } from '../api/types';

export function subtitleOperationStopsPolling(status: 'running' | 'completed' | 'cancelled' | 'error') {
  return status !== 'running';
}

export function subtitleOCRReviewKey(subtitle: ExternalSubtitle) {
  const logicalTransformKey = subtitle.logicalTransformKey?.trim();
  return logicalTransformKey ? `logical:${logicalTransformKey}` : subtitle.path;
}

export function subtitleOCRSidecarSelection(values: ExternalSubtitle[], sidecar: ExternalSubtitle) {
  const logicalTransformKey = sidecar.logicalTransformKey?.trim();
  if (logicalTransformKey) {
    const siblings = values.filter((candidate) => candidate.logicalTransformKey?.trim() === logicalTransformKey);
    const srt = siblings.find((candidate) => candidate.format === 'srt');
    const ass = siblings.find((candidate) => candidate.format === 'ass');
    if (srt && ass) {
      return { sidecarPaths: [srt.path, ass.path], format: 'both' as const };
    }
  }
  return { sidecarPaths: [sidecar.path], format: sidecar.format === 'ssa' ? 'ass' : sidecar.format };
}

export function filterAssetGroups(groups: AssetGroup[], query: string, filters: { provenance?: string; validation?: string; original?: string; workflow?: string; attention?: string } = {}) {
  const cleanQuery = query.trim().toLowerCase();
  const originalFilter = filters.original === 'archived' ? '' : filters.original;
  if (!cleanQuery && !filters.provenance && !filters.validation && !originalFilter && !filters.workflow && !filters.attention) return groups;
  return groups.flatMap((group) => {
    const matchingAssets = (Array.isArray(group.assets) ? group.assets : []).filter((asset) => {
      const lifecycle = asset.lifecycle;
      const provenance = lifecycle?.provenance ?? (asset.status === 'converted' ? 'converted' : asset.status === 'published_as_is' ? 'published_as_is' : asset.status === 'accepted' ? 'imported' : 'legacy_or_unknown');
      const validation = lifecycle?.validation ?? 'unavailable';
      const original = lifecycle?.original ?? 'unknown';
      const workflow = lifecycle?.lifecycle ?? (asset.status === 'unprocessed' ? 'available' : 'unknown');
      if (filters.provenance && provenance !== filters.provenance) return false;
      if (filters.validation && validation !== filters.validation) return false;
      if (originalFilter && original !== originalFilter) return false;
      if (filters.workflow && workflow !== filters.workflow) return false;
      if (filters.attention && !(lifecycle?.attention ?? []).includes(filters.attention)) return false;
      if (!cleanQuery) return true;
      const values = [asset.fileName, asset.path, asset.relativePath, asset.status, lifecycle?.lifecycle, lifecycle?.provenance, lifecycle?.validation, lifecycle?.publication, lifecycle?.original, ...(lifecycle?.attention ?? [])];
      return values.some((value) => (value ?? '').toLowerCase().includes(cleanQuery));
    });
    if (!matchingAssets.length && (filters.provenance || filters.validation || originalFilter || filters.workflow || filters.attention)) return [];
    if (!matchingAssets.length) {
      const groupValues = [group.path, group.relativePath, group.libraryName, group.status];
      if (!cleanQuery || !groupValues.some((value) => (value ?? '').toLowerCase().includes(cleanQuery))) return [];
    }
    return [{ ...group, assets: matchingAssets }];
  });
}
