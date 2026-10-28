// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
    ApiRequestError: actual.ApiRequestError,
    api: {
      ...actual.api,
      settings: vi.fn(),
      libraries: vi.fn(),
      runtimeState: vi.fn(),
      workerNodes: vi.fn(),
      runtimeProfiles: vi.fn(),
      profiles: vi.fn(),
      schedulerRecovery: vi.fn(),
      updateSetting: vi.fn(),
      updateSettingsBatch: vi.fn(),
      testOllamaConnection: vi.fn(),
      listOllamaModels: vi.fn(),
      ollamaRuntimeStatus: vi.fn(),
      jellyfinIntegrationState: vi.fn(),
      jellyfinOutcomes: vi.fn(),
    },
  };
});

import { api } from '../api/client';
import { SettingsPage } from './SettingsPage';

function CurrentSettingsSearch() {
  const location = useLocation();
  return <div data-testid="settings-search">{location.search}</div>;
}

function SettingsRouteControls() {
  const navigate = useNavigate();
  return (
    <>
      <button onClick={() => navigate('/settings?section=general')}>Test route General</button>
      <button onClick={() => navigate('/settings?section=subtitles')}>Test route Subtitles</button>
      <button onClick={() => navigate('/settings?section=ai-copilot')}>Test route AI Copilot</button>
    </>
  );
}

function renderSettings(initialPath: string, includeRouteControls = false) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <MemoryRouter initialEntries={[initialPath]}>
      <QueryClientProvider client={queryClient}>
        <SettingsPage />
        <CurrentSettingsSearch />
        {includeRouteControls ? <SettingsRouteControls /> : null}
      </QueryClientProvider>
    </MemoryRouter>,
  );
  return { ...view, queryClient };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise; });
  return { promise, resolve };
}

function sidebarDraftStatus(sectionTitle: string): boolean {
  const sidebar = screen.getByRole('navigation', { name: 'Settings sections' });
  const section = within(sidebar).getByRole('button', { name: new RegExp(`^${sectionTitle}`) });
  return within(section).queryByText('Draft') !== null;
}

afterEach(() => {
  cleanup();
  window.sessionStorage.clear();
  vi.restoreAllMocks();
});

