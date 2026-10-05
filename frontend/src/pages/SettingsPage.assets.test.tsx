// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
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
    },
  };
});

import { api } from '../api/client';
import { SettingsPage } from './SettingsPage';

afterEach(cleanup);

describe('Assets Settings historical analysis decisions', () => {
  beforeEach(() => {
    vi.mocked(api.settings).mockResolvedValue([{ key: 'analysisRecords', value: { records: [{ id: 'legacy-1', assetName: 'Episode 01.mkv', decision: 'keep', assetPath: '/raw/Episode 01.mkv', createdAt: '2024-01-01' }] } }] as never);
    vi.mocked(api.libraries).mockResolvedValue([]);
    vi.mocked(api.runtimeState).mockResolvedValue({} as never);
    vi.mocked(api.workerNodes).mockResolvedValue([]);
    vi.mocked(api.runtimeProfiles).mockResolvedValue({} as never);
    vi.mocked(api.profiles).mockResolvedValue([]);
    vi.mocked(api.schedulerRecovery).mockResolvedValue({} as never);
  });

  it('keeps preserved Legacy Analysis Decisions reachable under Assets Settings', async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <MemoryRouter initialEntries={['/settings?section=assets']}>
        <QueryClientProvider client={queryClient}>
          <SettingsPage />
        </QueryClientProvider>
      </MemoryRouter>,
    );

    const settingsSection = await screen.findByRole('region', { name: 'Assets Settings' });
    const legacyControl = await within(settingsSection).findByRole('button', { name: 'Legacy Analysis Decisions (1)' });
    await user.click(legacyControl);
    expect(within(settingsSection).getByText('Episode 01.mkv · keep')).toBeTruthy();
    expect(within(settingsSection).getByText(/Legacy record · \/raw\/Episode 01.mkv/)).toBeTruthy();
  });
});
