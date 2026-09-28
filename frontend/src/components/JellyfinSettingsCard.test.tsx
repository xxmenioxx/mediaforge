// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Library } from '../api/types';
import { JellyfinSettingsCard } from './JellyfinSettingsCard';

const { testConnection, manualRefresh } = vi.hoisted(() => ({ testConnection: vi.fn(), manualRefresh: vi.fn() }));

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ...actual,
    api: {
      ...actual.api,
      jellyfinOutcomes: vi.fn().mockResolvedValue([]),
      testJellyfinConnection: testConnection,
      manualJellyfinRefresh: manualRefresh,
    },
  };
});

const libraries: Library[] = [{ id: 4, name: 'Anime', sourcePath: '/raw/anime', destinationPath: '/library/anime', type: 'tv', validationRules: {}, createdAt: '', updatedAt: '' }];
const persistedSettings = {
  enabled: true,
  baseUrl: 'http://jellyfin:8096',
  credentialConfigured: true,
  refreshAfterPublish: true,
  refreshTestEncode: true,
  debounceSeconds: 10,
  mappings: [{ mvforgeLibraryId: 4, jellyfinLibraryId: 'Jellyfin-Exact-44', jellyfinName: 'Anime' }],
};

function renderCard(value: Record<string, unknown> = persistedSettings) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
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

  it('requires saving a changed Base URL before testing the persisted connection', () => {
    renderCard();
    fireEvent.change(screen.getByLabelText(/jellyfin base url/i), { target: { value: 'http://other-jellyfin:8096' } });

    expect((screen.getByRole('button', { name: /test connection and discover libraries/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/save jellyfin settings before testing the connection or requesting a manual refresh/i)).toBeTruthy();
    expect(testConnection).not.toHaveBeenCalled();
  });

  it('requires saving a changed mapping before manual refresh', async () => {
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
