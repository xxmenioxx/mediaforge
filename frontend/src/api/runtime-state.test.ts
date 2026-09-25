import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

describe('runtime state API compatibility', () => {
  afterEach(() => vi.restoreAllMocks());

  it('decodes live state metadata without treating the technical id as audit identity', async () => {
    const state = {
      id: 7,
      stateKind: 'live',
      detectedAt: '2026-09-24T12:00:00Z',
      stableFingerprint: 'runtime-stable-v1:abc',
      auditSnapshotId: null,
    };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(state), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.runtimeState()).resolves.toMatchObject(state);
    expect(fetchMock).toHaveBeenCalledWith('/api/system/runtime', expect.objectContaining({ credentials: 'omit' }));
    expect(state.id).not.toBe(state.auditSnapshotId);
  });

  it('keeps stable-transition provenance scoped to the refresh response', async () => {
    const transition = {
      id: 7,
      stateKind: 'live',
      detectedAt: '2026-09-24T12:00:00Z',
      stableFingerprint: 'runtime-stable-v1:def',
      auditSnapshotId: 42,
    };
    const unchanged = { ...transition, stableFingerprint: 'runtime-stable-v1:def', auditSnapshotId: null };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(transition), { status: 200, headers: { 'content-type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify(unchanged), { status: 200, headers: { 'content-type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.refreshRuntime()).resolves.toMatchObject({ auditSnapshotId: 42 });
    await expect(api.runtimeState()).resolves.toMatchObject({ auditSnapshotId: null });
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/system/runtime/refresh', '/api/system/runtime']);
  });
});
