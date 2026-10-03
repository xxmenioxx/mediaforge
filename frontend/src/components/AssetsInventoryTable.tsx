import { Box, Table, TableBody, TableCell, TableHead, TableRow } from '@mui/material';
import type { ReactNode } from 'react';

export type AssetsInventoryMode = 'library' | 'raw';

type InventoryColumn = {
  label: string;
  width: number;
};

const libraryColumns: InventoryColumn[] = [
  { label: 'Asset', width: 210 },
  { label: 'Advisor', width: 105 },
  { label: 'Status', width: 125 },
  { label: 'Codec', width: 88 },
  { label: 'Resolution', width: 105 },
  { label: 'Encoder', width: 140 },
  { label: 'Duration', width: 90 },
  { label: 'Bitrate', width: 105 },
  { label: 'HDR/SDR', width: 82 },
  { label: 'Size', width: 95 },
  { label: 'Modified', width: 145 },
  { label: 'Actions', width: 52 },
];

const rawColumns: InventoryColumn[] = [
  { label: 'Asset', width: 220 },
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

export function AssetsInventoryTable({ mode, selectable = false, children }: {
  mode: AssetsInventoryMode;
  selectable?: boolean;
  children: ReactNode;
}) {
  const columns = assetsInventoryColumns(mode);
  const selectionWidth = selectable ? 42 : 0;
  const minWidth = columns.reduce((total, column) => total + column.width, selectionWidth);

  return (
    <Box sx={{ width: '100%', maxWidth: '100%', overflowX: 'auto', border: 1, borderColor: 'rgba(39, 75, 108, 0.68)', borderRadius: 1, bgcolor: '#071727' }}>
      <Table
        size="small"
        aria-label={mode === 'raw' ? 'RAW assets' : 'Library assets'}
        sx={{
          width: '100%',
          minWidth,
          tableLayout: 'fixed',
          borderCollapse: 'separate',
          borderSpacing: 0,
          '& th': { color: '#93aac1', fontSize: '0.67rem', fontWeight: 700, bgcolor: '#0b1d30', borderColor: 'rgba(39, 75, 108, 0.48)', whiteSpace: 'nowrap', letterSpacing: '0.01em' },
          '& td': { borderColor: 'rgba(39, 75, 108, 0.34)', color: '#d5e0ec' },
          '& td, & th': { py: 0.66, px: 0.8 },
        }}
      >
        <TableHead>
          <TableRow>
            {selectable ? <TableCell padding="checkbox" scope="col" aria-label="Select assets" sx={{ width: selectionWidth }} /> : null}
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
