// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import { RemoveTracksDialog } from './RemoveTracksDialog';

vi.mock('../api/client', () => ({ api: {
  trackMaintenanceInventory: vi.fn(), startTrackRemoval: vi.fn(), maintenanceOperation: vi.fn(),
} }));

const inventory = {
  path: '/library/movie.mkv', fingerprint: 'sha256:before', chapters: 3,
  streams: [
    { index: 0, type: 'video', codec: 'hevc', default: true, forced: false, attachedPic: false, stillImage: false },
    { index: 1, type: 'audio', codec: 'truehd', language: 'jpn', default: true, forced: false, attachedPic: false, stillImage: false },
    { index: 2, type: 'subtitle', codec: 'hdmv_pgs_subtitle', language: 'eng', default: false, forced: false, attachedPic: false, stillImage: false },
  ],
};

function renderDialog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const rendered = render(<QueryClientProvider client={client}><RemoveTracksDialog open path={inventory.path} onClose={() => undefined} /></QueryClientProvider>);
  return { ...rendered, client };
}

describe('RemoveTracksDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  it('requires a track selection and irreversible-action confirmation', async () => {
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue(inventory);
    const user = userEvent.setup();
    renderDialog();
    const audio = await screen.findByRole('checkbox', { name: /#1 · audio/i });
    const remove = screen.getByRole('button', { name: /remove 0 tracks/i });
    expect((remove as HTMLButtonElement).disabled).toBe(true);
    await user.click(audio);
    expect((screen.getByRole('button', { name: /remove 1 track/i }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByRole('checkbox', { name: /cannot be recovered/i }));
    expect((screen.getByRole('button', { name: /remove 1 track/i }) as HTMLButtonElement).disabled).toBe(false);
  });

  it('blocks selecting every playable video stream', async () => {
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue(inventory);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('checkbox', { name: /#0 · video/i }));
    expect(screen.getByText(/at least one playable video stream must remain/i)).toBeTruthy();
  });

  it('sends absolute stream indexes and refreshes assets after completion', async () => {
    vi.mocked(api.trackMaintenanceInventory).mockResolvedValue(inventory);
    vi.mocked(api.startTrackRemoval).mockResolvedValue({
      id: 'maintenance-1', operationType: 'remove_tracks', assetPath: inventory.path,
      status: 'queued', phase: 'queued', progress: 0,
    });
    vi.mocked(api.maintenanceOperation).mockResolvedValue({
      id: 'maintenance-1', operationType: 'remove_tracks', assetPath: inventory.path,
      status: 'completed', phase: 'completed', progress: 100,
    });
    const user = userEvent.setup();
    const { client } = renderDialog();
    const invalidate = vi.spyOn(client, 'invalidateQueries');

    await user.click(await screen.findByRole('checkbox', { name: /#2 · subtitle/i }));
    await user.click(screen.getByRole('checkbox', { name: /cannot be recovered/i }));
    await user.click(screen.getByRole('button', { name: /remove 1 track/i }));

    await waitFor(() => expect(api.startTrackRemoval).toHaveBeenCalledWith({
      path: inventory.path,
      streamIndexes: [2],
      expectedFingerprint: inventory.fingerprint,
      confirmed: true,
    }));
    await act(async () => {
      await new Promise((resolve) => window.setTimeout(resolve, 1100));
    });
    await waitFor(() => {
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ['assets'] });
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ['trackMaintenanceInventory', inventory.path] });
    });
  });
});
