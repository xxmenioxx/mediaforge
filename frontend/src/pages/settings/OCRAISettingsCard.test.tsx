// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { ApiRequestError, api } from '../../api/client';
import type { OCRAISettingsValue, OllamaModelsResponse } from '../../api/types';
import { OCRAISettingsCard, ocrAISettingsValue } from './OCRAISettingsCard';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderCard(
  value: OCRAISettingsValue = ocrAISettingsValue(undefined),
  onDraftChange = vi.fn(),
  providerConfigured = true,
  providerConfigurationRevision = 'provider-v1',
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  function ControlledCard({ revision }: { revision: string }) {
    const [draft, setDraft] = useState(value);
    return (
      <OCRAISettingsCard
        value={draft}
        providerConfigured={providerConfigured}
        providerConfigurationRevision={revision}
        onDraftChange={(next) => {
          onDraftChange(next);
          setDraft(ocrAISettingsValue(next));
        }}
      />
    );
  }
  const view = render(
    <QueryClientProvider client={queryClient}>
      <ControlledCard revision={providerConfigurationRevision} />
    </QueryClientProvider>,
  );
  function rerenderWithProviderConfigurationRevision(revision: string) {
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <ControlledCard revision={revision} />
      </QueryClientProvider>,
    );
  }
  return { onDraftChange, queryClient, rerenderWithProviderConfigurationRevision, view };
}

function catalog(installedModels: OllamaModelsResponse['installedModels']): OllamaModelsResponse {
  return { state: 'available', installedModels };
}

describe('OCRAISettingsCard', () => {
  beforeEach(() => {
    vi.spyOn(api, 'listOllamaModels').mockResolvedValue(catalog([]));
    vi.spyOn(api, 'testOllamaModelReadiness').mockResolvedValue({
      state: 'available',
      modelId: 'qwen2.5:7b',
      available: true,
      checkedAt: '2026-10-20T12:00:00Z',
      scope: 'metadata_check',
    });
  });

  it('defaults off with no selected model and does not discover models automatically', () => {
    renderCard(ocrAISettingsValue(undefined), vi.fn(), false);

    expect(screen.getByRole('switch', { name: 'Enable OCR AI' })).toHaveProperty('checked', false);
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('No model selected');
    expect(screen.getByText('Configure the shared Ollama provider before loading or checking models.')).toBeTruthy();
    expect(api.listOllamaModels).not.toHaveBeenCalled();
    expect(api.testOllamaModelReadiness).not.toHaveBeenCalled();
  });

  it('uses an explicitly selected catalog model and requires a readiness check before enabling', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    vi.mocked(api.listOllamaModels).mockResolvedValue(catalog([
      { id: 'qwen2.5:7b', name: 'qwen2.5:7b' },
    ]));
    renderCard(ocrAISettingsValue(undefined), onDraftChange);

    await user.click(screen.getByRole('button', { name: 'Load installed models' }));
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'OCR AI model' }));
    await user.click(screen.getByRole('option', { name: 'qwen2.5:7b' }));

    const enabled = screen.getByRole('switch', { name: 'Enable OCR AI' });
    expect(enabled).toHaveProperty('checked', false);
    expect(enabled).toHaveProperty('disabled', true);
    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, model: 'qwen2.5:7b' }));

    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(vi.mocked(api.testOllamaModelReadiness).mock.calls[0]?.[0]).toBe('qwen2.5:7b'));
    await user.click(screen.getByRole('switch', { name: 'Enable OCR AI' }));
    expect(onDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ enabled: true, model: 'qwen2.5:7b' }));
  });

  it('turning OCR AI off preserves the selected model and makes no model or inference request', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    renderCard(ocrAISettingsValue({ enabled: true, model: 'qwen2.5:7b' }), onDraftChange);

    await user.click(screen.getByRole('switch', { name: 'Enable OCR AI' }));

    expect(onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, model: 'qwen2.5:7b' }));
    expect(api.listOllamaModels).not.toHaveBeenCalled();
    expect(api.testOllamaModelReadiness).not.toHaveBeenCalled();
  });

  it('reports a missing saved model without clearing or replacing it', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listOllamaModels).mockResolvedValue(catalog([{ id: 'other-model', name: 'other-model' }]));
    vi.mocked(api.testOllamaModelReadiness).mockRejectedValue(new ApiRequestError('missing', 404, { state: 'model_not_found' }));
    renderCard(ocrAISettingsValue({ enabled: false, model: 'removed-model' }));

    await user.click(screen.getByRole('button', { name: 'Load installed models' }));
    await waitFor(() => expect(screen.getByText(/saved model is absent/i)).toBeTruthy());
    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(screen.getByText(/selected model was not found/i)).toBeTruthy());
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('removed-model');
  });

  it('reports provider offline while retaining the selected model', async () => {
    const user = userEvent.setup();
    vi.mocked(api.testOllamaModelReadiness).mockRejectedValue(new ApiRequestError('unavailable', 503, { state: 'unavailable' }));
    renderCard(ocrAISettingsValue({ enabled: false, model: 'qwen2.5:7b' }));

    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(screen.getByText(/provider is unavailable or returned an invalid response/i)).toBeTruthy());
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('qwen2.5:7b');
  });

  it('invalidates successful readiness when provider configuration changes and when a fresh check reports offline', async () => {
    const user = userEvent.setup();
    const { rerenderWithProviderConfigurationRevision } = renderCard(
      ocrAISettingsValue({ enabled: false, model: 'qwen2.5:7b' }),
      vi.fn(),
      true,
      'provider-revision-1',
    );

    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(api.testOllamaModelReadiness).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('switch', { name: 'Enable OCR AI' })).toHaveProperty('disabled', false);

    rerenderWithProviderConfigurationRevision('provider-revision-2');
    expect(screen.getByRole('switch', { name: 'Enable OCR AI' })).toHaveProperty('disabled', true);
    expect(screen.queryByText(/available at the shared provider/i)).toBeNull();
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('qwen2.5:7b');

    vi.mocked(api.testOllamaModelReadiness).mockRejectedValueOnce(
      new ApiRequestError('provider offline', 503, { state: 'unavailable' }),
    );
    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    expect(screen.getByRole('switch', { name: 'Enable OCR AI' })).toHaveProperty('disabled', true);
    await waitFor(() => expect(screen.getByText(/provider is unavailable or returned an invalid response/i)).toBeTruthy());
    expect(screen.getByRole('switch', { name: 'Enable OCR AI' })).toHaveProperty('disabled', true);
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('qwen2.5:7b');
  });
});
