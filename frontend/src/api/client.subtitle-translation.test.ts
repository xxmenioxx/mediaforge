import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiRequestError, api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('subtitle translation API errors', () => {
  it('uses a bounded capability reason when the response has no error field', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      eligible: false,
      available: false,
      reason: 'Selected subtitle source is not supported.',
    }), {
      status: 422,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.subtitleTranslationCapability({
      path: '/media/episode.mkv',
      source: { type: 'embedded', embedded: { streamIndex: 4 } },
    })).rejects.toEqual(expect.objectContaining({
      name: 'ApiRequestError',
      message: 'Selected subtitle source is not supported.',
      status: 422,
    } satisfies Partial<ApiRequestError>));
  });

  it('keeps an explicit API error ahead of the capability reason', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: 'Source validation failed.',
      reason: 'Internal reason must not replace the API error.',
    }), {
      status: 422,
      headers: { 'content-type': 'application/json' },
    })));

    await expect(api.subtitleTranslationCapability({
      path: '/media/episode.mkv',
      source: { type: 'embedded', embedded: { streamIndex: 4 } },
    })).rejects.toMatchObject({ message: 'Source validation failed.' });
  });
});
