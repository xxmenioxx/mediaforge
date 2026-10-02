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
    <Box sx={{ width: '100%', maxWidth: '100%', overflowX: 'auto', border: 1, borderColor: 'divider', borderRadius: 1.25, bgcolor: 'background.paper' }}>
      <Table
        size="small"
        aria-label={mode === 'raw' ? 'RAW assets' : 'Library assets'}
        sx={{
          width: '100%',
          minWidth,
          tableLayout: 'fixed',
          '& th': { color: 'text.secondary', fontSize: '0.72rem', fontWeight: 700, bgcolor: 'rgba(255,255,255,0.025)', borderColor: 'divider' },
          '& td': { borderColor: 'rgba(148, 163, 184, 0.12)' },
          '& td, & th': { py: 0.65, px: 0.8 },
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
