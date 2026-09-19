// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SubtitleCompatibilityCard, subtitleSRTFallbackValue } from './SettingsPage';

afterEach(cleanup);

describe('subtitle SRT fallback settings', () => {
  it('defaults missing settings to off and reads a persisted enabled value', () => {
    expect(subtitleSRTFallbackValue(undefined).generateSRTFallbackForASS).toBe(false);
    expect(subtitleSRTFallbackValue({ generateSRTFallbackForASS: true }).generateSRTFallbackForASS).toBe(true);
  });

  it('toggles and saves the compatibility setting', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();

    render(
      <SubtitleCompatibilityCard
        value={subtitleSRTFallbackValue(undefined)}
        saving={false}
        onSave={onSave}
      />,
    );

    const toggle = screen.getByRole('switch', { name: 'Generate SRT fallback for ASS compatibility' });
    expect((toggle as HTMLInputElement).checked).toBe(false);
    await user.click(toggle);
    await user.click(screen.getByRole('button', { name: 'Save Subtitle Settings' }));

    expect(onSave).toHaveBeenCalledWith({ generateSRTFallbackForASS: true });
  });

  it('renders a persisted enabled setting as on', () => {
    render(
      <SubtitleCompatibilityCard
        value={subtitleSRTFallbackValue({ generateSRTFallbackForASS: true })}
        saving={false}
        onSave={vi.fn()}
      />,
    );

    expect((screen.getByRole('switch', { name: 'Generate SRT fallback for ASS compatibility' }) as HTMLInputElement).checked).toBe(true);
  });
});
