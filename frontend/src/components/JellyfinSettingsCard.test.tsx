// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Library } from '../api/types';
import { JellyfinSettingsCard } from './JellyfinSettingsCard';

const { testConnection, integrationState, manualRefresh } = vi.hoisted(() => ({ testConnection: vi.fn(), integrationState: vi.fn(), manualRefresh: vi.fn() }));

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      jellyfinOutcomes: vi.fn().mockResolvedValue([]),
      testJellyfinConnection: testConnection,
      jellyfinIntegrationState: integrationState,
      manualJellyfinRefresh: manualRefresh,
    },
  };
});

const libraries: Library[] = [{ id: 4, name: 'Anime', sourcePath: '/raw/anime', destinationPath: '/library/anime', type: 'tv', validationRules: {}, createdAt: '', updatedAt: '' }];
const persistedSettings = {
  enabled: true,
  baseUrl: 'http://jellyfin:8096',
  connectionConfigRevision: 1,
  lastConnectionSuccessful: true,
  lastSuccessfulConnectionAt: '2026-10-05T12:00:00Z',
  liveReachability: 'unknown',
  credentialConfigured: true,
  refreshAfterPublish: true,
  refreshTestEncode: true,
  debounceSeconds: 10,
  librariesLoaded: true,
  configurationValid: true,
  configurationNeedsRevalidation: false,
  mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', jellyfinName: 'Anime' }],
};

const hydratedState = {
  enabled: true,
  credentialConfigured: true,
  configurationValid: true,
  configurationNeedsRevalidation: false,
  lastConnectionSuccessful: true,
  lastSuccessfulConnectionAt: '2026-10-05T12:00:00Z',
  liveReachability: 'reachable' as const,
  librariesLoaded: true,
  libraries: [{ id: 'Jellyfin-Exact-44', name: 'Anime' }],
  mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', needsRemap: false }],
};

function renderCard(value: Record<string, unknown> = persistedSettings, client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(
    <QueryClientProvider client={client}>
      <JellyfinSettingsCard value={value} libraries={libraries} saving={false} onSave={vi.fn()} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  testConnection.mockResolvedValue({
    authenticated: true,
    versionAvailable: true,
    serverVersion: '10.10.0',
    libraries: [{ id: 'Jellyfin-Other-45', name: 'Drama' }],
  });
  integrationState.mockResolvedValue(hydratedState);
  manualRefresh.mockResolvedValue(undefined);
});

afterEach(() => cleanup());

