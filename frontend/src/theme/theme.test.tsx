// @vitest-environment jsdom

import { ThemeProvider } from '@mui/material/styles';
import {
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableRow,
  TextField,
} from '@mui/material';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { CompactActionGroup } from '../components/CompactActionGroup';
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

  it('keeps normal controls intact and applies compact, disabled, and focus-visible behavior', async () => {
    const user = userEvent.setup();
    render(
      <ThemeProvider theme={theme}>
        <Button data-testid="normal-button">Normal action</Button>
        <CompactActionGroup role="group" aria-label="Workspace actions">
          <Button data-testid="compact-button">Compact action</Button>
          <Button data-testid="disabled-button" size="small" variant="contained" color="secondary" disabled>
            Disabled action
          </Button>
          <IconButton aria-label="Workspace options"><span aria-hidden="true">⋯</span></IconButton>
        </CompactActionGroup>
        <TextField size="small" label="Compact filter" helperText="Filter by name" />
        <TextField size="small" label="Invalid field" error helperText="Required field" />
      </ThemeProvider>,
    );

    const normalButton = screen.getByTestId('normal-button');
    const compactButton = screen.getByTestId('compact-button');
    const disabledButton = screen.getByTestId('disabled-button');
    const iconButton = screen.getByRole('button', { name: 'Workspace options' });
    const compactField = screen.getByLabelText('Compact filter').closest('.MuiOutlinedInput-root');
    const invalidField = screen.getByLabelText('Invalid field').closest('.MuiOutlinedInput-root');
    const invalidOutline = invalidField?.querySelector('.MuiOutlinedInput-notchedOutline');

    expect(getComputedStyle(normalButton).minHeight).toBe('40px');
    expect(getComputedStyle(compactButton).minHeight).toBe('32px');
    expect(disabledButton.hasAttribute('disabled')).toBe(true);
    expect(getComputedStyle(disabledButton).backgroundColor).not.toBe('rgb(167, 139, 250)');
    expect(getComputedStyle(iconButton).minHeight).toBe('40px');
    expect(getComputedStyle(iconButton).minWidth).toBe('40px');
    expect(getComputedStyle(compactField as HTMLElement).minHeight).toBe('32px');
    expect(screen.queryByText('Filter by name')).not.toBeNull();
    expect(screen.queryByText('Required field')).not.toBeNull();
    expect(getComputedStyle(invalidOutline as HTMLElement).borderColor).toBe('rgb(255, 78, 106)');

    await user.tab();
    await user.tab();
    expect(document.activeElement).toBe(compactButton);
    const focusStyles = Array.from(document.querySelectorAll('style[data-emotion]'))
      .map((style) => style.textContent ?? '')
      .join('\n');
    expect(focusStyles).toContain('Mui-focusVisible{outline:2px solid #6c4cf6;outline-offset:2px;}');
  });

  it('applies semantic status, surface, table, and dialog treatments', () => {
    render(
      <ThemeProvider theme={theme}>
        <Chip label="Ready" color="success" data-testid="success-chip" />
        <Card data-testid="surface-card">Summary</Card>
        <Paper data-testid="outlined-paper" variant="outlined">Details</Paper>
        <Table data-ui-selection="tinted">
          <TableBody>
            <TableRow hover selected>
              <TableCell data-testid="selected-cell">Selected row</TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <Dialog open onClose={() => undefined}>
          <DialogTitle>Dialog heading</DialogTitle>
          <DialogContent dividers>Dialog content</DialogContent>
          <DialogActions><Button>Close</Button></DialogActions>
        </Dialog>
      </ThemeProvider>,
    );

    expect(getComputedStyle(screen.getByTestId('success-chip')).color).toBe('rgb(0, 217, 155)');
    expect(getComputedStyle(screen.getByTestId('success-chip')).backgroundColor).toBe('rgb(7, 61, 53)');
    expect(getComputedStyle(screen.getByTestId('surface-card')).backgroundColor).toBe('rgb(9, 26, 44)');
    expect(getComputedStyle(screen.getByTestId('outlined-paper')).borderColor).toBe('rgb(21, 48, 74)');
    expect(getComputedStyle(screen.getByTestId('selected-cell')).backgroundColor).toBe('rgba(58, 43, 164, 0.56)');
    const dialog = screen.getByRole('dialog');
    expect(dialog.classList.contains('MuiDialog-paper')).toBe(true);
    expect(getComputedStyle(dialog).backgroundColor).toBe('rgb(12, 32, 53)');
    expect(dialog.querySelector('.MuiDialogActions-root')).not.toBeNull();

    const generatedStyles = Array.from(document.querySelectorAll('style[data-emotion]'))
      .map((style) => style.textContent ?? '')
      .join('\n');
    expect(generatedStyles).toContain('.MuiTableRow-hover:hover');
    expect(generatedStyles).toContain(mvforgeTokens.colors.surfaceInteractive);
  });
});
