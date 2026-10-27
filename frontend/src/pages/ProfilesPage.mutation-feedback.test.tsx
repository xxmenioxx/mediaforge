// @vitest-environment jsdom

import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import { ProfilesPage } from './ProfilesPage';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => { resolve = resolvePromise; });
  return { promise, resolve };
}

function profile(id: number, name: string) {
  return {
    id, name, scope: 'asset', description: 'Synthetic profile', container: 'mkv', videoCodec: 'x264',
    codecFamily: 'h264', encoderPolicy: 'locked', preferredEncoder: 'libx264', allowedEncoders: ['libx264'],
    fallbackPolicy: 'wait', bitDepth: 8, pixelFormat: 'yuv420p', qualityStrategy: 'crf', profileVersion: 1,
    audioCodec: 'copy', qualityMode: 'crf', qualityValue: 22, preserveHdr: false, preserveSubtitles: true,
    preserveChapters: true, workerConfig: { encoder: 'ffmpeg', preset: 'medium' }, disabled: false,
    createdAt: '', updatedAt: '',
  } as never;
}

function mockProfilesPage() {
  vi.spyOn(api, 'profileTemplates').mockResolvedValue([] as never);
  vi.spyOn(api, 'runtimeState').mockResolvedValue({} as never);
  vi.spyOn(api, 'workerNodes').mockResolvedValue([] as never);
  vi.spyOn(api, 'settings').mockResolvedValue([] as never);
  vi.spyOn(api, 'recommendEncoderQuality').mockReturnValue(new Promise(() => undefined) as never);
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter><ProfilesPage /></MemoryRouter>
    </QueryClientProvider>,
  );
  return queryClient;
}

describe('Profiles mutation feedback timing', () => {
  it('records create feedback from the server response while refetch reconciles in the background', async () => {
    const profileRefresh = deferred<Awaited<ReturnType<typeof api.profilesAdmin>>>();
    const createResponse = deferred<Awaited<ReturnType<typeof api.createProfile>>>();
    let holdProfileRefresh = false;
    const profilesAdmin = vi.spyOn(api, 'profilesAdmin').mockImplementation(() => holdProfileRefresh
      ? profileRefresh.promise
      : Promise.resolve([] as never));
    const createProfile = vi.spyOn(api, 'createProfile').mockReturnValue(createResponse.promise);
    const queryClient = mockProfilesPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole('button', { name: 'Add Profile' }));
    await user.type(screen.getByRole('textbox', { name: 'Profile name' }), 'Synthetic profile');
    await user.click(screen.getByRole('button', { name: 'Create Profile' }));
    await waitFor(() => expect(createProfile).toHaveBeenCalledTimes(1));
    const profilesCallsBeforeWriteResponse = profilesAdmin.mock.calls.length;

    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    const createdProfile = profile(21, 'Synthetic profile');
    holdProfileRefresh = true;
    await act(async () => {
      syntheticNowMs += 120;
      createResponse.resolve(createdProfile);
      await Promise.resolve();
    });
    await waitFor(() => expect(profilesAdmin.mock.calls.length).toBeGreaterThan(profilesCallsBeforeWriteResponse));

    const responseAtMs = syntheticNowMs;
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.getByRole('button', { name: 'Add Profile' })).toBeTruthy();
    expect(queryClient.isFetching({ queryKey: ['profiles', 'admin'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;

    await act(async () => {
      syntheticNowMs += 240;
      profileRefresh.resolve([createdProfile]);
      await profileRefresh.promise;
    });
    await waitFor(() => expect(queryClient.getQueryData(['profiles', 'admin'])).toEqual([createdProfile]));
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
  }, 20000);

  it('records update feedback from the server response while refetch reconciles in the background', async () => {
    const existingProfile = profile(22, 'Before update');
    const updatedProfile = profile(22, 'After update');
    const profileRefresh = deferred<Awaited<ReturnType<typeof api.profilesAdmin>>>();
    let holdProfileRefresh = false;
    const profilesAdmin = vi.spyOn(api, 'profilesAdmin').mockImplementation(() => holdProfileRefresh
      ? profileRefresh.promise
      : Promise.resolve([existingProfile] as never));
    const updateResponse = deferred<Awaited<ReturnType<typeof api.updateProfile>>>();
    const updateProfile = vi.spyOn(api, 'updateProfile').mockReturnValue(updateResponse.promise);
    const queryClient = mockProfilesPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole('button', { name: /^Edit$/ }));
    const nameInput = screen.getByRole('textbox', { name: 'Profile name' });
    await user.clear(nameInput);
    await user.type(nameInput, 'After update');
    await user.click(screen.getByRole('button', { name: 'Save Changes' }));
    await waitFor(() => expect(updateProfile).toHaveBeenCalledTimes(1));
    const profilesCallsBeforeWriteResponse = profilesAdmin.mock.calls.length;

    let syntheticNowMs = 0;
    const requestStartedAtMs = syntheticNowMs;
    holdProfileRefresh = true;
    await act(async () => {
      syntheticNowMs += 120;
      updateResponse.resolve(updatedProfile);
      await Promise.resolve();
    });
    await waitFor(() => expect(profilesAdmin.mock.calls.length).toBeGreaterThan(profilesCallsBeforeWriteResponse));
    const responseAtMs = syntheticNowMs;
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(queryClient.isFetching({ queryKey: ['profiles', 'admin'] })).toBe(1);
    const feedbackAtMs = syntheticNowMs;

    await act(async () => {
      syntheticNowMs += 240;
      profileRefresh.resolve([updatedProfile]);
      await profileRefresh.promise;
    });
    await waitFor(() => expect(queryClient.getQueryData(['profiles', 'admin'])).toEqual([updatedProfile]));
    expect(responseAtMs - requestStartedAtMs).toBe(120);
    expect(feedbackAtMs - responseAtMs).toBe(0);
    expect(feedbackAtMs - requestStartedAtMs).toBe(120);
  }, 20000);

  it('keeps a create form and error visible when the profile request fails', async () => {
    vi.spyOn(api, 'profilesAdmin').mockResolvedValue([] as never);
    vi.spyOn(api, 'createProfile').mockRejectedValue(new Error('synthetic HTTP failure'));
    mockProfilesPage();
    const user = userEvent.setup();

    await user.click(await screen.findByRole('button', { name: 'Add Profile' }));
    const nameInput = screen.getByRole('textbox', { name: 'Profile name' });
    await user.type(nameInput, 'Synthetic profile');
    await user.click(screen.getByRole('button', { name: 'Create Profile' }));

    expect(await screen.findByText('Profile could not be created. Profile names must be unique.')).toBeTruthy();
    expect((screen.getByRole('textbox', { name: 'Profile name' }) as HTMLInputElement).value).toBe('Synthetic profile');
    expect(screen.getByRole('dialog')).toBeTruthy();
  }, 20000);
});
