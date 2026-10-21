// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { TableCell, TableRow } from '@mui/material';
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

  it('keeps a bounded, keyboard-reachable Library scroller and row actions with long names and large pages', () => {
    const longName = `${'A very long Library filename '.repeat(12)}.mkv`;
    const onFinalAction = vi.fn();
    render(
      <div style={{ width: 420, resize: 'horizontal', overflow: 'hidden' }}>
        <AssetsInventoryTable mode="library" selectable>
          {Array.from({ length: 100 }, (_, index) => (
            <TableRow key={index}>
              {Array.from({ length: 13 }, (__, cellIndex) => (
                <TableCell key={cellIndex}>
                  {cellIndex === 1 ? index === 0 ? longName : `Asset ${index + 1}.mkv` : cellIndex === 12 ? (
                    <button type="button" aria-label={`More actions for asset ${index + 1}`} onClick={onFinalAction}>More</button>
                  ) : `Asset ${index + 1}`}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </AssetsInventoryTable>
      </div>,
    );

    const scroller = screen.getByTestId('assets-inventory-scroll');
    const table = screen.getByRole('table', { name: 'Library assets' });
    expect(screen.getByRole('region', { name: 'Library table horizontal scroll area' })).toBe(scroller);
    expect(scroller.tabIndex).toBe(0);
    expect(table.getAttribute('data-inventory-layout')).toBe('library-responsive');
    expect(table.querySelectorAll('tbody tr')).toHaveLength(100);
    expect(screen.getByText(longName)).toBeTruthy();
    const finalAction = screen.getByRole('button', { name: 'More actions for asset 100' });
    finalAction.focus();
    expect(document.activeElement).toBe(finalAction);
    fireEvent.click(finalAction);
    expect(onFinalAction).toHaveBeenCalledOnce();
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
