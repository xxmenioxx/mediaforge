import { afterEach, describe, expect, it, vi } from 'vitest';
import type { CompatiblePreviewOptions, ProfileInput } from './types';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

function largeProfile(): ProfileInput {
  return {
    name: 'Large restoration draft',
    videoCodec: 'x265',
    workerConfig: {
      videoFilters: 'deblock=filter=strong:block=8,hqdn3d=4:3:6:4.5,deband=1thr=0.024:2thr=0.024:3thr=0.024:4thr=0.024,exposure=exposure=0.12,eq=brightness=0:contrast=1:saturation=0.96:gamma=0.94',
      upscaleMode: 'auto',
      upscaleSharpen: 'custom',
      upscaleSharpenCustomStrength: 0.16,
      fieldStructureMode: 'deinterlace',
      deinterlaceFieldOrder: 'bff',
      restorationRecommendationProvenance: 'evidence,'.repeat(2_000),
    },
  } as unknown as ProfileInput;
}

describe('compatible preview request transport', () => {
  it('posts the complete large profile and returns bounded media URLs', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      requestId: 'a'.repeat(64),
      cacheIdentity: 'a'.repeat(64),
      expiresInSeconds: 7200,
      path: '/media/raw/test.mkv',
    }), { status: 201, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    const options: CompatiblePreviewOptions = { path: '/media/raw/test.mkv', start: '00:00:05', seconds: 20, mode: 'quality', profile: largeProfile() };
    expect(encodeURIComponent(JSON.stringify(options)).length).toBeGreaterThan(16_384);

    const created = await api.createCompatiblePreviewRequest(options);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/assets/preview/compatible/requests');
    expect(init.method).toBe('POST');
    const posted = JSON.parse(String(init.body)) as CompatiblePreviewOptions;
    expect(posted.profile?.workerConfig?.upscaleSharpenCustomStrength).toBe(0.16);
    expect(posted.profile?.workerConfig?.deinterlaceFieldOrder).toBe('bff');
    expect(String(posted.profile?.workerConfig?.restorationRecommendationProvenance).length).toBeGreaterThan(16_000);

    const videoURL = api.compatibleAssetPreviewUrl(created.requestId);
    const sourceFrameURL = api.compatibleAssetFrameUrl(created.requestId, 'source');
    expect(videoURL.length).toBeLessThan(180);
    expect(sourceFrameURL.length).toBeLessThan(200);
    expect(videoURL).not.toContain('profile=');
    expect(sourceFrameURL).not.toContain('profile=');
  });

  it('rejects structured profiles on the legacy scalar compatible URL', () => {
    expect(() => api.compatibleAssetPreviewUrl({ path: '/media/raw/test.mkv', profile: largeProfile() })).toThrow(/must be registered/);
  });

  it('uses one request ID for inspect and metrics', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ source: {}, output: {} }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ comparable: false, reason: 'test', sourceDimensions: '', outputDimensions: '' }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    const requestId = 'b'.repeat(64);

    await api.inspectCompatibleAssetPreview(requestId);
    await api.compatibleAssetFrameMetrics(requestId);

    expect(fetchMock.mock.calls[0][0]).toBe(`/api/assets/preview/compatible/requests/${requestId}/inspect`);
    expect(fetchMock.mock.calls[1][0]).toBe(`/api/assets/preview/compatible/requests/${requestId}/metrics`);
  });

  it('propagates AbortController signals through request creation and JSON fidelity calls', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId: 'c'.repeat(64), cacheIdentity: 'c'.repeat(64), expiresInSeconds: 7200, path: '/media/raw/test.mkv' }), { status: 201, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ source: {}, output: {} }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    const controller = new AbortController();

    const created = await api.createCompatiblePreviewRequest({ path: '/media/raw/test.mkv', profile: largeProfile() }, controller.signal);
    await api.inspectCompatibleAssetPreview(created.requestId, controller.signal);

    expect((fetchMock.mock.calls[0][1] as RequestInit).signal).toBe(controller.signal);
    expect((fetchMock.mock.calls[1][1] as RequestInit).signal).toBe(controller.signal);
  });
});
