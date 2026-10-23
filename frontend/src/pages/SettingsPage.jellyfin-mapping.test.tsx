// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';

const libraries = [{
  id: 4,
  name: 'Anime',
  sourcePath: '/raw/anime',
  destinationPath: '/library/anime',
  type: 'tv',
  validationRules: {},
  createdAt: '',
  updatedAt: '',
}];

const jellyfinLibraries = [{ id: 'jf-library-exact-44', name: 'Anime', collectionType: 'tvshows' }];
const initialJellyfinValue: Record<string, unknown> = {
  enabled: true,
  baseUrl: 'http://jellyfin.local:8096',
  credentialConfigured: true,
  connectionConfigRevision: 1,
  lastConnectionSuccessful: true,
  lastSuccessfulConnectionAt: '2026-10-05T12:00:00Z',
  configurationValid: true,
  configurationNeedsRevalidation: false,
  librariesLoaded: true,
  refreshAfterPublish: true,
  refreshTestEncode: true,
  debounceSeconds: 10,
  libraries: jellyfinLibraries,
  mappings: [],
};

let persistedJellyfinValue: Record<string, unknown>;
let settingsReads: number;
let stateReads: number;
let settingsPosts: Array<{ path: string; value: Record<string, unknown> }>;
let rejectJellyfinSave: boolean;
let liveReachability: string;
let discoveredStateLibraries: typeof jellyfinLibraries;

function jsonResponse(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function renderSettings(queryClient = makeQueryClient()) {
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/settings?section=providers-integrations']}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

async function addAnimeMapping(user: ReturnType<typeof userEvent.setup>) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: /mvforge library/i }));
  await user.click(await screen.findByRole('option', { name: /anime · id 4/i }));
  fireEvent.mouseDown(screen.getByRole('combobox', { name: /discovered jellyfin library/i }));
  await user.click(await screen.findByRole('option', { name: /anime · id jf-library-exact-44/i }));
  await user.click(screen.getByRole('button', { name: /^add mapping$/i }));
}

function savedMappingIsDisplayed() {
  return screen.getByText(/anime \(id 4\) → anime \(id jf-library-exact-44\)/i);
}

