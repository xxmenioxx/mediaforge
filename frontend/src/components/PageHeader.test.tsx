// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { Button, Chip, ThemeProvider, Typography } from '@mui/material';
import { afterEach, describe, expect, it } from 'vitest';
import { PageContent, PageHeader } from './PageHeader';
import { theme } from '../theme/theme';

afterEach(cleanup);

describe('shared page chrome', () => {
  it('renders a semantic page heading with optional supporting text, actions, and metadata', () => {
    render(
      <ThemeProvider theme={theme}>
        <PageHeader
          eyebrow="Pipeline"
          title="Queue"
          actions={<Button>Queue job</Button>}
          metadata={<Chip size="small" label="3 active" />}
        >
          <Typography>Queue review and progress.</Typography>
        </PageHeader>
      </ThemeProvider>,
    );

    expect(screen.getByRole('banner')).toBeTruthy();
    expect(screen.getByRole('heading', { level: 1, name: 'Queue' })).toBeTruthy();
    expect(screen.getByText('Pipeline')).toBeTruthy();
    expect(screen.getByText('Queue review and progress.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Queue job' })).toBeTruthy();
    expect(screen.getByText('3 active')).toBeTruthy();
  });

  it('provides responsive gutters around a semantic workspace surface', () => {
    render(
      <ThemeProvider theme={theme}>
        <PageContent>
          <Typography>Operational workspace</Typography>
        </PageContent>
      </ThemeProvider>,
    );

    const workspace = screen.getByTestId('page-content-workspace');
    const content = screen.getByTestId('page-content-inner');
    const generatedCss = Array.from(document.styleSheets)
      .flatMap((styleSheet) => {
        try {
          return Array.from(styleSheet.cssRules).map((rule) => rule.cssText);
        } catch {
          return [];
        }
      })
      .join('\n');
    expect(window.getComputedStyle(workspace).backgroundColor).toBe('rgb(6, 21, 37)');
    expect(window.getComputedStyle(content).paddingBottom).toBe('32px');
    expect(window.getComputedStyle(content).minWidth).toBe('0px');
    expect(generatedCss).toMatch(/padding-left:\s*16px/);
    expect(generatedCss).toMatch(/@media\s*\(min-width:\s*900px\)[\s\S]*padding-left:\s*32px/);
  });
});
