// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AppSetting, Profile } from '../api/types';
import { AssetDefaultsCard } from './AssetDefaultsCard';

afterEach(cleanup);

const settings = [
  { key: 'audioEnhancementProfiles', value: { profiles: [] } },
  { key: 'trackProfiles', value: { profiles: [] } },
] as unknown as AppSetting[];

const profiles = [{ id: 7, name: 'Video Profile', disabled: false }] as unknown as Profile[];

const value = {
  schemaVersion: 1,
  video: { source: 'custom', custom: { videoCodec: 'x265' } },
  audio: { source: 'custom', custom: {} },
  tracks: { source: 'custom', custom: {} },
};

describe('AssetDefaultsCard', () => {
  it('renders separate domain sections and saves a profile reference without custom fields', async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={value} settings={settings} profiles={profiles} saving={false} onSave={onSave} />);

    expect(screen.getByText('Video General')).toBeTruthy();
    expect(screen.getByText('Audio General')).toBeTruthy();
    expect(screen.getByText('Tracks General')).toBeTruthy();

    await user.click(screen.getAllByRole('combobox')[0]);
    await user.click(screen.getByRole('option', { name: 'Existing profile' }));
    await user.click(screen.getByRole('combobox', { name: 'Profile reference' }));
    await user.click(screen.getByRole('option', { name: 'Video Profile' }));
    await user.click(screen.getByRole('button', { name: 'Save Asset Defaults' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      video: expect.objectContaining({ source: 'profile', profileId: 7, custom: {} }),
    }));
  });

  it('offers only encoders reported usable by the runtime', async () => {
    const user = userEvent.setup();
    render(
      <AssetDefaultsCard
        value={value}
        settings={settings}
        profiles={profiles}
        runtimeEncoders={{
          hevc_qsv: { listed: true, usable: true },
          libx265: { listed: true, usable: true },
          h264_nvenc: { listed: true, usable: false, reason: 'probe failed' },
        }}
        saving={false}
        onSave={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('combobox', { name: 'Preferred encoder' }));

    expect(screen.getByRole('option', { name: 'hevc_qsv' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'libx265' })).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'h264_nvenc' })).toBeNull();
  });

  it('disables compatible track fields without clearing dormant values', async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={{ ...value, audio: { source: 'custom', custom: {
      addCompatibleTrack: false,
      compatibleTrackQuality: 384,
      compatibleTrackChannels: 8,
      compatibleTrackSource: 'default',
      preferredDefaultLanguage: 'jpn',
    } } }} settings={settings} profiles={profiles} saving={false} onSave={onSave} />);

    expect((screen.getByLabelText('Quality / bitrate') as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByRole('combobox', { name: 'Channels' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('combobox', { name: 'Source stream' }).getAttribute('aria-disabled')).toBe('true');
    expect((screen.getByLabelText('Preferred default language') as HTMLInputElement).disabled).toBe(false);

    await user.click(screen.getByRole('button', { name: 'Save Asset Defaults' }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      audio: expect.objectContaining({
        custom: expect.objectContaining({
          addCompatibleTrack: false,
          compatibleTrackQuality: 384,
          compatibleTrackChannels: 8,
          compatibleTrackSource: 'default',
        }),
      }),
    }));
  });

  it('enables compatible track fields when Add compatible track is selected', async () => {
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={value} settings={settings} profiles={profiles} saving={false} onSave={vi.fn()} />);

    await user.click(screen.getByRole('switch', { name: 'Add compatible track' }));
    expect((screen.getByLabelText('Quality / bitrate') as HTMLInputElement).disabled).toBe(false);
    expect(screen.getByRole('combobox', { name: 'Channels' }).getAttribute('aria-disabled')).not.toBe('true');
    expect(screen.getByRole('combobox', { name: 'Source stream' }).getAttribute('aria-disabled')).not.toBe('true');
  });

  it('adds canonical preferred subtitle rules with mixed actions and preserves them on save', async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={value} settings={settings} profiles={profiles} saving={false} onSave={onSave} />);

    await user.click(screen.getByRole('button', { name: '+ Add language' }));
    await user.click(screen.getByRole('combobox', { name: 'Language 1' }));
    await user.click(screen.getByRole('option', { name: 'Spanish · spa' }));
    await user.click(screen.getByRole('combobox', { name: 'Action' }));
    await user.click(screen.getByRole('option', { name: 'Keep and extract' }));
    await user.click(screen.getByRole('button', { name: '+ Add language' }));
    await user.click(screen.getByRole('combobox', { name: 'Language 2' }));
    await user.click(screen.getByRole('option', { name: 'English · eng' }));
    await user.click(screen.getAllByRole('combobox', { name: 'Action' })[1]);
    await user.click(screen.getByRole('option', { name: 'Extract' }));
    await user.click(screen.getByRole('button', { name: 'Save Asset Defaults' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      tracks: expect.objectContaining({
        custom: expect.objectContaining({
          preferredSubtitleRules: [
            { language: 'spa', action: 'keep_and_extract' },
            { language: 'eng', action: 'extract' },
          ],
        }),
      }),
    }));
  });

  it('uses the canonical selector for the optional default subtitle language', async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={{ ...value, tracks: { source: 'custom', custom: { defaultSubtitleLanguage: 'es' } } }} settings={settings} profiles={profiles} saving={false} onSave={onSave} />);

    const selector = screen.getByRole('combobox', { name: 'Default subtitle language' });
    const selectorInput = selector.parentElement?.querySelector('input') as HTMLInputElement;
    expect(selectorInput.value).toBe('spa');
    await user.click(selector);
    await user.click(screen.getByRole('option', { name: 'Japanese · jpn' }));
    await user.click(screen.getByRole('button', { name: 'Save Asset Defaults' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      tracks: expect.objectContaining({
        custom: expect.objectContaining({ defaultSubtitleLanguage: 'jpn' }),
      }),
    }));
  });

  it('allows an empty default subtitle language', async () => {
    const user = userEvent.setup();
    render(<AssetDefaultsCard value={value} settings={settings} profiles={profiles} saving={false} onSave={vi.fn()} />);
    const selector = screen.getByRole('combobox', { name: 'Default subtitle language' });
    const selectorInput = selector.parentElement?.querySelector('input') as HTMLInputElement;
    expect(selectorInput.value).toBe('');
    await user.click(selector);
    expect(screen.getByRole('option', { name: 'None' })).toBeTruthy();
  });
});
