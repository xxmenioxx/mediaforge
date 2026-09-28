// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Library } from '../api/types';
import { JellyfinSettingsCard } from './JellyfinSettingsCard';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return { ...actual, api: { ...actual.api, jellyfinOutcomes: vi.fn().mockResolvedValue([]) } };
});

const libraries: Library[] = [{ id: 4, name: 'Anime', sourcePath: '/raw/anime', destinationPath: '/library/anime', type: 'tv', validationRules: {}, createdAt: '', updatedAt: '' }];

describe('JellyfinSettingsCard', () => {
  it('shows configured state without populating the write-only credential and keeps mapping IDs', async () => {
    const onSave = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <JellyfinSettingsCard
          value={{
            enabled: true, baseUrl: 'http://jellyfin:8096', credentialConfigured: true,
            refreshAfterPublish: true, refreshTestEncode: true, debounceSeconds: 10,
            mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', jellyfinName: 'Anime' }],
          }}
          libraries={libraries}
          saving={false}
          onSave={onSave}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByText(/Jellyfin-Exact-44/)).toBeTruthy();
    const credential = screen.getByLabelText(/replace api key \/ token/i) as HTMLInputElement;
    expect(credential.value).toBe('');
    expect(screen.getByText(/credential is configured; stored value is never returned/i)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /save jellyfin settings/i }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ enabled: true, mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', jellyfinName: 'Anime' }] }));
    expect(onSave.mock.calls[0][0]).not.toHaveProperty('apiToken');
  });
});
