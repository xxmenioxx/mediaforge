import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('shared Ollama model settings API', () => {
  it('uses backend connection, catalog, readiness, and runtime status endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ state: 'reachable', checkedAt: '', serverVersion: '0.1.0' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ state: 'available', installedModels: [{ id: 'same-model', name: 'same-model' }] }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ state: 'available', modelId: 'same-model', available: true, checkedAt: '', scope: 'metadata_check' }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ state: 'available', models: [], activeOperations: [], failures: [] }), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }));
    vi.stubGlobal('fetch', fetchMock);

    await api.testOllamaConnection();
    await api.listOllamaModels();
    await api.testOllamaModelReadiness('same-model');
    await api.ollamaRuntimeStatus();

    expect(fetchMock).toHaveBeenCalledTimes(4);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/settings/ollama/test-connection');
    expect(fetchMock.mock.calls[0]?.[1]?.method).toBe('POST');
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/settings/ollama/models');
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/settings/ollama/model-readiness');
    expect(JSON.parse(String(fetchMock.mock.calls[2]?.[1]?.body))).toEqual({ modelId: 'same-model' });
    expect(fetchMock.mock.calls[3]?.[0]).toBe('/api/settings/ollama/runtime');
    expect(fetchMock.mock.calls.every(([url]) => !String(url).includes('11434'))).toBe(true);
  });
});
