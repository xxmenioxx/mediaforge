// @vitest-environment jsdom

import { ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { AssetsSummaryStrip } from '../components/AssetsWorkspace';
import { mvforgeTokens, theme } from './theme';

afterEach(cleanup);

function relativeLuminance(hexColor: string) {
  const [red, green, blue] = hexColor.match(/[a-f\d]{2}/gi)!.map((channel) => parseInt(channel, 16) / 255);
  const linearize = (channel: number) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  return 0.2126 * linearize(red) + 0.7152 * linearize(green) + 0.0722 * linearize(blue);
}

function contrastRatio(foreground: string, background: string) {
  const foregroundLuminance = relativeLuminance(foreground);
  const backgroundLuminance = relativeLuminance(background);
  return (Math.max(foregroundLuminance, backgroundLuminance) + 0.05)
    / (Math.min(foregroundLuminance, backgroundLuminance) + 0.05);
}

describe('MVForge UI V2 theme', () => {
  it('creates semantic color, typography, and density tokens', () => {
    expect(theme.mvforge.colors.canvas).toBe('#04101d');
    expect(theme.mvforge.colors.workspace).toBe('#061525');
    expect(theme.mvforge.colors.surface).toBe('#091a2c');
    expect(theme.mvforge.colors.surfaceRaised).toBe('#0c2035');
    expect(theme.mvforge.colors.surfaceInteractive).toBe('#112a44');
    expect(theme.mvforge.colors.surfaceSelected).toBe('rgba(58, 43, 164, 0.56)');
    expect(theme.mvforge.colors.text).toEqual({
      primary: '#e7eef8',
      secondary: '#9cb0c8',
      muted: '#8398b0',
      disabled: '#71849b',
    });
    expect(theme.mvforge.colors.status.neutral).toBe('#71849b');
    expect(theme.mvforge.typography).toEqual({
      body: '0.8rem',
      compactBody: '0.78rem',
      supporting: '0.75rem',
      tertiary: '0.72rem',
      sectionHeading: '0.8rem',
      pageHeading: '1.25rem',
    });
    expect(theme.mvforge.density.controlHeight).toEqual({ compact: 32, normal: 36 });
    expect(theme.mvforge.density.compactRowHeight).toBe(42);
  });

  it('maps semantic colors into MUI and preserves the Inter-first typography', () => {
    expect(theme.palette.background.default).toBe(mvforgeTokens.colors.canvas);
    expect(theme.palette.background.paper).toBe(mvforgeTokens.colors.surface);
    expect(theme.palette.text.primary).toBe(mvforgeTokens.colors.text.primary);
    expect(theme.palette.text.secondary).toBe(mvforgeTokens.colors.text.secondary);
    expect(theme.palette.divider).toBe(mvforgeTokens.colors.border.subtle);
    expect(theme.palette.primary.main).toBe(mvforgeTokens.colors.accent.primary);
    expect(theme.palette.secondary.main).toBe(mvforgeTokens.colors.accent.secondary);
    expect(theme.palette.success.main).toBe(mvforgeTokens.colors.status.success);
    expect(theme.palette.warning.main).toBe(mvforgeTokens.colors.status.warning);
    expect(theme.palette.error.main).toBe(mvforgeTokens.colors.status.error);
    expect(theme.palette.info.main).toBe(mvforgeTokens.colors.status.info);
    expect(theme.typography.fontFamily).toBe(
      'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    );
    expect(theme.typography.h1.fontSize).toBe('2rem');
    expect(theme.typography.button.textTransform).toBe('none');
  });

  it('keeps text and status accents readable on their intended surfaces', () => {
    const colors = theme.mvforge.colors;
    for (const textColor of [colors.text.primary, colors.text.secondary, colors.text.muted, colors.text.disabled]) {
      expect(contrastRatio(textColor, colors.surface)).toBeGreaterThanOrEqual(4.5);
    }

    const statusPairs = [
      [colors.status.success, colors.status.successSurface],
      [colors.status.warning, colors.status.warningSurface],
      [colors.status.error, colors.status.errorSurface],
      [colors.status.info, colors.status.infoSurface],
    ];
    for (const [foreground, background] of statusPairs) {
      expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(3);
    }
  });

  it('renders the Assets summary with semantic surface, text, accent, and type tokens', () => {
    render(
      <ThemeProvider theme={theme}>
        <AssetsSummaryStrip total={21} processing={1} completed={18} needsReview={2} />
      </ThemeProvider>,
    );

    const label = screen.getByText('Total Assets');
    const value = screen.getByText('21');
    const card = label.closest('.MuiCard-root');

    expect(getComputedStyle(label).color).toBe('rgb(156, 176, 200)');
    expect(getComputedStyle(label).fontSize).toBe('12px');
    expect(getComputedStyle(value).color).toBe('rgb(231, 238, 248)');
    expect(card).not.toBeNull();
    expect(getComputedStyle(card as HTMLElement).backgroundImage).toContain('linear-gradient');
  });
});
