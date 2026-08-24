// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import { TrackMaintenancePanel } from './TrackMaintenancePanel';

vi.mock('../api/client', () => ({ api: {
  trackMaintenanceInventory: vi.fn(), editTrack: vi.fn(), addAACTrack: vi.fn(), startTrackRemoval: vi.fn(), maintenanceOperation: vi.fn(),
} }));

const inventory = {
  path: '/library/movie.mkv', fingerprint: 'before', chapters: 2, maintenanceAllowed: true,
  streams: [
    { index: 0, type: 'video', codec: 'hevc', default: true, forced: false, attachedPic: false, stillImage: false },
    { index: 1, type: 'audio', codec: 'truehd', language: 'jpn', title: 'Main', channels: 6, layout: '5.1', default: true, forced: false, attachedPic: false, stillImage: false },
  ],
};

function renderPanel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><TrackMaintenancePanel path={inventory.path} /></QueryClientProvider>);
}

describe('TrackMaintenancePanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('opens AAC copy controls with explicit quality choices', async () => {
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue(inventory);
    const user = userEvent.setup();
    renderPanel();
    await user.click(await screen.findByRole('button', { name: /create an additional AAC audio track/i }));
    expect(screen.getByRole('dialog', { name: /create AAC copy/i })).toBeTruthy();
    expect(screen.getByRole('combobox', { name: /AAC bitrate/i })).toBeTruthy();
    expect(screen.getByText(/source track and every other stream are preserved/i)).toBeTruthy();
  });

  it('shows protected originals but disables maintenance actions', async () => {
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue({ ...inventory, maintenanceAllowed: false, maintenanceDisabledReason: 'Original Raw and Archive assets are protected.' });
    renderPanel();
    expect(await screen.findByText(/Original Raw and Archive assets are protected/i)).toBeTruthy();
    expect((screen.getByRole('button', { name: /create an additional AAC audio track/i }) as HTMLButtonElement).disabled).toBe(true);
  });
});
