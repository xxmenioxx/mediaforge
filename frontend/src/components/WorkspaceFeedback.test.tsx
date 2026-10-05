// @vitest-environment jsdom

import { ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { theme } from '../theme/theme';
import { WorkspaceFeedback } from './WorkspaceFeedback';

afterEach(cleanup);

function renderFeedback(kind: 'loading' | 'empty' | 'error') {
  return render(
    <ThemeProvider theme={theme}>
      <WorkspaceFeedback kind={kind}>No assets are available.</WorkspaceFeedback>
    </ThemeProvider>,
  );
}

describe('WorkspaceFeedback', () => {
  it('announces a compact loading state', () => {
    renderFeedback('loading');

    expect(screen.getByRole('status').textContent).toContain('No assets are available.');
    expect(screen.getByRole('progressbar', { name: 'Loading' })).not.toBeNull();
  });

  it('presents an empty state on the common outlined surface', () => {
    renderFeedback('empty');

    const status = screen.getByRole('status');
    expect(status.classList.contains('MuiPaper-outlined')).toBe(true);
    expect(getComputedStyle(status).backgroundColor).toBe('rgb(9, 26, 44)');
  });

  it('keeps recoverable errors in an accessible semantic alert', () => {
    renderFeedback('error');

    expect(screen.getByRole('alert').textContent).toContain('No assets are available.');
  });
});
