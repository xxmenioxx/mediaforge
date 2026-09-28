import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProfileTemplate } from './types';
import { api } from './client';

const template: ProfileTemplate = {
  key: 'archive-quality',
  version: 1,
  name: 'Archive Quality',
  category: 'Quality',
  description: 'Preserve source quality.',
  definition: {
    container: 'mkv',
    videoCodec: 'x265',
    audioCodec: 'copy',
    qualityMode: 'crf',
    qualityValue: 20,
    preserveHdr: true,
    preserveSubtitles: true,
    preserveChapters: true,
  },
};

afterEach(() => vi.unstubAllGlobals());

describe('profile template API', () => {
  it('loads Guided Setup choices from the canonical catalog endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify([template]), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.profileTemplates()).resolves.toEqual([template]);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(fetchMock.mock.calls[0][0]).toBe('/api/profile-templates');
  });

  it('creates a persisted user profile through the versioned clone endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 9, templateKey: template.key, templateVersion: template.version }), {
      status: 201,
      headers: { 'content-type': 'application/json' },
    }));
    vi.stubGlobal('fetch', fetchMock);

    await api.cloneProfileTemplate({
      key: template.key,
      templateVersion: template.version,
      name: 'My archive profile',
      scope: 'asset',
    });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/profile-templates/archive-quality/clone');
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({
      templateVersion: template.version,
      name: 'My archive profile',
      scope: 'asset',
    });
  });
});
