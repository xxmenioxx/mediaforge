// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AssetsInventoryTable } from './AssetsInventoryTable';

afterEach(cleanup);

describe('AssetsInventoryTable', () => {
  it('shows the compact Library technical columns', () => {
    render(<AssetsInventoryTable mode="library">{null}</AssetsInventoryTable>);

    expect(screen.getByRole('table', { name: 'Library assets' }).getAttribute('data-ui-selection')).toBeNull();

    const table = screen.getByRole('table', { name: 'Library assets' });
    const headers = screen.getAllByRole('columnheader').map((header) => header.textContent);
    expect(headers).toEqual([
      'Asset', 'Advisor', 'Status', 'Codec', 'Resolution', 'Encoder', 'Duration', 'Bitrate', 'HDR/SDR', 'Size', 'Modified', 'Actions',
    ]);
    expect(table.querySelector('th[aria-label="Select assets"]')).toBeNull();
  });

  it('uses source preparation columns and keeps RAW bulk selection available', () => {
    render(<AssetsInventoryTable mode="raw" selectable>{null}</AssetsInventoryTable>);

    screen.getByRole('table', { name: 'RAW assets' });
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual([
      '', 'Asset', 'Advisor', 'Status', 'Source / Codec', 'Resolution', 'Duration', 'Bitrate', 'Size', 'Modified', 'Actions',
    ]);
    expect(screen.getByRole('checkbox', { name: 'Select all visible RAW assets' })).toBeTruthy();
  });

  it('exposes checked and mixed header states and selects through the provided callback', () => {
    const onChange = vi.fn();
    render(
      <AssetsInventoryTable
        mode="library"
        selectable
        selectAll={{ checked: false, indeterminate: true, disabled: false, onChange }}
      >{null}</AssetsInventoryTable>,
    );

    const header = screen.getByRole('checkbox', { name: 'Select all visible Library assets' });
    expect(header.getAttribute('aria-checked')).toBe('mixed');
    expect((header as HTMLInputElement).disabled).toBe(false);
    fireEvent.click(header);
    expect(onChange).toHaveBeenCalledWith(true);
  });

  it('disables header selection when the current visible page has no eligible assets', () => {
    render(
      <AssetsInventoryTable
        mode="raw"
        selectable
        selectAll={{ checked: false, indeterminate: false, disabled: true, onChange: vi.fn() }}
      >{null}</AssetsInventoryTable>,
    );
    expect((screen.getByRole('checkbox', { name: 'Select all visible RAW assets' }) as HTMLInputElement).disabled).toBe(true);
  });
});
