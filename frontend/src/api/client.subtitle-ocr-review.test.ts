import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('canonical OCR review revision API', () => {
  it('round trips nanosecond source and artifact revisions as exact decimal strings', async () => {
    const sourceMtime = '1791511871171317368';
    const artifactMtime = '1791511871171319132';
    const sidecarPath = '/media/raw/anime/Samurai Champloo/Disc 1.eng.4.srt';
    const revision = {
      artifactPath: sidecarPath,
      artifactIdentity: `path:${sidecarPath}`,
      sizeBytes: 21_814,
      modifiedAtUnixNano: artifactMtime,
      contentHash: 'review-artifact-hash',
      logicalTransform: 'subtitle:4:converted:both',
    };
    const reviewTarget = {
      state: 'exact' as const,
      identity: {
        version: 1,
        sourcePath: '/media/raw/anime/Samurai Champloo/Disc 1.mkv',
        sourceSize: 3_808_874_633,
        sourceModTimeUnixNano: sourceMtime,
        sourceSnapshotVersion: 'stat-v1',
        streamIndex: 4,
        streamId: '<nil>',
        streamCodec: 'hdmv_pgs_subtitle',
        requestedLanguage: 'eng',
        ocrMode: 'clean',
        preparationScope: 'full-asset',
        segmentStart: '0.000000',
        segmentDuration: 0,
        preparedStreamMapping: 'subtitle-stream:4;codec:hdmv_pgs_subtitle',
        preparedRepresentation: 'prepared-subtitle-v1',
        preparedRepresentationHash: 'prepared-hash',
        pipelineProvenance: 'canonical-subtitle-pipeline/ocr-evidence-v1',
        runtimeProvenance: 'ocr-runtime-v1',
      },
      logicalTransformKey: 'subtitle:4:converted:both',
      format: 'both' as const,
      artifacts: [
        { ownerIdentity: `srt-owner\u0000srt\u0000subtitle:4:converted:both`, path: sidecarPath, format: 'srt' as const, logicalTransformKey: 'subtitle:4:converted:both', revision },
        { ownerIdentity: `ass-owner\u0000ass\u0000subtitle:4:converted:both`, path: sidecarPath.replace(/\.srt$/, '.ass'), format: 'ass' as const, logicalTransformKey: 'subtitle:4:converted:both', revision: { ...revision, artifactPath: sidecarPath.replace(/\.srt$/, '.ass'), sizeBytes: 28_784, modifiedAtUnixNano: '1791511871171319132', contentHash: 'ass-review-artifact-hash' } },
      ],
      sidecarPaths: [sidecarPath, sidecarPath.replace(/\.srt$/, '.ass')],
      revisions: [revision, { ...revision, artifactPath: sidecarPath.replace(/\.srt$/, '.ass'), sizeBytes: 28_784, modifiedAtUnixNano: '1791511871171319132', contentHash: 'ass-review-artifact-hash' }],
      baselineRevision: 'exact-revision-set-hash',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ operationId: 'review-operation', status: 'running', phase: 'preparing', progress: 1 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await api.analyzeSubtitleOCR({
      mediaPath: reviewTarget.identity.sourcePath,
      sidecarPath,
      sidecarPaths: reviewTarget.sidecarPaths,
      format: 'both',
      logicalTransformKey: reviewTarget.logicalTransformKey,
      reviewTarget,
    });
    await api.correctSubtitleOCR({
      mediaPath: reviewTarget.identity.sourcePath,
      sidecarPath,
      sidecarPaths: reviewTarget.sidecarPaths,
      format: 'both',
      logicalTransformKey: reviewTarget.logicalTransformKey,
      baselineRevision: reviewTarget.baselineRevision,
      revisions: reviewTarget.revisions,
      reviewTarget,
      corrections: [{ cueId: 63, action: 'approve' }],
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const analyzeBody = JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body));
    const correctionBody = JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body));
    expect(analyzeBody.reviewTarget.identity.sourceModTimeUnixNano).toBe(sourceMtime);
    expect(analyzeBody.reviewTarget.revisions[0].modifiedAtUnixNano).toBe(artifactMtime);
    expect(correctionBody.reviewTarget.identity.sourceModTimeUnixNano).toBe(sourceMtime);
    expect(correctionBody.revisions[0].modifiedAtUnixNano).toBe(artifactMtime);
  });
});