beforeEach(() => {
  persistedJellyfinValue = structuredClone(initialJellyfinValue);
  settingsReads = 0;
  stateReads = 0;
  settingsPosts = [];
  rejectJellyfinSave = false;
  liveReachability = 'reachable';
  discoveredStateLibraries = jellyfinLibraries;

  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost');
    const method = init?.method ?? 'GET';

    if (url.pathname === '/api/settings' && method === 'GET') {
      settingsReads += 1;
      return jsonResponse([{ key: 'jellyfin', value: persistedJellyfinValue, createdAt: '', updatedAt: '' }]);
    }
    if (url.pathname === '/api/libraries' && method === 'GET') return jsonResponse(libraries);
    if (url.pathname === '/api/settings/jellyfin/state' && method === 'GET') {
      stateReads += 1;
      const mappings = Array.isArray(persistedJellyfinValue.mappings) ? persistedJellyfinValue.mappings : [];
      return jsonResponse({
        enabled: true,
        credentialConfigured: true,
        configurationValid: true,
        configurationNeedsRevalidation: false,
        lastConnectionSuccessful: true,
        lastSuccessfulConnectionAt: '2026-10-05T12:00:00Z',
        liveReachability,
        serverId: 'mapping-server',
        librariesLoaded: true,
        libraries: discoveredStateLibraries,
        mappings,
      });
    }
    if (url.pathname === '/api/jellyfin/outcomes' && method === 'GET') return jsonResponse([]);
    if (url.pathname === '/api/settings/jellyfin' && method === 'POST') {
      const submitted = JSON.parse(String(init?.body)) as { value: Record<string, unknown> };
      settingsPosts.push({ path: url.pathname, value: submitted.value });
      if (rejectJellyfinSave) return jsonResponse({ error: 'database unavailable' }, 500);

      const mappings = Array.isArray(submitted.value.mappings)
        ? submitted.value.mappings as Array<Record<string, unknown>>
        : [];
      persistedJellyfinValue = {
        ...submitted.value,
        mappings: mappings.map((mapping) => ({
          mvforgeLibraryId: mapping.mvforgeLibraryId,
          jellyfinLibraryId: mapping.jellyfinLibraryId,
          jellyfinName: mapping.jellyfinName,
          needsRemap: false,
        })),
      };
      return jsonResponse({ key: 'jellyfin', value: persistedJellyfinValue, createdAt: '', updatedAt: 'saved' });
    }

    if (url.pathname === '/api/system/runtime/profiles') return jsonResponse({ profiles: [], effective: {} });
    if (url.pathname === '/api/system/scheduler-recovery') return jsonResponse({});
    if (url.pathname === '/api/system/runtime') return jsonResponse({});
    if (url.pathname === '/api/workers/nodes' || url.pathname === '/api/profiles') return jsonResponse([]);
    throw new Error(`Unexpected test request: ${method} ${url.pathname}`);
  }));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('SettingsPage Jellyfin mapping persistence', () => {
  it('keeps a dirty mapping draft through an unrelated settings refetch', async () => {
    const user = userEvent.setup();
    const queryClient = makeQueryClient();
    renderSettings(queryClient);

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    await addAnimeMapping(user);
    expect(savedMappingIsDisplayed()).toBeTruthy();

    persistedJellyfinValue = {
      ...persistedJellyfinValue,
      lastConnectionSuccessful: false,
      lastSuccessfulConnectionAt: '2026-10-06T12:00:00Z',
      libraries: [{ ...jellyfinLibraries[0], name: 'Anime discovered again' }],
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    });

    expect(settingsReads).toBeGreaterThan(1);
    expect(savedMappingIsDisplayed()).toBeTruthy();
    expect(await screen.findByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /^save settings$/i })).toBeNull();
    expect(settingsPosts).toHaveLength(0);
  });

  it('does not expose the global core save action in Providers & Integrations', async () => {
    renderSettings();

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    expect(screen.queryByRole('button', { name: /^save settings$/i })).toBeNull();
    expect(screen.getByRole('button', { name: /^save jellyfin settings$/i })).toBeTruthy();
  });

  it('saves through the real SettingsPage mutation and restores the mapping after refetch, navigation, and reload', async () => {
    const user = userEvent.setup();
    const view = renderSettings();

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    expect(await screen.findByText(/live reachability: reachable/i)).toBeTruthy();
    await addAnimeMapping(user);
    expect(savedMappingIsDisplayed()).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /save jellyfin settings/i }));

    await waitFor(() => expect(settingsPosts).toHaveLength(1));
    expect(settingsPosts[0].path).toBe('/api/settings/jellyfin');
    expect(settingsPosts[0].value.mappings).toEqual([
      expect.objectContaining({ mvforgeLibraryId: 4, jellyfinLibraryId: 'jf-library-exact-44' }),
    ]);
    expect(await screen.findByText(/settings saved/i)).toBeTruthy();
    await waitFor(() => expect(settingsReads).toBeGreaterThan(1));
    expect(savedMappingIsDisplayed()).toBeTruthy();
    expect(stateReads).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: 'Assets & Storage' }));
    expect(await screen.findByRole('heading', { name: 'Assets & Storage' })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Providers & Integrations' }));
    expect(await screen.findByRole('heading', { name: 'Providers & Integrations' })).toBeTruthy();
    expect(savedMappingIsDisplayed()).toBeTruthy();

    view.unmount();
    renderSettings();
    expect(await screen.findByText(/anime \(id 4\) → anime \(id jf-library-exact-44\)/i)).toBeTruthy();
    expect(settingsReads).toBeGreaterThan(2);
  });

  it('shows a failed Jellyfin save as an unsaved draft', async () => {
    rejectJellyfinSave = true;
    const user = userEvent.setup();
    renderSettings();

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    expect(await screen.findByText(/live reachability: reachable/i)).toBeTruthy();
    await addAnimeMapping(user);
    await user.click(screen.getByRole('button', { name: /save jellyfin settings/i }));

    expect(await screen.findByText(/jellyfin settings could not be saved: database unavailable\. the mapping is still an unsaved draft/i)).toBeTruthy();
    expect(savedMappingIsDisplayed()).toBeTruthy();
    expect(persistedJellyfinValue.mappings).toEqual([]);
  });

  it('refreshes live Jellyfin state without replacing a mapping draft', async () => {
    const user = userEvent.setup();
    const queryClient = makeQueryClient();
    renderSettings(queryClient);

    expect(await screen.findByText(/live reachability: reachable/i)).toBeTruthy();
    await addAnimeMapping(user);

    liveReachability = 'unavailable';
    discoveredStateLibraries = [{ ...jellyfinLibraries[0], name: 'Anime collection refreshed' }];
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['jellyfinIntegrationState'] });
    });

    expect(await screen.findByText(/live reachability: unavailable/i)).toBeTruthy();
    expect(screen.getByText(/anime \(id 4\) → anime collection refreshed \(id jf-library-exact-44\)/i)).toBeTruthy();
    expect(screen.getByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
  });

  it('surfaces an external configuration conflict and reloads only after an explicit choice', async () => {
    const user = userEvent.setup();
    const queryClient = makeQueryClient();
    renderSettings(queryClient);

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    await addAnimeMapping(user);

    persistedJellyfinValue = {
      ...persistedJellyfinValue,
      baseUrl: 'http://jellyfin-external.local:8096',
      connectionConfigRevision: 2,
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    });

    expect(await screen.findByText(/jellyfin settings changed elsewhere\. your draft is preserved/i)).toBeTruthy();
    expect(savedMappingIsDisplayed()).toBeTruthy();
    expect((screen.getByRole('textbox', { name: /jellyfin base url/i }) as HTMLInputElement).value).toBe('http://jellyfin.local:8096');

    await user.click(screen.getByRole('button', { name: /reload persisted settings/i }));

    expect(await screen.findByText(/no mappings configured/i)).toBeTruthy();
    expect((screen.getByRole('textbox', { name: /jellyfin base url/i }) as HTMLInputElement).value).toBe('http://jellyfin-external.local:8096');
    expect(screen.queryByText(/jellyfin settings changed elsewhere/i)).toBeNull();
  });
});
