import { Box, Button, Popover, Tooltip, Typography } from '@mui/material';
import FilterListOutlinedIcon from '@mui/icons-material/FilterListOutlined';
import RefreshOutlinedIcon from '@mui/icons-material/RefreshOutlined';
import SwapVertOutlinedIcon from '@mui/icons-material/SwapVertOutlined';
import { useTheme } from '@mui/material/styles';
import { useState, type ReactNode } from 'react';
import { CompactActionGroup } from './CompactActionGroup';
import { mvforgeTokens } from '../theme/theme';

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
  const tokens = useTheme().mvforge ?? mvforgeTokens;

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
        borderColor: tokens.colors.border.default,
        color: tokens.colors.text.secondary,
        bgcolor: tokens.colors.workspace,
      }}
    >
      <Typography variant="caption" aria-label={`${count.toLocaleString()} assets`} sx={{ fontSize: tokens.typography.supporting, color: tokens.colors.text.secondary, whiteSpace: 'nowrap' }}>
        {count.toLocaleString()} {count === 1 ? 'asset' : 'assets'}
      </Typography>
      <CompactActionGroup role="group" aria-label="Asset list actions" justifyContent="flex-end">
        <Tooltip title={syncDetails}>
          <span>
            <Button
              size="small"
              variant="contained"
              color="secondary"
              startIcon={<RefreshOutlinedIcon />}
              onClick={onSync}
              disabled={syncPending}
            >
              Sync
            </Button>
          </span>
        </Tooltip>
        <Button
          size="small"
          variant="outlined"
          color="inherit"
          startIcon={<FilterListOutlinedIcon />}
          onClick={(event) => setFilterAnchor(event.currentTarget)}
          aria-label={filterCount ? `Filter assets, ${filterCount} active` : 'Filter assets'}
          aria-expanded={Boolean(filterAnchor)}
          aria-controls="assets-filter-popover"
        >
          Filter{filterCount ? ` · ${filterCount}` : ''}
        </Button>
        <Tooltip title="Sorting is not available for this inventory yet">
          <span>
            <Button
              size="small"
              variant="outlined"
              color="inherit"
              startIcon={<SwapVertOutlinedIcon />}
              disabled
              aria-label="Sort assets (unavailable)"
            >
              Sort
            </Button>
          </span>
        </Tooltip>
      </CompactActionGroup>
      <Popover
        id="assets-filter-popover"
        open={Boolean(filterAnchor)}
        anchorEl={filterAnchor}
        onClose={() => setFilterAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        PaperProps={{ sx: { mt: 0.75, p: 1.5, width: { xs: 'calc(100vw - 32px)', sm: 620 }, maxWidth: 'calc(100vw - 24px)' } }}
      >
        <Box data-testid="assets-filter-popover-content">{children}</Box>
      </Popover>
    </Box>
  );
}
