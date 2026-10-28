// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { useState } from 'react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../../api/client';
import * as ollamaSettingsApi from '../../api/ollama-settings';
import { AICopilotSettingsCard, AIModelRuntimeSettingsCard, OllamaProviderSettingsCard, aiCopilotSettingsValue } from './OllamaSettings';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderWithClient(node: ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<MemoryRouter><QueryClientProvider client={queryClient}>{node}</QueryClientProvider></MemoryRouter>);
}

describe('Ollama provider settings', () => {
  beforeEach(() => {
    vi.spyOn(ollamaSettingsApi, 'testOllamaConnection').mockResolvedValue({ state: 'reachable', checkedAt: '', serverVersion: '0.1.0' });
    vi.spyOn(api, 'listOllamaModels').mockResolvedValue({ state: 'available', installedModels: [{ id: 'shared-model', name: 'shared-model' }] });
    vi.spyOn(ollamaSettingsApi, 'ollamaRuntimeStatus').mockResolvedValue({
      state: 'available', models: [{ modelId: 'shared-model', installed: true, resident: true, state: 'resident', activeInferenceLeases: 1 }], activeOperations: [], failures: [],
    });
  });

  it('keeps save, connection test, model refresh, and runtime status as separate actions', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    renderWithClient(
      <OllamaProviderSettingsCard
        value={{ endpoint: 'http://ollama.local:11434' }}
        savedValue={{ endpoint: 'http://ollama.local:11434', state: 'configured' }}
        configured
        providerConfigurationRevision="provider-v1"
        onDraftChange={onDraftChange}
      />,
    );

    expect(await screen.findByText(/Resident models: 1/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Test connection' }));
    await waitFor(() => expect(ollamaSettingsApi.testOllamaConnection).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/Connection test: Connected/)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Refresh models' }));
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('Installed models (1)')).toBeTruthy();
    expect(screen.getByText('shared-model')).toBeTruthy();
    expect(onDraftChange).not.toHaveBeenCalled();
    expect(screen.getByRole('link', { name: 'Runtime protection and model warmup status' }).getAttribute('href')).toBe('/settings?section=runtime');
  });

  it('reports an empty catalog and disconnected provider without clearing the saved endpoint', async () => {
    const user = userEvent.setup();
    vi.mocked(api.listOllamaModels).mockResolvedValue({ state: 'available', installedModels: [] });
    vi.mocked(ollamaSettingsApi.testOllamaConnection).mockRejectedValue(new Error('Connection refused'));
    renderWithClient(
      <OllamaProviderSettingsCard
        value={{ endpoint: 'http://offline.local:11434' }}
        savedValue={{ endpoint: 'http://offline.local:11434', state: 'configured' }}
        configured
        providerConfigurationRevision="provider-v1"
        onDraftChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Test connection' }));
    expect(await screen.findByText('Connection refused')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Refresh models' }));
    expect(await screen.findByText(/No Ollama models are installed/)).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Ollama endpoint' })).toHaveProperty('value', 'http://offline.local:11434');
  });
});

describe('AI Copilot and runtime settings', () => {
  beforeEach(() => {
    vi.spyOn(api, 'listOllamaModels').mockResolvedValue({ state: 'available', installedModels: [{ id: 'shared-model', name: 'shared-model' }] });
    vi.spyOn(api, 'testOllamaModelReadiness').mockResolvedValue({
      state: 'available', modelId: 'shared-model', available: true, checkedAt: '', scope: 'metadata_check',
    });
  });

  it('selects and readiness-checks its own installed model before enabling Copilot', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    function ControlledCopilot() {
      const [value, setValue] = useState(aiCopilotSettingsValue(undefined));
      return <AICopilotSettingsCard
        value={value}
        providerConfigured
        providerConfigurationRevision="provider-v1"
        onDraftChange={(next) => {
          onDraftChange(next);
          setValue(aiCopilotSettingsValue(next));
        }}
      />;
    }
    renderWithClient(<ControlledCopilot />);

    expect(screen.getByRole('switch', { name: 'AI Copilot disabled' })).toHaveProperty('disabled', true);
    await user.click(screen.getByRole('button', { name: 'Load installed models' }));
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'AI Copilot model' }));
    await user.click(await screen.findByRole('option', { name: 'shared-model' }));
    await user.click(screen.getByRole('combobox', { name: 'Quality preference' }));
    await user.click(await screen.findByRole('option', { name: 'Compression' }));

    expect(onDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'shared-model', preference: 'compression', enabled: false }));
    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(api.testOllamaModelReadiness).toHaveBeenCalledWith('shared-model'));
    await user.click(screen.getByRole('switch', { name: 'AI Copilot disabled' }));
    expect(onDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'shared-model', preference: 'compression', enabled: true }));
  });

  it('shows resident model status and drafts protection controls', async () => {
    const user = userEvent.setup();
    const onDraftChange = vi.fn();
    vi.spyOn(ollamaSettingsApi, 'ollamaRuntimeStatus').mockResolvedValue({
      state: 'available', models: [{ modelId: 'shared-model', installed: true, resident: true, state: 'resident', activeInferenceLeases: 0 }], activeOperations: [], failures: [],
    });
    renderWithClient(
      <AIModelRuntimeSettingsCard
        value={{ enabled: true, maxResidentModels: 1, maxConcurrentInferences: 1, maxConcurrentModelLoads: 1 }}
        configured
        providerConfigurationRevision="provider-v1"
        onDraftChange={onDraftChange}
      />,
    );

    expect(await screen.findByText('shared-model: resident')).toBeTruthy();
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Concurrent inferences' }), { target: { value: '2' } });
    expect(onDraftChange).toHaveBeenLastCalledWith(expect.objectContaining({ maxConcurrentInferences: 2 }));
  });
});
