// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { SubtitleTranslationReadiness } from '../api/types';
import { SubtitleTranslationSettingsCard } from './SettingsPage';
import { subtitleTranslationSettingsValue } from '../utils/settingsPageHelpers';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderCard(value = subtitleTranslationSettingsValue(undefined), onSave = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <SubtitleTranslationSettingsCard value={value} saving={false} onSave={onSave} />
    </QueryClientProvider>,
  );
  return { queryClient, onSave, view };
}

function configuredValue(overrides: Record<string, unknown> = {}) {
  return subtitleTranslationSettingsValue({
    provider: 'ollama',
    ollamaEndpoint: 'http://nas.local:11434',
    ollamaModel: 'qwen2.5:7b',
    readiness: 'untested',
    ...overrides,
  });
}

function readiness(overrides: Partial<SubtitleTranslationReadiness> = {}): SubtitleTranslationReadiness {
  return {
    configured: true,
    enabled: false,
    readiness: 'ready',
    lastCheckedAt: '2026-09-20T23:30:00Z',
    diagnostic: '',
    ...overrides,
  };
}

describe('SubtitleTranslationSettingsCard', () => {
  beforeEach(() => {
    vi.spyOn(api, 'testSubtitleTranslationConnection').mockResolvedValue(readiness());
    vi.spyOn(api, 'enableSubtitleTranslation').mockResolvedValue(readiness({ enabled: true }));
    vi.spyOn(api, 'updateSetting').mockResolvedValue({
      key: 'subtitleTranslationSettings',
      value: {},
      createdAt: '',
      updatedAt: '',
    });
  });

  it('defaults missing settings to not configured and disabled', () => {
    renderCard();

    expect(screen.getByText('Configured: No')).toBeTruthy();
    expect(screen.getByText('Enabled: No')).toBeTruthy();
    expect(screen.getByText('Readiness: Not configured')).toBeTruthy();
    expect(api.testSubtitleTranslationConnection).not.toHaveBeenCalled();
  });

  it('saves Ollama configuration without enabling through generic settings', async () => {
    const user = userEvent.setup();
    const { onSave } = renderCard();

    await user.click(screen.getByRole('combobox', { name: 'Provider' }));
    await user.click(screen.getByRole('option', { name: 'Ollama' }));
    await user.type(screen.getByRole('textbox', { name: 'Ollama endpoint' }), 'http://nas.local:11434');
    await user.type(screen.getByRole('textbox', { name: 'Ollama model' }), 'qwen2.5:7b');
    await user.click(screen.getByRole('button', { name: 'Save translation configuration' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'ollama',
      ollamaEndpoint: 'http://nas.local:11434',
      ollamaModel: 'qwen2.5:7b',
      maxCuesPerRequest: 32,
      maxRequestBytes: 65536,
      maxContextCues: 8,
      maxContextChars: 4096,
    }));
    expect(onSave.mock.calls[0]?.[0]).not.toHaveProperty('enabled', true);
    expect(api.testSubtitleTranslationConnection).not.toHaveBeenCalled();
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('trusts backend invalidation after a provider-defining configuration change', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const view = render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <SubtitleTranslationSettingsCard value={configuredValue({ enabled: true, readiness: 'ready', ollamaModel: 'model-a' })} saving={false} onSave={onSave} />
      </QueryClientProvider>,
    );

    const model = screen.getByRole('textbox', { name: 'Ollama model' });
    await user.clear(model);
    await user.type(model, 'model-b');
    await user.click(screen.getByRole('button', { name: 'Save translation configuration' }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ ollamaModel: 'model-b' }));

    view.rerender(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
        <SubtitleTranslationSettingsCard value={configuredValue({ enabled: false, readiness: 'untested', ollamaModel: 'model-b' })} saving={false} onSave={onSave} />
      </QueryClientProvider>,
    );
    await waitFor(() => {
      expect(screen.getByText('Enabled: No')).toBeTruthy();
      expect(screen.getByText('Readiness: Untested')).toBeTruthy();
    });
  });

  it('shows successful Test Connection as ready without enabling translation', async () => {
    const user = userEvent.setup();
    const testConnection = vi.mocked(api.testSubtitleTranslationConnection);
    testConnection.mockResolvedValue(readiness({ enabled: false }));
    renderCard(configuredValue());

    await user.click(screen.getByRole('button', { name: 'Test Connection' }));

    await waitFor(() => expect(screen.getByText('Readiness: Ready')).toBeTruthy());
    expect(testConnection).toHaveBeenCalledTimes(1);
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
    expect(screen.getByText('Enabled: No')).toBeTruthy();
    expect(screen.getByText(/Last checked:/)).toBeTruthy();
  });

  it('shows model-not-found readiness diagnostics without enabling', async () => {
    const user = userEvent.setup();
    vi.mocked(api.testSubtitleTranslationConnection).mockResolvedValue(readiness({
      readiness: 'model_not_found',
      diagnostic: 'Configured translation model is unavailable.',
    }));
    renderCard(configuredValue());

    await user.click(screen.getByRole('button', { name: 'Test Connection' }));

    expect(await screen.findByText('Readiness: Model not found')).toBeTruthy();
    expect(screen.getByText('Configured translation model is unavailable.')).toBeTruthy();
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('enables only through the dedicated readiness-validated endpoint', async () => {
    const user = userEvent.setup();
    vi.mocked(api.enableSubtitleTranslation).mockResolvedValue(readiness({ enabled: true }));
    renderCard(configuredValue({ readiness: 'ready', enabled: false }));

    await user.click(screen.getByRole('button', { name: 'Enable translation' }));

    await waitFor(() => expect(api.enableSubtitleTranslation).toHaveBeenCalledTimes(1));
    expect(api.updateSetting).not.toHaveBeenCalledWith(expect.objectContaining({
      value: expect.objectContaining({ enabled: true }),
    }));
    expect(await screen.findByText('Enabled: Yes')).toBeTruthy();
  });

  it('refreshes settings after a failed Enable and renders the persisted unavailable state', async () => {
    const user = userEvent.setup();
    const { queryClient, view } = renderCard(configuredValue({ readiness: 'ready', enabled: false }));
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    vi.mocked(api.enableSubtitleTranslation).mockRejectedValue(new Error('Request failed with 409'));

    await user.click(screen.getByRole('button', { name: 'Enable translation' }));
    await waitFor(() => expect(api.enableSubtitleTranslation).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['settings'] }));

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <SubtitleTranslationSettingsCard
          value={configuredValue({
            enabled: false,
            readiness: 'unavailable',
            readinessDiagnostic: 'Translation provider is unavailable.',
            readinessCheckedAt: '2026-09-21T00:15:00Z',
          })}
          saving={false}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Enabled: No')).toBeTruthy();
      expect(screen.getByText('Readiness: Unavailable')).toBeTruthy();
      expect(screen.getByText('Translation provider is unavailable.')).toBeTruthy();
      expect(screen.getByText(/Last checked:/)).toBeTruthy();
    });
    expect(screen.queryByText('Readiness: Ready')).toBeNull();
  });

  it('refreshes settings after a failed Test Connection and renders refreshed diagnostics', async () => {
    const user = userEvent.setup();
    const { queryClient, view } = renderCard(configuredValue({ readiness: 'untested', enabled: false }));
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    vi.mocked(api.testSubtitleTranslationConnection).mockRejectedValue(new Error('Request failed with 503'));

    await user.click(screen.getByRole('button', { name: 'Test Connection' }));
    await waitFor(() => expect(api.testSubtitleTranslationConnection).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['settings'] }));

    view.rerender(
      <QueryClientProvider client={queryClient}>
        <SubtitleTranslationSettingsCard
          value={configuredValue({
            enabled: false,
            readiness: 'model_not_found',
            readinessDiagnostic: 'Configured translation model is unavailable.',
            readinessCheckedAt: '2026-09-21T00:16:00Z',
          })}
          saving={false}
          onSave={vi.fn()}
        />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Enabled: No')).toBeTruthy();
      expect(screen.getByText('Readiness: Model not found')).toBeTruthy();
      expect(screen.getByText('Configured translation model is unavailable.')).toBeTruthy();
      expect(screen.getByText(/Last checked:/)).toBeTruthy();
    });
    expect(screen.queryByText('Readiness: Untested')).toBeNull();
  });

  it('disables without testing or enabling and preserves configuration policy', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const value = configuredValue({ enabled: true, readiness: 'ready', maxCuesPerRequest: 24, maxRequestBytes: 32000, maxContextCues: 6, maxContextChars: 3000 });
    renderCard(value, onSave);

    await user.click(screen.getByRole('button', { name: 'Disable translation' }));

    expect(onSave).toHaveBeenCalledWith({
      provider: 'ollama',
      ollamaEndpoint: 'http://nas.local:11434',
      ollamaModel: 'qwen2.5:7b',
      maxCuesPerRequest: 24,
      maxRequestBytes: 32000,
      maxContextCues: 6,
      maxContextChars: 3000,
      enabled: false,
    });
    expect(api.testSubtitleTranslationConnection).not.toHaveBeenCalled();
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('blocks readiness actions while provider configuration is dirty', async () => {
    const user = userEvent.setup();
    renderCard(configuredValue({ readiness: 'ready', enabled: false, ollamaModel: 'model-a' }));

    await user.clear(screen.getByRole('textbox', { name: 'Ollama model' }));
    await user.type(screen.getByRole('textbox', { name: 'Ollama model' }), 'model-b');

    expect(screen.getByText('Save configuration before testing or enabling.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Test Connection' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('button', { name: 'Enable translation' })).toHaveProperty('disabled', true);
    expect(api.testSubtitleTranslationConnection).not.toHaveBeenCalled();
    expect(api.enableSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('uses persisted configuration when disabling with a dirty draft', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    renderCard(configuredValue({ enabled: true, readiness: 'ready', ollamaModel: 'model-a' }), onSave);

    await user.clear(screen.getByRole('textbox', { name: 'Ollama model' }));
    await user.type(screen.getByRole('textbox', { name: 'Ollama model' }), 'unsaved-model');
    await user.click(screen.getByRole('button', { name: 'Disable translation' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      ollamaModel: 'model-a',
      enabled: false,
    }));
  });
});