describe('JellyfinSettingsCard', () => {
  it('shows configured state without returning a stored credential in the frontend projection', async () => {
    const onSave = vi.fn();
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    expect(persistedSettings).not.toHaveProperty('apiToken');
    render(
      <QueryClientProvider client={client}>
        <JellyfinSettingsCard
          value={persistedSettings}
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
    expect(screen.queryByText(/stored-plaintext-secret/i)).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: /save jellyfin settings/i }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ enabled: true, mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', jellyfinName: 'Anime' }] }));
    expect(onSave.mock.calls[0][0]).not.toHaveProperty('apiToken');
  });

  it('allows connection testing when persisted settings are clean and configured', async () => {
    renderCard();
    const button = screen.getByRole('button', { name: /test connection and discover libraries/i });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(button);
    await waitFor(() => expect(testConnection).toHaveBeenCalledTimes(1));
  });

  it('hydrates libraries and saved connection state automatically without testing again', async () => {
    integrationState.mockResolvedValue({
      ...hydratedState,
      liveReachability: 'unavailable',
      libraries: [{ id: 'Jellyfin-Exact-44', name: 'Anime' }],
    });
    renderCard();

    expect(await screen.findByText(/last connection succeeded/i)).toBeTruthy();
    await waitFor(() => expect(screen.getAllByRole('alert').map((alert) => alert.textContent ?? '').join(' ')).toContain('Live reachability: unavailable'));
    const statusText = screen.getAllByRole('alert').map((alert) => alert.textContent ?? '').join(' ');
    expect(statusText).toContain('Live reachability: unavailable');
    expect(statusText).toContain('Libraries loaded: yes');
    expect(screen.getByText(/anime \(id 4\) → anime \(id jellyfin-exact-44\)/i)).toBeTruthy();
    expect(testConnection).not.toHaveBeenCalled();
    expect(integrationState).toHaveBeenCalledTimes(1);
  });

  it('shows persisted success while live reachability is still unknown during hydration', () => {
    integrationState.mockImplementation(() => new Promise(() => undefined));
    renderCard();

    expect(screen.getByText(/last connection succeeded/i)).toBeTruthy();
    expect(screen.getByText(/live reachability: unknown/i)).toBeTruthy();
    expect(screen.queryByText(/no successful connection recorded/i)).toBeNull();
  });

  it('marks a persisted mapping whose Jellyfin library disappeared as needing remap', async () => {
    integrationState.mockResolvedValue({
      ...hydratedState,
      libraries: [],
      mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', needsRemap: true }],
    });
    renderCard();

    expect(await screen.findByText(/jellyfin library unavailable \(id jellyfin-exact-44\) · needs remap/i)).toBeTruthy();
  });

  it('revalidates persisted state after navigating away and back', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const first = renderCard(persistedSettings, client);
    await screen.findByText(/last connection succeeded/i);
    first.unmount();
    renderCard(persistedSettings, client);

    await waitFor(() => expect(integrationState).toHaveBeenCalledTimes(2));
    expect(testConnection).not.toHaveBeenCalled();
  });

  it.each([
    ['Base URL', { baseUrl: 'http://replacement-jellyfin:8096' }],
    ['credential', { baseUrl: persistedSettings.baseUrl }],
  ])('re-fetches persisted integration state after a saved %s change', async (_label, change) => {
    integrationState.mockResolvedValueOnce(hydratedState).mockResolvedValueOnce({
      ...hydratedState,
      configurationNeedsRevalidation: true,
      librariesLoaded: false,
      liveReachability: 'unavailable',
    });
    const view = renderCard();
    await screen.findByText(/last connection succeeded/i);

    view.rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <JellyfinSettingsCard
          value={{ ...persistedSettings, ...change, connectionConfigRevision: 2, configurationNeedsRevalidation: true, librariesLoaded: false, lastConnectionSuccessful: true }}
          libraries={libraries}
          saving={false}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByText(/connection settings changed/i)).toBeTruthy();
    await waitFor(() => expect(integrationState).toHaveBeenCalledTimes(2));
    expect(testConnection).not.toHaveBeenCalled();
  });

  it('requires saving a changed Base URL before testing the persisted connection', () => {
    renderCard();
    fireEvent.change(screen.getByLabelText(/jellyfin base url/i), { target: { value: 'http://other-jellyfin:8096' } });

    expect((screen.getByRole('button', { name: /test connection and discover libraries/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
    expect(testConnection).not.toHaveBeenCalled();
  });

  it('requires saving a changed mapping before manual refresh', async () => {
    integrationState
      .mockResolvedValueOnce(hydratedState)
      .mockResolvedValueOnce({ ...hydratedState, libraries: [{ id: 'Jellyfin-Other-45', name: 'Drama' }] });
    renderCard();
    const manualButton = screen.getByRole('button', { name: /request manual refresh/i });
    expect((manualButton as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(manualButton);
    await waitFor(() => expect(manualRefresh).toHaveBeenCalled());
    expect(manualRefresh.mock.calls[0][0]).toBe(4);

    fireEvent.click(screen.getByRole('button', { name: /test connection and discover libraries/i }));
    await screen.findByText(/1 libraries discovered/i);
    fireEvent.mouseDown(screen.getByRole('combobox', { name: /mvforge library/i }));
    fireEvent.click(await screen.findByRole('option', { name: /anime · id 4/i }));
    fireEvent.mouseDown(screen.getByRole('combobox', { name: /discovered jellyfin library/i }));
    fireEvent.click(await screen.findByRole('option', { name: /drama · id jellyfin-other-45/i }));
    fireEvent.click(screen.getByRole('button', { name: /add mapping/i }));

    expect((screen.getByRole('button', { name: /request manual refresh/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(manualRefresh).toHaveBeenCalledTimes(1);
  });

  it.each([
    ['entering a credential', false],
    ['replacing a credential', true],
  ])('%s makes the draft dirty until saved', (_label, credentialConfigured) => {
    renderCard({ ...persistedSettings, credentialConfigured });
    const input = screen.getByLabelText(credentialConfigured ? /replace api key \/ token/i : /^api key \/ token$/i);
    fireEvent.change(input, { target: { value: 'new-write-only-secret' } });

    expect(screen.getByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
    expect((screen.getByRole('button', { name: /test connection and discover libraries/i }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: /request manual refresh/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(testConnection).not.toHaveBeenCalled();
    expect(manualRefresh).not.toHaveBeenCalled();
  });

  it('treats clearing a stored credential as a dirty settings draft', () => {
    renderCard();
    fireEvent.click(screen.getByRole('button', { name: /clear stored credential/i }));

    expect(screen.getByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
    expect((screen.getByRole('button', { name: /test connection and discover libraries/i }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: /request manual refresh/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(testConnection).not.toHaveBeenCalled();
    expect(manualRefresh).not.toHaveBeenCalled();
  });
});