describe('Settings domain navigation and drafts', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    vi.mocked(api.updateSettingsBatch).mockClear();
    vi.mocked(api.updateSetting).mockClear();
    vi.mocked(api.listOllamaModels).mockClear();
    vi.mocked(api.testOllamaConnection).mockClear();
    vi.mocked(api.ollamaRuntimeStatus).mockClear();
    vi.mocked(api.jellyfinIntegrationState).mockClear();
    vi.mocked(api.jellyfinOutcomes).mockClear();
    vi.mocked(api.settings).mockResolvedValue([{ key: 'workers', value: {
      defaultWorkerName: 'local',
      autoWorkerEnabled: true,
      maxConcurrentJobs: 2,
      maxJobsPerBatch: 8,
      delaySecondsBetweenJobs: 0,
      batchCooldownSeconds: 0,
      dryRunOnly: false,
    } }] as never);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.runtimeState).mockResolvedValue({
      selectedProfile: 'desktop_safe',
      recommendedProfile: 'desktop_safe',
      preferredProfile: '',
      appliedOverrides: [],
      os: 'linux',
      architecture: 'amd64',
      cpuCores: 4,
      cpuLoad1: 0,
      totalMemoryBytes: 1024,
      availableMemoryBytes: 512,
      container: true,
      batteryPresent: false,
      onBattery: false,
      batteryPercent: 0,
      powerSource: 'AC',
      disks: {},
      encoders: {},
      selectionReasons: [],
      warnings: [],
      detectedAt: new Date().toISOString(),
    } as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.runtimeProfiles).mockResolvedValue({ profiles: [] } as never);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.schedulerRecovery).mockResolvedValue({
      ranAt: new Date().toISOString(),
      interruptedJobs: 0,
      partialOutputsPreserved: 0,
      reservationsReleased: 0,
      workersMarkedOffline: 0,
      missingCompletedOutputs: 0,
      missingPublishedOutputs: 0,
      orphanWorkspacePaths: [],
      warnings: [],
    } as never);
    vi.mocked(api.updateSettingsBatch).mockResolvedValue([] as never);
    vi.mocked(api.ollamaRuntimeStatus).mockResolvedValue({ state: 'available', models: [], activeOperations: [], failures: [] } as never);
    vi.mocked(api.jellyfinIntegrationState).mockResolvedValue({ enabled: false, credentialConfigured: false, configurationValid: false, configurationNeedsRevalidation: false, lastConnectionSuccessful: false, liveReachability: 'unknown', librariesLoaded: false, libraries: [], mappings: [] } as never);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
  });

  it('keeps a runtime draft across section navigation and saves only that section atomically', async () => {
    const user = userEvent.setup();
    const firstPage = renderSettings('/settings?section=runtime');
    const workerLimit = await screen.findByLabelText('Max concurrent jobs');

    await user.clear(workerLimit);
    await user.type(workerLimit, '3');
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');

    await user.click(screen.getByRole('button', { name: /Assets & Storage/ }));
    expect(await screen.findByRole('heading', { name: 'Assets & Storage' })).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId('settings-search').textContent).toBe('?section=assets'));

    firstPage.unmount();
    renderSettings('/settings');
    const restoredWorkerLimit = await screen.findByLabelText('Max concurrent jobs');
    expect((restoredWorkerLimit as HTMLInputElement).value).toBe('3');
    expect(await screen.findByText('An unsaved settings draft was restored from this tab session.')).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0];
    expect(batch.settings.map((setting) => setting.key)).toEqual(['workers', 'runtimePolicy', 'directPlay', 'aiResourceProtectionSettings']);
    expect(batch.settings[0].value).toEqual(expect.objectContaining({ maxConcurrentJobs: 3 }));
    expect(api.updateSetting).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Saved'));
  }, 15000);

  it('preserves the OCR AI draft through settings refetch and navigation, then saves and refetches it centrally', async () => {
    const user = userEvent.setup();
    const initialSettings = [
      { key: 'workers', value: { defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8, delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false } },
      { key: 'ollamaProviderSettings', value: { state: 'configured' } },
      { key: 'subtitleTranslationSettings', value: { enabled: true, model: 'translation-model' } },
      { key: 'ocrAISettings', value: { enabled: false, model: '' } },
    ];
    const changedElsewhereSettings = initialSettings.map((setting) => setting.key === 'ocrAISettings'
      ? { ...setting, value: { enabled: false, model: 'external-model' } }
      : setting);
    const savedSettings = initialSettings.map((setting) => setting.key === 'ocrAISettings'
      ? { ...setting, value: { enabled: false, model: 'ocr-model:latest' } }
      : setting);
    vi.mocked(api.settings).mockReset()
      .mockResolvedValueOnce(initialSettings as never)
      .mockResolvedValueOnce(changedElsewhereSettings as never)
      .mockResolvedValueOnce(changedElsewhereSettings as never)
      .mockResolvedValue(savedSettings as never);
    vi.mocked(api.listOllamaModels).mockResolvedValue({
      state: 'available',
      installedModels: [{ id: 'ocr-model:latest', name: 'ocr-model:latest' }],
    } as never);

    let page = renderSettings('/settings?section=subtitles', true);
    const modelSelect = await screen.findByRole('combobox', { name: 'OCR AI model' });
    expect(modelSelect.textContent).toContain('No model selected');

    await user.click(screen.getAllByRole('button', { name: 'Load installed models' })[1]!);
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'OCR AI model' }));
    await user.click(await screen.findByRole('option', { name: 'ocr-model:latest' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('Subtitles')).toBe(true);

    await page.queryClient.invalidateQueries({ queryKey: ['settings'] });
    expect(api.settings).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('ocr-model:latest');
    expect(await screen.findByText(/settings changed elsewhere/i)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Test route General' }));
    expect(await screen.findByRole('heading', { name: 'General' })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Test route Subtitles' }));
    expect(await screen.findByRole('heading', { name: 'Subtitles' })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('ocr-model:latest');

    page.unmount();
    page = renderSettings('/settings?section=subtitles', true);
    expect((await screen.findByRole('combobox', { name: 'OCR AI model' })).textContent).toContain('ocr-model:latest');
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Unsaved changes'));

    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings).toEqual([
      { key: 'ocrAISettings', value: { enabled: false, model: 'ocr-model:latest' } },
    ]);
    await waitFor(() => expect(api.settings).toHaveBeenCalledTimes(4));
    expect(screen.getByRole('combobox', { name: 'OCR AI model' }).textContent).toContain('ocr-model:latest');
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Saved'));
    expect(sidebarDraftStatus('Subtitles')).toBe(false);
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('navigates to URL-addressable AI Copilot settings and saves its independent model draft', async () => {
    const user = userEvent.setup();
    const initialSettings = [
      { key: 'workers', value: { defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8, delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false } },
      { key: 'ollamaProviderSettings', value: { endpoint: 'http://ollama.local:11434', state: 'configured' }, updatedAt: 'provider-v1' },
      { key: 'aiCopilotSettings', value: { enabled: false, model: '', preference: 'balanced' } },
    ];
    const savedSettings = initialSettings.map((setting) => setting.key === 'aiCopilotSettings'
      ? { ...setting, value: { enabled: true, model: 'shared-model', preference: 'balanced' } }
      : setting);
    vi.mocked(api.settings).mockReset().mockResolvedValueOnce(initialSettings as never).mockResolvedValueOnce(initialSettings as never).mockResolvedValue(savedSettings as never);
    vi.mocked(api.listOllamaModels).mockResolvedValue({ state: 'available', installedModels: [{ id: 'shared-model', name: 'shared-model' }] } as never);
    vi.spyOn(api, 'testOllamaModelReadiness').mockResolvedValue({ state: 'available', modelId: 'shared-model', available: true, checkedAt: '', scope: 'metadata_check' });
    vi.mocked(api.updateSettingsBatch).mockResolvedValue([savedSettings[2]] as never);

    const page = renderSettings('/settings?section=ai-copilot');
    expect(await screen.findAllByRole('heading', { name: 'AI Copilot' })).toHaveLength(2);
    expect(screen.getByTestId('settings-search').textContent).toBe('?section=ai-copilot');
    await user.click(screen.getByRole('button', { name: 'Load installed models' }));
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'AI Copilot model' }));
    await user.click(await screen.findByRole('option', { name: 'shared-model' }));
    await user.click(screen.getByRole('button', { name: 'Check selected model' }));
    await waitFor(() => expect(api.testOllamaModelReadiness).toHaveBeenCalledWith('shared-model'));
    await page.queryClient.invalidateQueries({ queryKey: ['settings'] });
    expect(screen.getByRole('combobox', { name: 'AI Copilot model' }).textContent).toContain('shared-model');
    await user.click(screen.getByRole('switch', { name: 'AI Copilot disabled' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('AI Copilot')).toBe(true);

    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0]?.[0].settings).toEqual([
      { key: 'aiCopilotSettings', value: { enabled: true, model: 'shared-model', preference: 'balanced' } },
    ]);
  });

  it('saves the Translation model in Subtitles without persisting the legacy endpoint projection', async () => {
    const user = userEvent.setup();
    const initialSettings = [
      { key: 'workers', value: { defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8, delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false } },
      { key: 'ollamaProviderSettings', value: { endpoint: 'http://ollama.local:11434', state: 'configured' }, updatedAt: 'provider-v1' },
      { key: 'subtitleTranslationSettings', value: { provider: 'ollama', ollamaEndpoint: 'http://ollama.local:11434', ollamaModel: 'old-model', enabled: true, readiness: 'ready', qualityReviewerModel: 'review-model' } },
      { key: 'ocrAISettings', value: { enabled: false, model: '' } },
    ];
    const savedTranslation = {
      key: 'subtitleTranslationSettings',
      value: { provider: 'ollama', ollamaEndpoint: 'http://ollama.local:11434', ollamaModel: 'shared-model', enabled: false, readiness: 'untested', qualityReviewerModel: 'review-model' },
    };
    vi.mocked(api.settings).mockReset().mockResolvedValue(initialSettings as never);
    vi.mocked(api.listOllamaModels).mockResolvedValue({ state: 'available', installedModels: [{ id: 'shared-model', name: 'shared-model' }] } as never);
    vi.mocked(api.updateSettingsBatch).mockResolvedValue([savedTranslation] as never);

    renderSettings('/settings?section=subtitles');
    expect(await screen.findByRole('heading', { name: 'AI Translation' })).toBeTruthy();
    expect(screen.queryByRole('textbox', { name: 'Ollama endpoint' })).toBeNull();
    const modelLoad = screen.getAllByRole('button', { name: 'Load installed models' })[0];
    await user.click(modelLoad!);
    await waitFor(() => expect(api.listOllamaModels).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('combobox', { name: 'AI Translation model' }));
    await user.click(await screen.findByRole('option', { name: 'shared-model' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));

    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0]?.[0].settings).toEqual([{
      key: 'subtitleTranslationSettings',
      value: expect.objectContaining({ provider: 'ollama', ollamaModel: 'shared-model', qualityReviewerModel: 'review-model' }),
    }]);
    const savedValue = vi.mocked(api.updateSettingsBatch).mock.calls[0]?.[0].settings[0]?.value;
    expect(savedValue).not.toHaveProperty('ollamaEndpoint');
    expect(savedValue).not.toHaveProperty('enabled');
    expect(savedValue).not.toHaveProperty('readiness');
  });

  it('saves the one shared Ollama endpoint from Providers & Integrations', async () => {
    const user = userEvent.setup();
    const initialSettings = [
      { key: 'workers', value: { defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8, delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false } },
      { key: 'ollamaProviderSettings', value: { endpoint: 'http://old.local:11434', state: 'configured' }, updatedAt: 'provider-v1' },
      { key: 'jellyfin', value: { enabled: false, mappings: [] } },
    ];
    const savedProvider = { key: 'ollamaProviderSettings', value: { endpoint: 'http://new.local:11434', state: 'configured' }, updatedAt: 'provider-v2' };
    vi.mocked(api.settings).mockReset().mockResolvedValueOnce(initialSettings as never).mockResolvedValue([ ...initialSettings.filter((setting) => setting.key !== 'ollamaProviderSettings'), savedProvider ] as never);
    vi.mocked(api.updateSettingsBatch).mockResolvedValue([savedProvider] as never);

    renderSettings('/settings?section=providers-integrations');
    expect(await screen.findByRole('heading', { name: 'Ollama' })).toBeTruthy();
    expect(screen.getAllByRole('textbox', { name: 'Ollama endpoint' })).toHaveLength(1);
    const endpoint = screen.getByRole('textbox', { name: 'Ollama endpoint' });
    await waitFor(() => expect(endpoint).toHaveProperty('value', 'http://old.local:11434'));
    await user.clear(endpoint);
    await waitFor(() => expect(endpoint).toHaveProperty('value', ''));
    await user.type(endpoint, 'http://new.local:11434');
    expect(sidebarDraftStatus('Providers & Integrations')).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0]?.[0].settings).toEqual([
      { key: 'ollamaProviderSettings', value: { endpoint: 'http://new.local:11434' } },
    ]);
    expect(api.updateSetting).not.toHaveBeenCalled();
    expect(api.testOllamaConnection).not.toHaveBeenCalled();
    expect(api.listOllamaModels).not.toHaveBeenCalled();
  });

  it('uses the section save bar for General and atomic Analysis settings', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=general');

    await user.click(await screen.findByLabelText('Quality / storage preference'));
    await user.click(await screen.findByRole('option', { name: 'Maximum quality' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('General')).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(sidebarDraftStatus('General')).toBe(false));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings).toEqual([
      { key: 'mvforgePreferences', value: expect.objectContaining({ qualityGoal: 'maximum_quality' }) },
    ]);

    await user.click(screen.getByRole('button', { name: 'Analysis' }));
    expect(await screen.findByRole('heading', { name: 'Analysis' })).toBeTruthy();
    await user.click(screen.getByLabelText('Analysis mode'));
    await user.click(await screen.findByRole('option', { name: 'Thorough' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('Analysis')).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(sidebarDraftStatus('Analysis')).toBe(false));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[1][0].settings).toEqual([
      { key: 'analysisPolicy', value: expect.objectContaining({ mode: 'thorough' }) },
      { key: 'profileSampleEstimatePolicy', value: expect.objectContaining({ hardwareWindows: 5 }) },
    ]);
  }, 15000);

  it('saves working hours with Pipeline Automation in the Pipeline section batch', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=pipeline');

    const timezone = await screen.findByLabelText('Timezone');
    await user.clear(timezone);
    await user.type(timezone, 'UTC');
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual(['pipelineAutomation', 'workingHours']);
    expect(batch[1].value).toEqual(expect.objectContaining({ timezone: 'UTC' }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('saves Runtime profile and DirectPlay policy through the shared section batch', async () => {
    const user = userEvent.setup();
    vi.mocked(api.runtimeProfiles).mockResolvedValueOnce({
      profiles: [{
        key: 'desktop_safe',
        name: 'Desktop Safe',
        description: 'Safe test profile',
        official: true,
        values: {
          maxRunningJobs: 2,
          maxVideoJobs: 2,
          maxSoftwareX265Jobs: 1,
          maxHardwareEncodeJobs: 2,
          maxAudioJobs: 2,
          maxLabJobs: 1,
          minFreeRamGb: 2,
          minFreeWorkGb: 10,
          minFreeLibraryGb: 5,
          maxWorkspaceGb: 50,
          allowDirectMode: false,
          pauseWhenOnBattery: true,
          preventSleepDuringJobs: true,
        },
      }],
      effective: {},
    } as never);
    renderSettings('/settings?section=runtime');

    await user.click(await screen.findByLabelText('Detection mode'));
    await user.click(await screen.findByRole('option', { name: 'Manual' }));
    const minimumScore = await screen.findByLabelText('Minimum client score');
    fireEvent.change(minimumScore, { target: { value: '75' } });
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual(['workers', 'runtimePolicy', 'directPlay', 'aiResourceProtectionSettings']);
    expect(batch[1].value).toEqual(expect.objectContaining({ mode: 'manual', preferredProfile: 'desktop_safe' }));
    expect(batch[2].value).toEqual(expect.objectContaining({ minimumScore: 75 }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('saves Assets storage roles and workspace policy in one scoped batch', async () => {
    const user = userEvent.setup();
    vi.mocked(api.settings).mockResolvedValueOnce([
      { key: 'workers', value: { defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8, delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false } },
      { key: 'storageRoles', value: { originals_archive: { path: '/archive-preserved' } } },
    ] as never);
    renderSettings('/settings?section=assets');

    fireEvent.change(await screen.findByLabelText('work'), { target: { value: '/work-reviewed' } });
    await user.click(await screen.findByLabelText('Preferred mode'));
    await user.click(await screen.findByRole('option', { name: 'Read raw directly' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual([
      'paths', 'assetInventory', 'assetTypes', 'assetCategories', 'storageRoles', 'workspace',
    ]);
    expect(batch[4].value).toEqual(expect.objectContaining({
      work: { path: '/work-reviewed' },
      originals_archive: { path: '/archive-preserved' },
    }));
    expect(batch[5].value).toEqual(expect.objectContaining({ preferredMode: 'direct_mode' }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('keeps Asset Defaults dirty after a failed save and lets the user discard it', async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateSettingsBatch).mockRejectedValueOnce(new Error('offline'));
    renderSettings('/settings?section=asset-defaults');

    const videoHandling = await screen.findByLabelText('Video handling');
    await user.click(videoHandling);
    await user.click(await screen.findByRole('option', { name: 'Encode as HEVC' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('Asset Defaults')).toBe(true);
    expect(screen.queryByRole('button', { name: 'Save Asset Defaults' })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('Save failed'));
    expect(sidebarDraftStatus('Asset Defaults')).toBe(true);
    expect(screen.getByLabelText('Video handling').textContent).toContain('Encode as HEVC');
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings.map((setting) => setting.key)).toEqual(['assetDefaults']);
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(screen.getByLabelText('Video handling').textContent).toContain('Copy'));
    await waitFor(() => expect(sidebarDraftStatus('Asset Defaults')).toBe(false));
    expect(screen.getByRole('status').textContent).not.toBe('Unsaved changes');
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('saves Validation warning advancement through its section draft', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=validation');

    await user.click(await screen.findByRole('checkbox', { name: 'Allow PASS_WITH_WARNINGS for automatic advancement' }));
    await user.click(await screen.findByRole('button', { name: 'Confirm warning opt-in' }));
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Confirm automatic warning advancement' })).toBeNull());
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('Validation & Publishing')).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(sidebarDraftStatus('Validation & Publishing')).toBe(false));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual(['validation']);
    expect(batch[0].value).toEqual(expect.objectContaining({ autoAdvanceOutcomes: ['PASS', 'PASS_WITH_WARNINGS'] }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('saves housekeeping policy with cancellation policy in the Operations section batch', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=operations');

    const interval = await screen.findByLabelText('Interval (hours)');
    fireEvent.change(interval, { target: { value: '48' } });
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(screen.getByRole('button', { name: 'Preview cleanup' }).hasAttribute('disabled')).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual(['cancellationPolicy', 'housekeeping']);
    expect(batch[1].value).toEqual(expect.objectContaining({ intervalHours: 48 }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('saves Test Encode policy through the section draft', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=test-encode');

    const duration = await screen.findByLabelText('Default duration (seconds)');
    fireEvent.change(duration, { target: { value: '30' } });
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    expect(sidebarDraftStatus('Test Encode')).toBe(true);
    expect(screen.queryByRole('button', { name: 'Save Test Encode settings' })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(sidebarDraftStatus('Test Encode')).toBe(false));
    const batch = vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings;
    expect(batch.map((setting) => setting.key)).toEqual(['testEncodePolicy']);
    expect(batch[0].value).toEqual(expect.objectContaining({ defaultDurationSeconds: 30 }));
    expect(api.updateSetting).not.toHaveBeenCalled();
  }, 15000);

  it('records Settings feedback from the authoritative response while refetch reconciles in the background', async () => {
    const user = userEvent.setup();
    const settingsRefresh = deferred<Awaited<ReturnType<typeof api.settings>>>();
    const saveResponse = deferred<Awaited<ReturnType<typeof api.updateSettingsBatch>>>();
    let holdSettingsRefresh = false;
    vi.mocked(api.settings).mockImplementation(() => holdSettingsRefresh
      ? settingsRefresh.promise
      : Promise.resolve([{ key: 'testEncodePolicy', value: { defaultDurationSeconds: 10 } }] as never));
    vi.mocked(api.updateSettingsBatch).mockReturnValue(saveResponse.promise);
    const { queryClient } = renderSettings('/settings?section=test-encode');

    fireEvent.change(await screen.findByLabelText('Default duration (seconds)'), { target: { value: '30' } });
    const settingsCallsBeforeSave = vi.mocked(api.settings).mock.calls.length;
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('status').textContent).toBe('Saving…');

    // Deterministic synthetic timing: 120 ms for the write and 240 ms for the
    // active settings read-model refresh. These are virtual fixture durations.
    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    holdSettingsRefresh = true;
    await act(async () => {
      syntheticNowMs += 120;
      saveResponse.resolve([{ key: 'testEncodePolicy', value: { defaultDurationSeconds: 30 } }] as never);
      await Promise.resolve();
    });
    await waitFor(() => expect(vi.mocked(api.settings).mock.calls.length).toBeGreaterThan(settingsCallsBeforeSave));
    const responseAtMs = syntheticNowMs;
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Saved'));
    expect(queryClient.isFetching({ queryKey: ['settings'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;

    await act(async () => {
      syntheticNowMs += 240;
      settingsRefresh.resolve([{ key: 'testEncodePolicy', value: { defaultDurationSeconds: 30 } }] as never);
      await settingsRefresh.promise;
    });
    await waitFor(() => expect(queryClient.getQueryData(['settings'])).toEqual([{ key: 'testEncodePolicy', value: { defaultDurationSeconds: 30 } }]));
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
    expect(screen.getByRole('status').textContent).toBe('Saved');
  });

  it('keeps concurrent section saves when authoritative responses arrive out of order', async () => {
    const user = userEvent.setup();
    const initialSettings = [
      { key: 'workers', value: {
        defaultWorkerName: 'local', autoWorkerEnabled: true, maxConcurrentJobs: 2, maxJobsPerBatch: 8,
        delaySecondsBetweenJobs: 0, batchCooldownSeconds: 0, dryRunOnly: false,
      } },
      { key: 'mvforgePreferences', value: { qualityGoal: 'balanced_quality' } },
      { key: 'analysisPolicy', value: { mode: 'balanced' } },
      { key: 'profileSampleEstimatePolicy', value: { hardwareWindows: 3 } },
    ];
    const finalSettings = initialSettings.map((setting) => setting.key === 'mvforgePreferences'
      ? { ...setting, value: { qualityGoal: 'maximum_quality' } }
      : setting.key === 'analysisPolicy'
        ? { ...setting, value: { mode: 'thorough' } }
        : setting.key === 'profileSampleEstimatePolicy'
          ? { ...setting, value: { hardwareWindows: 5 } }
          : setting);
    const generalResponse = deferred<Awaited<ReturnType<typeof api.updateSettingsBatch>>>();
    const analysisResponse = deferred<Awaited<ReturnType<typeof api.updateSettingsBatch>>>();
    const settingsRefresh = deferred<Awaited<ReturnType<typeof api.settings>>>();
    let holdSettingsRefresh = false;
    const settingsRead = vi.mocked(api.settings).mockImplementation(() => holdSettingsRefresh
      ? settingsRefresh.promise
      : Promise.resolve(initialSettings as never));
    const saveSettings = vi.mocked(api.updateSettingsBatch)
      .mockImplementationOnce(() => generalResponse.promise)
      .mockImplementationOnce(() => analysisResponse.promise);
    const { queryClient } = renderSettings('/settings?section=general', true);

    await user.click(await screen.findByLabelText('Quality / storage preference'));
    await user.click(await screen.findByRole('option', { name: 'Maximum quality' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole('button', { name: 'Analysis' }));
    await user.click(await screen.findByLabelText('Analysis mode'));
    await user.click(await screen.findByRole('option', { name: 'Thorough' }));
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalledTimes(2));

    // Both writes remain in flight; resolve the later Analysis response first.
    holdSettingsRefresh = true;
    await act(async () => {
      analysisResponse.resolve([
        { key: 'analysisPolicy', value: { mode: 'thorough' } },
        { key: 'profileSampleEstimatePolicy', value: { hardwareWindows: 5 } },
      ] as never);
      await Promise.resolve();
    });
    await waitFor(() => expect(settingsRead.mock.calls.length).toBeGreaterThan(1));
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Saved'));
    expect(queryClient.isFetching({ queryKey: ['settings'] })).toBe(1);
    expect(queryClient.getQueryData(['settings'])).toEqual(expect.arrayContaining([
      { key: 'analysisPolicy', value: { mode: 'thorough' } },
      { key: 'profileSampleEstimatePolicy', value: { hardwareWindows: 5 } },
      { key: 'mvforgePreferences', value: { qualityGoal: 'balanced_quality' } },
    ]));

    await act(async () => {
      generalResponse.resolve([{ key: 'mvforgePreferences', value: { qualityGoal: 'maximum_quality' } }] as never);
      await Promise.resolve();
    });
    await waitFor(() => expect(queryClient.getQueryData(['settings'])).toEqual(expect.arrayContaining([
      { key: 'analysisPolicy', value: { mode: 'thorough' } },
      { key: 'profileSampleEstimatePolicy', value: { hardwareWindows: 5 } },
      { key: 'mvforgePreferences', value: { qualityGoal: 'maximum_quality' } },
    ])));

    await act(async () => {
      settingsRefresh.resolve(finalSettings as never);
      await settingsRefresh.promise;
    });
    await waitFor(() => expect(queryClient.getQueryData(['settings'])).toEqual(finalSettings));
    expect(queryClient.isFetching({ queryKey: ['settings'] })).toBe(0);

    await user.click(screen.getByRole('button', { name: 'Test route General' }));
    expect((await screen.findByLabelText('Quality / storage preference')).textContent).toContain('Maximum quality');
    await user.click(screen.getByRole('button', { name: 'Analysis' }));
    expect((await screen.findByLabelText('Analysis mode')).textContent).toContain('Thorough');
  }, 15000);

  it('does not show Saved when the Settings write fails', async () => {
    const user = userEvent.setup();
    vi.mocked(api.updateSettingsBatch).mockRejectedValueOnce(new Error('synthetic HTTP failure'));
    renderSettings('/settings?section=test-encode');

    fireEvent.change(await screen.findByLabelText('Default duration (seconds)'), { target: { value: '30' } });
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    expect((await screen.findByRole('status')).textContent).toContain('Save failed');
    expect((screen.getByLabelText('Default duration (seconds)') as HTMLInputElement).value).toBe('30');
    expect(sidebarDraftStatus('Test Encode')).toBe(true);
  });
});
