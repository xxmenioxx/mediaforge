// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/client')>();
  return {
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
    },
  };
});

import { api } from '../api/client';
import { SettingsPage } from './SettingsPage';

function CurrentSettingsSearch() {
  const location = useLocation();
  return <div data-testid="settings-search">{location.search}</div>;
}

function renderSettings(initialPath: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <QueryClientProvider client={queryClient}>
        <SettingsPage />
        <CurrentSettingsSearch />
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  window.sessionStorage.clear();
});

describe('Settings domain navigation and drafts', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    vi.mocked(api.updateSettingsBatch).mockClear();
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
    vi.mocked(api.schedulerRecovery).mockResolvedValue({} as never);
    vi.mocked(api.updateSettingsBatch).mockResolvedValue([] as never);
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
    expect(batch.settings).toEqual([
      { key: 'workers', value: expect.objectContaining({ maxConcurrentJobs: 3 }) },
    ]);
    await waitFor(() => expect(screen.getByRole('status').textContent).toBe('Saved'));
  }, 15000);

  it('uses the section save bar for General and atomic Analysis settings', async () => {
    const user = userEvent.setup();
    renderSettings('/settings?section=general');

    await user.click(await screen.findByLabelText('Quality / storage preference'));
    await user.click(await screen.findByRole('option', { name: 'Maximum quality' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(1));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[0][0].settings).toEqual([
      { key: 'mvforgePreferences', value: expect.objectContaining({ qualityGoal: 'maximum_quality' }) },
    ]);

    await user.click(screen.getByRole('button', { name: 'Analysis' }));
    expect(await screen.findByRole('heading', { name: 'Analysis' })).toBeTruthy();
    await user.click(screen.getByLabelText('Analysis mode'));
    await user.click(await screen.findByRole('option', { name: 'Thorough' }));
    expect(screen.getByRole('status').textContent).toBe('Unsaved changes');
    await user.click(screen.getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(api.updateSettingsBatch).toHaveBeenCalledTimes(2));
    expect(vi.mocked(api.updateSettingsBatch).mock.calls[1][0].settings).toEqual([
      { key: 'analysisPolicy', value: expect.objectContaining({ mode: 'thorough' }) },
      { key: 'profileSampleEstimatePolicy', value: expect.objectContaining({ hardwareWindows: 5 }) },
    ]);
  }, 15000);
});
