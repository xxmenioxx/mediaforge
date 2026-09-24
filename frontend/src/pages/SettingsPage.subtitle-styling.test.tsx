// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SubtitleStylingCard, subtitleHexError, subtitleNormalizeHexColor, subtitleStylingValue } from './SettingsPage';

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

  it('normalizes HEX input, displays invalid errors, and bounds saved/recent colors', async () => {
    expect(subtitleNormalizeHexColor('#abc')).toBe('#AABBCC');
    expect(subtitleNormalizeHexColor('#aBc123')).toBe('#ABC123');
    expect(subtitleHexError('#GGGGGG')).toContain('valid HEX');
    const saved = Array.from({ length: 40 }, (_, index) => `#${index.toString(16).padStart(6, '0')}`);
    const recent = [...saved, '#FFFFFF', '#ffffff'];
    const bounded = subtitleStylingValue({ subtitleStyling: { savedColors: saved, recentColors: recent } });
    expect(bounded.savedColors).toHaveLength(32);
    expect(bounded.recentColors).toHaveLength(12);

    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<SubtitleStylingCard value={subtitleStylingValue(undefined)} saving={false} onSave={onSave} />);
    const hexFields = screen.getAllByRole('textbox', { name: 'HEX' });
    await user.clear(hexFields[0]);
    await user.type(hexFields[0], '#GGGGGG');
    expect(screen.getByText('Use a valid HEX color (#RRGGBB or #RGB).')).toBeTruthy();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('deduplicates saved colors, bounds the visible palette, and renders all preview roles/backgrounds', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<SubtitleStylingCard value={subtitleStylingValue(undefined)} saving={false} onSave={onSave} />);
    await user.click(screen.getByRole('button', { name: 'Save current color' }));
    await user.click(screen.getByRole('button', { name: 'Save current color' }));
    expect(screen.getByText('Saved colors (1/32)')).toBeTruthy();
    expect(screen.getByText('Recent colors (0/12): none')).toBeTruthy();
    for (const label of ['Dark background', 'Light background', 'Mixed background', 'Top note', 'Dialogue', 'Lyrics', 'Sign']) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0);
    }
    await user.click(screen.getByRole('button', { name: 'Use #FFFF00' }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it('shows the native override warning without starting an operation from preview controls', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const value = subtitleStylingValue({ subtitleStyling: { enabled: true, overrideExistingASSStyles: true } });
    render(<SubtitleStylingCard value={value} saving={false} onSave={onSave} />);
    expect(screen.getByText(/Native ASS\/SSA appearance/)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Use #00FFFF' }));
    expect(onSave).not.toHaveBeenCalled();
  });
});
