// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SubtitleStylingCard, subtitleStylingValue } from './SettingsPage';

afterEach(cleanup);

describe('subtitle styling settings', () => {
  it('defaults the master gate off and keeps the generated-style control independent', () => {
    const value = subtitleStylingValue(undefined);
    expect(value).toMatchObject({
      version: 1,
      enabled: false,
      applyToGeneratedASS: true,
      preserveSourceStyles: true,
      overrideExistingASSStyles: false,
    });
  });

  it('saves explicit enablement, native preservation, and custom preset identity', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<SubtitleStylingCard value={subtitleStylingValue(undefined)} saving={false} onSave={onSave} />);

    await user.click(screen.getByRole('switch', { name: 'Enable position-aware styling' }));
    await user.click(screen.getByRole('button', { name: 'Add custom style' }));
    await user.click(screen.getByRole('button', { name: 'Save Subtitle Styling' }));

    const saved = onSave.mock.calls[0]?.[0];
    expect(saved).toMatchObject({ enabled: true, applyToGeneratedASS: true, preserveSourceStyles: true, overrideExistingASSStyles: false });
    expect(saved.customPresets).toHaveLength(1);
    expect(saved.customPresets[0]).toMatchObject({ provenance: 'custom' });
    expect(saved.customPresets[0].id).toMatch(/^custom_/);
  });
});
