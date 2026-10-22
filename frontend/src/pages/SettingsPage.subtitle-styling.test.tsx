// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SubtitleStylingCard } from './SettingsPage';
import { subtitleHexError, subtitleNormalizeHexColor, subtitleStylingValue } from '../utils/settingsPageHelpers';

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

  it('shows persisted numeric values, switches presets, and previews ASS units and zero outline/shadow', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const value = subtitleStylingValue({ subtitleStyling: { builtInOverrides: {
      MVF_Default: { fontSize: 34, outlineWidth: 0, shadowDepth: 0 },
      MVF_Top: { fontSize: 28, outlineWidth: 3, shadowDepth: 1 },
    } } });
    render(<SubtitleStylingCard value={value} saving={false} onSave={onSave} />);

    const fontSize = screen.getByRole('spinbutton', { name: 'Font size (ASS units)' }) as HTMLInputElement;
    const outline = screen.getByRole('spinbutton', { name: 'Outline width (ASS units)' }) as HTMLInputElement;
    const shadow = screen.getByRole('spinbutton', { name: 'Shadow depth (ASS units)' }) as HTMLInputElement;
    expect(fontSize.value).toBe('34');
    expect(outline.value).toBe('0');
    expect(shadow.value).toBe('0');
    expect(screen.getByText(/Saved: 34 ASS units/)).toBeTruthy();
    expect(screen.getAllByText(/Saved: 0 · draft: 0/)).toHaveLength(2);
    expect(screen.getByText(/384×288 PlayRes reference/)).toBeTruthy();
    expect(screen.getAllByText('We need to leave before sunrise.')).toHaveLength(2);

    const darkPreview = screen.getByRole('img', { name: 'Dark background dialogue preview' });
    const previewText = darkPreview.querySelector('text');
    expect(previewText?.getAttribute('font-size')).toBe('34');
    expect(previewText?.getAttribute('stroke-width')).toBe('0');
    expect(previewText?.getAttribute('filter')).toBeNull();

    await user.click(screen.getByRole('combobox', { name: 'Style preset' }));
    await user.click(screen.getByRole('option', { name: /Top/ }));
    expect(fontSize.value).toBe('28');
    expect(outline.value).toBe('3');
    expect(shadow.value).toBe('1');
    expect(darkPreview.querySelector('text')?.getAttribute('stroke-width')).toBe('6');
    expect(darkPreview.querySelector('text')?.getAttribute('filter')).toBe('url(#subtitle-shadow-dark)');
    expect(darkPreview.querySelector('filter feDropShadow')?.getAttribute('dx')).toBe('1');
    await user.clear(fontSize);
    await user.type(fontSize, '42');
    await user.clear(outline);
    await user.type(outline, '0');
    await user.clear(shadow);
    await user.type(shadow, '0');
    await user.click(screen.getByRole('switch', { name: 'Auto font size (fixed 20-unit fallback)' }));
    await user.click(screen.getByRole('switch', { name: 'Bold' }));
    await user.click(screen.getByRole('button', { name: 'Use #00FFFF' }));

    expect(screen.getByText(/Auto font size is a legacy fixed fallback/)).toBeTruthy();
    expect(screen.getByText(/draft output: 20 ASS units/)).toBeTruthy();
    const updatedPreviewText = darkPreview.querySelector('text');
    expect(updatedPreviewText?.getAttribute('font-size')).toBe('20');
    expect(updatedPreviewText?.getAttribute('stroke-width')).toBe('0');
    expect(updatedPreviewText?.getAttribute('font-weight')).toBe('700');
    expect(updatedPreviewText?.getAttribute('fill')).toBe('#00FFFF');
    expect(updatedPreviewText?.getAttribute('filter')).toBeNull();

    await user.click(screen.getByRole('combobox', { name: 'Style preset' }));
    await user.click(screen.getByRole('option', { name: /Default/ }));
    expect(fontSize.value).toBe('34');
    await user.click(screen.getByRole('combobox', { name: 'Style preset' }));
    await user.click(screen.getByRole('option', { name: /Top/ }));
    await user.click(screen.getByRole('button', { name: 'Save Subtitle Styling' }));

    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({
      builtInOverrides: expect.objectContaining({
        MVF_Top: expect.objectContaining({ fontSize: 42, autoFontSize: true, outlineWidth: 0, shadowDepth: 0, bold: true, primaryColor: '#00FFFF' }),
      }),
    }));
  });

  it('resets a local draft to defaults and saves only after explicit confirmation', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const value = subtitleStylingValue({ subtitleStyling: {
      enabled: true,
      builtInOverrides: { MVF_Default: { fontSize: 44, outlineWidth: 8, shadowDepth: 5 } },
      customPresets: [{ id: 'custom_saved', displayName: 'Saved custom', primaryColor: '#FFFFFF', outlineColor: '#000000', shadowBackColor: '#000000', opacity: 100, fontFamily: 'Arial', fontSize: 20, autoFontSize: false, bold: false, outlineWidth: 2, shadowDepth: 0, alignmentBehavior: 'bottom', provenance: 'custom' }],
    } });
    render(<SubtitleStylingCard value={value} saving={false} onSave={onSave} />);

    await user.click(screen.getByRole('button', { name: 'Reset to defaults' }));
    expect(screen.getByRole('switch', { name: 'Enable position-aware styling' })).toHaveProperty('checked', false);
    expect(screen.getByRole('spinbutton', { name: 'Font size (ASS units)' })).toHaveProperty('value', '20');
    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Reset to defaults' })).toHaveProperty('disabled', true);

    await user.click(screen.getByRole('button', { name: 'Save Subtitle Styling' }));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, builtInOverrides: {}, customPresets: [] }));
  });

  it('deduplicates saved colors, bounds the visible palette, and renders dialogue on light and dark backgrounds', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<SubtitleStylingCard value={subtitleStylingValue(undefined)} saving={false} onSave={onSave} />);
    await user.click(screen.getByRole('button', { name: 'Save current color' }));
    await user.click(screen.getByRole('button', { name: 'Save current color' }));
    expect(screen.getByText('Saved colors (1/32)')).toBeTruthy();
    expect(screen.getByText('Recent colors (0/12): none')).toBeTruthy();
    expect(screen.getByText('Dark background')).toBeTruthy();
    expect(screen.getByText('Light background')).toBeTruthy();
    expect(screen.getAllByText('We need to leave before sunrise.')).toHaveLength(2);
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
