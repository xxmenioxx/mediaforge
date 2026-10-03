import { Box, Button, Popover, Stack, Tooltip, Typography } from '@mui/material';
import FilterListOutlinedIcon from '@mui/icons-material/FilterListOutlined';
import RefreshOutlinedIcon from '@mui/icons-material/RefreshOutlined';
import SwapVertOutlinedIcon from '@mui/icons-material/SwapVertOutlined';
import { useState, type ReactNode } from 'react';

export function AssetsV2InventoryControls({
  count,
  filterCount,
  syncDetails,
  syncPending,
  onSync,
  children,
}: {
  count: number;
  filterCount: number;
  syncDetails: string;
  syncPending: boolean;
  onSync: () => void;
  children: ReactNode;
}) {
  const [filterAnchor, setFilterAnchor] = useState<HTMLElement | null>(null);

  return (
    <Box
      component="section"
      aria-label="Asset list controls"
      data-testid="assets-inventory-controls"
      sx={{
        minHeight: 42,
        px: { xs: 0.75, sm: 1.5 },
        py: 0.4,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1,
        borderBottom: 1,
        borderColor: '#15304a',
        color: '#9cb0c8',
        bgcolor: '#071727',
      }}
    >
      <Typography variant="caption" aria-label={`${count.toLocaleString()} assets`} sx={{ fontSize: '0.66rem', color: '#9cb0c8', whiteSpace: 'nowrap' }}>
        {count.toLocaleString()} {count === 1 ? 'asset' : 'assets'}
      </Typography>
      <Stack direction="row" alignItems="center" spacing={0.5}>
        <Tooltip title={syncDetails}>
          <span>
            <Button
              size="small"
              variant="contained"
              startIcon={<RefreshOutlinedIcon />}
              onClick={onSync}
              disabled={syncPending}
              sx={{ minHeight: 30, px: 1.1, borderRadius: '5px', border: '1px solid #6c4cf6', fontSize: '0.64rem', fontWeight: 600, textTransform: 'none', bgcolor: '#6c4cf6', color: '#f3f0ff', boxShadow: 'none', '&:hover': { borderColor: '#8772ff', bgcolor: '#755cf8', color: '#fff', boxShadow: 'none' }, '&.Mui-disabled': { borderColor: '#15304a', bgcolor: '#0c2035', color: '#526981' }, '& .MuiButton-startIcon': { mr: 0.45 }, '& svg': { fontSize: 14 } }}
            >
              Sync
            </Button>
          </span>
        </Tooltip>
        <Button
          size="small"
          variant="outlined"
          startIcon={<FilterListOutlinedIcon />}
          onClick={(event) => setFilterAnchor(event.currentTarget)}
          aria-label={filterCount ? `Filter assets, ${filterCount} active` : 'Filter assets'}
          aria-expanded={Boolean(filterAnchor)}
          aria-controls="assets-filter-popover"
          sx={{ minHeight: 30, px: 1, borderRadius: '5px', fontSize: '0.64rem', fontWeight: 600, textTransform: 'none', borderColor: '#15304a', bgcolor: '#0c2035', color: '#9cb0c8', '&:hover': { borderColor: '#1f4467', bgcolor: '#112a44', color: '#e7eef8' }, '& .MuiButton-startIcon': { mr: 0.45 }, '& svg': { fontSize: 14 } }}
        >
          Filter{filterCount ? ` · ${filterCount}` : ''}
        </Button>
        <Tooltip title="Sorting is not available for this inventory yet">
          <span>
            <Button
              size="small"
              variant="outlined"
              startIcon={<SwapVertOutlinedIcon />}
              disabled
              aria-label="Sort assets (unavailable)"
              sx={{ minHeight: 30, px: 1, borderRadius: '5px', fontSize: '0.64rem', fontWeight: 600, textTransform: 'none', borderColor: '#15304a', color: '#526981', bgcolor: 'rgba(12, 32, 53, 0.38)', '& .MuiButton-startIcon': { mr: 0.45 }, '& svg': { fontSize: 14 } }}
            >
              Sort
            </Button>
          </span>
        </Tooltip>
      </Stack>
      <Popover
        id="assets-filter-popover"
        open={Boolean(filterAnchor)}
        anchorEl={filterAnchor}
        onClose={() => setFilterAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        PaperProps={{ sx: { mt: 0.75, p: 1.5, width: { xs: 'calc(100vw - 32px)', sm: 620 }, maxWidth: 'calc(100vw - 24px)', bgcolor: '#091a2c', border: 1, borderColor: '#15304a', borderRadius: 1.25 } }}
      >
        <Box data-testid="assets-filter-popover-content">{children}</Box>
      </Popover>
    </Box>
  );
}
