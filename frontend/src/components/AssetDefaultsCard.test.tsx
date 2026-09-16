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
});
