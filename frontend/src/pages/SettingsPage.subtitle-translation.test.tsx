// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiRequestError, api } from '../api/client';
import type { OllamaModelReadinessResponse } from '../api/types';
import { AITranslationSettingsCard } from './settings/OllamaSettings';
import { subtitleTranslationSettingsValue } from '../utils/settingsPageHelpers';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(api, 'listOllamaModels').mockResolvedValue({ state: 'available', installedModels: [] });
  vi.spyOn(api, 'testOllamaModelReadiness').mockResolvedValue(readiness());
  vi.spyOn(api, 'enableSubtitleTranslation').mockResolvedValue({ configured: true, enabled: true, readiness: 'ready', lastCheckedAt: '', diagnostic: '' });
});

function configuredValue(overrides: Record<string, unknown> = {}) {
  return subtitleTranslationSettingsValue({
    provider: 'ollama',
    ollamaEndpoint: 'http://nas.local:11434',
    ollamaModel: 'qwen2.5:7b',
    readiness: 'untested',
    ...overrides,
  });
}

function readiness(overrides: Partial<OllamaModelReadinessResponse> = {}): OllamaModelReadinessResponse {
  return {
    state: 'available', modelId: 'qwen2.5:7b', available: true,
    checkedAt: '2026-10-20T12:00:00Z', scope: 'metadata_check', ...overrides,
  };
}

function renderCard(value = configuredValue(), options: { configDirty?: boolean; providerConfigured?: boolean } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const onDraftChange = vi.fn();
  const onDisable = vi.fn();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <AITranslationSettingsCard
        value={value}
        configDirty={options.configDirty ?? false}
        providerConfigured={options.providerConfigured ?? true}
        providerConfigurationRevision="provider-v1"
        disableSaving={false}
        onDraftChange={onDraftChange}
        onDisable={onDisable}
      />
    </QueryClientProvider>,
  );
  return { onDisable, onDraftChange, queryClient, view };
}

describe('AITranslationSettingsCard', () => {
  it('uses the shared installed-model catalog and never exposes an endpoint field', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    vi.mocked(api.listOllamaModels).mockResolvedValue({
      state: 'available', installedModels: [{ id: 'qwen2.5:7b', name: 'qwen2.5:7b' }],
    });
    cleanup();
    const card = render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <AITranslationSettingsCard value={configuredValue({ ollamaModel: 'old-model' })} configDirty={false} providerConfigured providerConfigurationRevision="provider-v1" disableSaving={false} onDraftChange={onDraftChange} onDisable={vi.fn()} />
      </QueryClientProvider>,
    );

    expect(screen.queryByRole('textbox', { name: 'Ollama endpoint' })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Load installed models' }));
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'AI Translation model' }));
    await user.click(await screen.findByRole('option', { name: 'qwen2.5:7b' }));

    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ ollamaModel: 'qwen2.5:7b' }));
    expect(onDraftChange.mock.calls[0]?.[0]).not.toHaveProperty('ollamaEndpoint');
    expect(api.listOllamaModels).toHaveBeenCalledTimes(1);
    card.unmount();
  });

  it('requires a successful explicit readiness check before enabling and retains a missing model', async () => {
    const user = userEvent.setup();
    vi.mocked(api.testOllamaModelReadiness).mockRejectedValue(new ApiRequestError('missing', 404, { state: 'model_not_found' }));
    renderCard(configuredValue({ ollamaModel: 'missing-model' }));

    const enabled = screen.getByRole('switch', { name: 'AI Translation disabled' });
    expect(enabled).toHaveProperty('disabled', true);
    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(api.testOllamaModelReadiness).toHaveBeenCalledWith('missing-model'));
    expect(await screen.findByText(/selected model was not found/i)).toBeTruthy();
    expect(screen.getByRole('switch', { name: 'AI Translation disabled' })).toHaveProperty('disabled', true);
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('enables only after readiness succeeds and disables independently when provider is offline', async () => {
    const user = userEvent.setup();
    vi.mocked(api.testOllamaModelReadiness).mockResolvedValue(readiness());
    vi.mocked(api.enableSubtitleTranslation).mockResolvedValue({ configured: true, enabled: true, readiness: 'ready', lastCheckedAt: '', diagnostic: '' });
    renderCard(configuredValue());

    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(api.testOllamaModelReadiness).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('switch', { name: 'AI Translation disabled' }));
    await waitFor(() => expect(api.enableSubtitleTranslation).toHaveBeenCalledTimes(1));

    cleanup();
    const offline = renderCard(configuredValue({ enabled: true }), { providerConfigured: false });
    await user.click(screen.getByRole('switch', { name: 'AI Translation enabled' }));
    expect(offline.onDisable).toHaveBeenCalledTimes(1);
    expect(api.testOllamaModelReadiness).toHaveBeenCalledTimes(1);
  });

  it('keeps changes in the central Subtitles draft before readiness actions', () => {
    renderCard(configuredValue(), { configDirty: true });
    expect(screen.getByRole('button', { name: 'Check selected model' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('switch', { name: 'AI Translation disabled' })).toHaveProperty('disabled', true);
    expect(api.testOllamaModelReadiness).not.toHaveBeenCalled();
  });
});
