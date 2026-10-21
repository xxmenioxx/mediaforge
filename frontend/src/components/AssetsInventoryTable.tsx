import { Box, Checkbox, Table, TableBody, TableCell, TableHead, TableRow } from '@mui/material';
import type { ReactNode } from 'react';

export type AssetsInventoryMode = 'library' | 'raw';

export type AssetsInventorySelectAll = {
  checked: boolean;
  indeterminate: boolean;
  disabled: boolean;
  onChange: (selected: boolean) => void;
};

type InventoryColumn = {
  label: string;
  width: number;
};

const libraryColumns: InventoryColumn[] = [
  { label: 'Asset', width: 300 },
  { label: 'Advisor', width: 85 },
  { label: 'Status', width: 100 },
  { label: 'Codec', width: 74 },
  { label: 'Resolution', width: 90 },
  { label: 'Encoder', width: 105 },
  { label: 'Duration', width: 70 },
  { label: 'Bitrate', width: 82 },
  { label: 'HDR/SDR', width: 70 },
  { label: 'Size', width: 78 },
  { label: 'Modified', width: 112 },
  { label: 'Actions', width: 52 },
];

const rawColumns: InventoryColumn[] = [
  { label: 'Asset', width: 340 },
  { label: 'Advisor', width: 105 },
  { label: 'Status', width: 130 },
  { label: 'Source / Codec', width: 170 },
  { label: 'Resolution', width: 115 },
  { label: 'Duration', width: 100 },
  { label: 'Bitrate', width: 115 },
  { label: 'Size', width: 100 },
  { label: 'Modified', width: 160 },
  { label: 'Actions', width: 52 },
];

function assetsInventoryColumns(mode: AssetsInventoryMode) {
  return mode === 'raw' ? rawColumns : libraryColumns;
}

