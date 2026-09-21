import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('subtitle translation readiness API', () => {
  it('uses MVForge readiness endpoints and never constructs a direct Ollama request', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ configured: true, enabled: false, readiness: 'ready' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ configured: true, enabled: true, readiness: 'ready' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }));
    vi.stubGlobal('fetch', fetchMock);

    await api.testSubtitleTranslationConnection();
    await api.enableSubtitleTranslation();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/settings/subtitle-translation/test-connection');
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/settings/subtitle-translation/enable');
    expect(fetchMock.mock.calls.every(([url]) => !String(url).includes('11434'))).toBe(true);
  });
});
