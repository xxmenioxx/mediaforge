import { afterEach, describe, expect, it, vi } from 'vitest';

import { api } from './client';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('subtitle extraction cancellation API mapping', () => {
  it('posts cancellation by operation identity and returns the operation snapshot', async () => {
    const snapshot = {
      id: 'subtitle-operation-1',
      assetPath: '/media/movie.mkv',
      status: 'cancelled',
      phase: 'cleanup',
      progress: 42,
      processed: 2,
      total: 5,
      streamIndex: 4,
      format: 'srt',
      createdAt: '2026-09-17T00:00:00Z',
      updatedAt: '2026-09-17T00:00:01Z',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => snapshot,
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.cancelSubtitleExtractionOperation('subtitle-operation-1')).resolves.toEqual(snapshot);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/assets/extract-subtitles/subtitle-operation-1/cancel',
      expect.objectContaining({ method: 'POST' }),
    );
  });
});

describe('subtitle extraction optional OCR request fields', () => {
  it('omits untouched OCR suggestions and preserves explicit values', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ created: [], existing: [], unsupported: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await api.extractAssetSubtitles({ path: '/media/movie.mkv', streamIndex: 4, format: 'srt' });
    const omittedBody = JSON.parse(String(fetchMock.mock.calls[0][1]?.body));
    expect(omittedBody).toEqual({ streamIndex: 4, format: 'srt' });

    await api.extractAssetSubtitles({
      path: '/media/movie.mkv', streamIndex: 4, format: 'srt', ocrLanguage: 'auto', ocrMode: 'raw',
    });
    const explicitBody = JSON.parse(String(fetchMock.mock.calls[1][1]?.body));
    expect(explicitBody).toEqual({ streamIndex: 4, format: 'srt', ocrLanguage: 'auto', ocrMode: 'raw' });
  });
});
