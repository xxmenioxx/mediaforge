import { afterEach, describe, expect, it, vi } from 'vitest';
import type { CompatiblePreviewOptions, ProfileInput } from './types';
import { ApiRequestError, api, request, waitForCompatiblePreviewReady } from './client';
import { compatiblePreviewRecoveryMessage, isCompatiblePreviewRecoveryError } from './compatible-preview-recovery';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

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
      cacheIdentity: 'b'.repeat(64),
      expiresInSeconds: 7200,
      path: '/media/raw/test.mkv',
    }), { status: 201, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    const options: CompatiblePreviewOptions = { path: '/media/raw/test.mkv', start: '00:00:05', seconds: 20, mode: 'quality', profile: largeProfile() };
    expect(encodeURIComponent(JSON.stringify(options)).length).toBeGreaterThan(16_384);

    const created = await api.createCompatiblePreviewRequest(options);

    expect(created.requestId).not.toBe(created.cacheIdentity);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/assets/preview/compatible/requests');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('omit');
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

  it('keeps the canonical request credentialless when the caller requests credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await request('/api/test', { credentials: 'include', headers: { 'X-Test': 'available' } });

    expect((fetchMock.mock.calls[0][1] as RequestInit).credentials).toBe('omit');
    expect((fetchMock.mock.calls[0][1] as RequestInit).headers).toMatchObject({ 'X-Test': 'available' });
  });

  it('preserves machine-readable restart lifecycle errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: 'The preview was interrupted by a backend restart.',
      code: 'interrupted',
      state: 'interrupted',
      reason: 'backend restarted while generation was active',
      retryable: true,
      recreate: true,
    }), { status: 409, headers: { 'content-type': 'application/json' } })));

    await expect(request('/api/assets/preview/compatible/requests/request/inspect')).rejects.toMatchObject({
      status: 409,
      code: 'interrupted',
      state: 'interrupted',
      retryable: true,
      recreate: true,
    } satisfies Partial<ApiRequestError>);
  });

  it('maps recovery states to recreate actions without treating them as timeouts', () => {
    for (const code of ['interrupted', 'stale_artifact', 'expired', 'unknown_request', 'failed', 'canceled']) {
      const error = new ApiRequestError(code, 409, { code, state: code, recreate: true });
      expect(isCompatiblePreviewRecoveryError(error)).toBe(true);
      expect(compatiblePreviewRecoveryMessage(error)).toMatch(/Recreate/);
    }
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

  it('uses credentialless lifecycle calls and accepts 202 generating responses', async () => {
    const requestId = 'd'.repeat(64);
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating', leaseToken: 'lease-a', ownerId: 'owner-a' }), { status: 202, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'ready', state: 'ready', cacheHit: false }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'ready', state: 'ready', leaseReleased: false }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    const started = await api.startCompatiblePreview(requestId, 'owner-a');
    const status = await api.compatiblePreviewStatus(requestId);
    const canceled = await api.cancelCompatiblePreview(requestId, 'owner-a', 'lease-a');

    expect(started.state).toBe('generating');
    expect(status.state).toBe('ready');
    expect(canceled.leaseReleased).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    for (const [, init] of fetchMock.mock.calls) {
      expect((init as RequestInit).credentials).toBe('omit');
    }
    expect(fetchMock.mock.calls[0][0]).toBe(`/api/assets/preview/compatible/requests/${requestId}/start`);
    expect(JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))).toEqual({ ownerId: 'owner-a' });
  });

  it('returns an already-ready cache hit without requiring a lease token', async () => {
    const requestId = 'f'.repeat(64);
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      requestId,
      code: 'ready',
      state: 'ready',
      cacheHit: true,
    }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    const ready = await waitForCompatiblePreviewReady(requestId, undefined, 'owner-cache-hit');

    expect(ready).toMatchObject({ state: 'ready', cacheHit: true });
    expect(ready.leaseToken).toBeFalsy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`/api/assets/preview/compatible/requests/${requestId}/start`);
  });

  it('re-reads canonical status when renewal loses a terminal transition race', async () => {
    vi.useFakeTimers();
    const requestId = 'e'.repeat(64);
    const generating = JSON.stringify({ requestId, code: 'generating', state: 'generating', leaseToken: 'lease-a', ownerId: 'owner-a' });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(generating, { status: 202, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'generating', state: 'generating' }), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 'invalid_lease', state: 'generating' }), { status: 409, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ requestId, code: 'ready', state: 'ready' }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    const pending = waitForCompatiblePreviewReady(requestId, undefined, 'owner-a');
    await vi.advanceTimersByTimeAsync(18_000);
    await expect(pending).resolves.toMatchObject({ state: 'ready', leaseToken: 'lease-a', ownerId: 'owner-a' });
    expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit).method === 'POST' && String((init as RequestInit).body).includes('lease-a'))).toBe(true);
  });

  it('rejects empty request IDs before constructing resource URLs', () => {
    expect(() => api.compatibleAssetPreviewUrl('')).toThrow(/requestId is required/);
    expect(() => api.compatibleAssetFrameUrl('   ', 'output')).toThrow(/requestId is required/);
    expect(() => api.inspectCompatibleAssetPreview('')).toThrow(/requestId is required/);
  });

  it('does not expose a request handle until creation returns a non-empty ID', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ requestId: ' ', cacheIdentity: 'artifact' }), {
      status: 201,
      headers: { 'content-type': 'application/json' },
    })));
    await expect(api.createCompatiblePreviewRequest({ path: '/media/raw/test.mkv' })).rejects.toThrow(/empty requestId/);
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

  it('starts an explicit bounded QSV temporal experiment through the existing LAB operation', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'lab-1', status: 'queued', phase: 'waiting_for_capacity', primaryRequestId: 'p', completedWindows: 0, totalWindows: 1, createdAt: '', updatedAt: '' }), { status: 202, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    await api.startLabFidelityOperation({
      primaryRequestId: 'p',
      qsvTemporalExperiment: { variantsByWindow: [{ windowIdentity: 'window:primary', variants: [{ changedVariable: 'gop', requestedDelta: { gopFrames: 48 } }] }] },
    });
    expect(String((fetchMock.mock.calls[0][1] as RequestInit).body)).toContain('qsvTemporalExperiment');
    expect(String((fetchMock.mock.calls[0][1] as RequestInit).body)).toContain('window:primary');
  });

  it('cancels the owning LAB operation without creating a replacement request', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'lab-1', status: 'canceled', phase: 'canceled', primaryRequestId: 'p', completedWindows: 1, totalWindows: 3, createdAt: '', updatedAt: '' }), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);
    await api.cancelLabFidelityOperation('lab-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/lab/fidelity/operations/lab-1/cancel');
    expect((fetchMock.mock.calls[0][1] as RequestInit).method).toBe('POST');
  });
});
