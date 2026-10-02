// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { AssetsInventoryTable } from './AssetsInventoryTable';

afterEach(cleanup);

describe('AssetsInventoryTable', () => {
  it('shows the compact Library technical columns', () => {
    render(<AssetsInventoryTable mode="library">{null}</AssetsInventoryTable>);

    const table = screen.getByRole('table', { name: 'Library assets' });
    const headers = screen.getAllByRole('columnheader').map((header) => header.textContent);
    expect(headers).toEqual([
      'Asset', 'Advisor', 'Status', 'Codec', 'Resolution', 'Encoder', 'Duration', 'Bitrate', 'HDR/SDR', 'Size', 'Modified', 'Actions',
    ]);
    expect(table.querySelector('th[aria-label="Select assets"]')).toBeNull();
  });

  it('uses source preparation columns and keeps RAW bulk selection available', () => {
    render(<AssetsInventoryTable mode="raw" selectable>{null}</AssetsInventoryTable>);

    const table = screen.getByRole('table', { name: 'RAW assets' });
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      '', 'Asset', 'Advisor', 'Status', 'Source / Codec', 'Resolution', 'Duration', 'Bitrate', 'Size', 'Modified', 'Actions',
    ]);
    expect(table.querySelector('th[aria-label="Select assets"]')).toBeTruthy();
  });
});
