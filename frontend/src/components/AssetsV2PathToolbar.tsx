import { useState } from 'react';
import { Box, Divider, IconButton, Menu, Stack, Tooltip, Typography } from '@mui/material';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import type { ReactNode } from 'react';

export function AssetsV2PathToolbar({
  path,
  label,
  assetCount,
  selectedCount = 0,
  mode,
  primaryActions,
  pathOperations,
  destructiveActions,
}: {
  path: string;
  label: string;
  assetCount: number;
  selectedCount?: number;
  mode: 'raw' | 'library';
  primaryActions: ReactNode;
  pathOperations?: ReactNode;
  destructiveActions?: (close: () => void) => ReactNode;
}) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const closeMenu = () => setAnchor(null);

  return (
    <Box
      component="section"
      aria-label={`${mode === 'raw' ? 'RAW' : 'Library'} path actions`}
      sx={{
        mb: 0.75,
        px: { xs: 1, sm: 1.25 },
        py: 0.9,
        border: 1,
        borderColor: 'divider',
        borderRadius: 1.25,
        bgcolor: 'background.paper',
      }}
    >
      <Stack direction={{ xs: 'column', lg: 'row' }} alignItems={{ xs: 'stretch', lg: 'center' }} justifyContent="space-between" spacing={1}>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="subtitle2" fontWeight={700} noWrap title={path}>{label}</Typography>
          <Typography variant="caption" color="text.secondary" noWrap title={path}>
            {path} · {assetCount} {assetCount === 1 ? 'asset' : 'assets'} · {selectedCount} selected
          </Typography>
        </Box>
        <Stack direction="row" alignItems="center" spacing={0.75} flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
          {primaryActions}
          {pathOperations}
          {destructiveActions ? (
            <>
              <Divider orientation="vertical" flexItem sx={{ mx: 0.25, display: { xs: 'none', sm: 'block' } }} />
              <Tooltip title="Destructive path actions">
                <IconButton size="small" aria-label={`More path actions for ${path}`} onClick={(event) => setAnchor(event.currentTarget)}>
                  <MoreVertIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              <Menu anchorEl={anchor} open={Boolean(anchor)} onClose={closeMenu}>
                {destructiveActions(closeMenu)}
              </Menu>
            </>
          ) : null}
        </Stack>
      </Stack>
    </Box>
  );
}