export function AssetsInventoryTable({ mode, selectable = false, selectAll, children }: {
  mode: AssetsInventoryMode;
  selectable?: boolean;
  selectAll?: AssetsInventorySelectAll;
  children: ReactNode;
}) {
  const columns = assetsInventoryColumns(mode);
  const selectionWidth = selectable ? 42 : 0;
  const minWidth = columns.reduce((total, column) => total + column.width, selectionWidth);
  const responsiveLibrary = mode === 'library' && selectable;

  return (
    <Box data-testid="assets-inventory-scroll" role={responsiveLibrary ? 'region' : undefined} aria-label={responsiveLibrary ? 'Library table horizontal scroll area' : undefined} tabIndex={responsiveLibrary ? 0 : undefined} sx={{ width: '100%', maxWidth: '100%', minWidth: responsiveLibrary ? 0 : undefined, overflowX: 'auto', overscrollBehaviorX: responsiveLibrary ? 'contain' : undefined, containerType: responsiveLibrary ? 'inline-size' : undefined, containerName: responsiveLibrary ? 'assets-library-inventory' : undefined, border: 1, borderColor: 'divider', borderRadius: 1, bgcolor: 'background.default' }}>
      <Table
        size="small"
        data-ui-density="compact"
        data-inventory-layout={responsiveLibrary ? 'library-responsive' : 'fixed'}
        aria-label={mode === 'raw' ? 'RAW assets' : 'Library assets'}
        sx={{
          width: '100%',
          minWidth: responsiveLibrary ? 0 : minWidth,
          tableLayout: 'fixed',
          borderCollapse: 'separate',
          borderSpacing: '0 4px',
          '& th': { height: 33, whiteSpace: 'nowrap', letterSpacing: '0.015em' },
          '& td': { borderColor: 'divider' },
          '& td, & th': { py: 0.48, px: 0.72 },
          '& tbody td': { height: 39 },
          '& tbody tr:not(.Mui-selected) > td': {
            borderTop: (theme) => `1px solid ${theme.palette.divider}`,
            borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
          },
          '& tbody tr > td:first-of-type': { borderTopLeftRadius: '5px', borderBottomLeftRadius: '5px' },
          '& tbody tr > td:last-of-type': { borderTopRightRadius: '5px', borderBottomRightRadius: '5px' },
          ...(responsiveLibrary ? {
            '@container assets-library-inventory (max-width: 1279px)': {
              minWidth: 872,
              '& th:nth-child(2), & td:nth-child(2)': { width: 280 },
              '& th:nth-child(4), & td:nth-child(4)': { width: 90 },
              '& th:nth-child(5), & td:nth-child(5)': { width: 75 },
              '& th:nth-child(6), & td:nth-child(6)': { width: 85 },
              '& th:nth-child(8), & td:nth-child(8)': { width: 70 },
              '& th:nth-child(11), & td:nth-child(11)': { width: 75 },
              '& th:nth-child(12), & td:nth-child(12)': { width: 100 },
              '& th:nth-child(13), & td:nth-child(13)': { width: 55 },
              '& th:nth-child(3), & td:nth-child(3), & th:nth-child(7), & td:nth-child(7), & th:nth-child(9), & td:nth-child(9), & th:nth-child(10), & td:nth-child(10)': { display: 'none' },
            },
            '@container assets-library-inventory (max-width: 959px)': {
              minWidth: 692,
              '& th:nth-child(5), & td:nth-child(5), & th:nth-child(8), & td:nth-child(8)': { display: 'none' },
              '& th:nth-child(2), & td:nth-child(2)': { width: 250 },
              '& th:nth-child(4), & td:nth-child(4)': { width: 85 },
              '& th:nth-child(6), & td:nth-child(6)': { width: 85 },
              '& th:nth-child(11), & td:nth-child(11)': { width: 75 },
              '& th:nth-child(12), & td:nth-child(12)': { width: 100 },
              '& th:nth-child(13), & td:nth-child(13)': { width: 55 },
            },
            '@container assets-library-inventory (max-width: 799px)': {
              minWidth: 567,
              '& th:nth-child(6), & td:nth-child(6)': { display: 'none' },
              '& th:nth-child(2), & td:nth-child(2)': { width: 230 },
              '& th:nth-child(4), & td:nth-child(4)': { width: 80 },
              '& th:nth-child(11), & td:nth-child(11)': { width: 70 },
              '& th:nth-child(12), & td:nth-child(12)': { width: 90 },
              '& th:nth-child(13), & td:nth-child(13)': { width: 55 },
            },
            '@container assets-library-inventory (max-width: 675px)': {
              minWidth: 0,
              '& th:nth-child(6), & td:nth-child(6), & th:nth-child(11), & td:nth-child(11), & th:nth-child(12), & td:nth-child(12)': { display: 'none' },
              '& th:nth-child(2), & td:nth-child(2)': { width: 125 },
              '& th:nth-child(4), & td:nth-child(4)': { width: 60 },
              '& th:nth-child(13), & td:nth-child(13)': { width: 45 },
            },
          } : {}),
        }}
      >
        <TableHead>
          <TableRow>
            {selectable ? (
              <TableCell padding="checkbox" scope="col" sx={{ width: selectionWidth }}>
                <Checkbox
                  size="small"
                  checked={selectAll?.checked ?? false}
                  indeterminate={selectAll?.indeterminate ?? false}
                  disabled={selectAll?.disabled ?? true}
                  onChange={(event) => selectAll?.onChange(event.target.checked)}
                  inputProps={{
                    'aria-label': `Select all visible ${mode === 'raw' ? 'RAW' : 'Library'} assets`,
                    'aria-checked': selectAll?.indeterminate ? 'mixed' : selectAll?.checked ?? false,
                  }}
                />
              </TableCell>
            ) : null}
            {columns.map((column) => (
              <TableCell key={column.label} scope="col" sx={{ width: column.width }}>
                {column.label}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>{children}</TableBody>
      </Table>
    </Box>
  );
}
