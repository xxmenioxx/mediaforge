import { Box, Divider, Stack, Typography } from '@mui/material';
import type { ReactNode } from 'react';

export function AssetsV2PathToolbar({
  path,
  mode,
  breadcrumbs,
  primaryActions,
  pathOperations,
  selectedCount = 0,
  destructiveActions,
}: {
  path: string;
  mode: 'raw' | 'library';
  breadcrumbs: ReactNode;
  primaryActions?: ReactNode;
  pathOperations?: ReactNode;
  selectedCount?: number;
  destructiveActions?: (close: () => void) => ReactNode;
}) {
  return (
    <Box
      component="section"
      aria-label={`${mode === 'raw' ? 'RAW' : 'Library'} path actions for ${path || 'current location'}`}
      data-testid="assets-path-toolbar"
      sx={{
        minHeight: 44,
        mb: 0.7,
        px: { xs: 0.85, sm: 1.25 },
        py: 0.55,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1,
        border: 1,
        borderColor: '#15304a',
        borderRadius: 1,
        bgcolor: '#091a2c',
      }}
    >
      <Box sx={{ minWidth: 120, flex: '1 1 220px', overflow: 'hidden' }}>{breadcrumbs}</Box>
      <Stack direction="row" alignItems="center" justifyContent="flex-end" spacing={0.45} flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
        {primaryActions}
        {pathOperations}
        {destructiveActions ? (
          <>
            <Divider orientation="vertical" flexItem sx={{ mx: 0.35, height: 24, alignSelf: 'center', borderColor: '#1f4467' }} />
            <Stack direction="row" alignItems="center" spacing={0.35} flexWrap="wrap" useFlexGap aria-label="Destructive path actions" data-testid="assets-path-destructive-actions">
              {destructiveActions(() => undefined)}
            </Stack>
          </>
        ) : null}
        {selectedCount > 0 ? (
          <Typography variant="caption" aria-label={`${selectedCount} selected`} sx={{ px: 0.5, color: '#59718b', fontSize: '0.64rem', whiteSpace: 'nowrap' }}>
            {selectedCount} selected
          </Typography>
        ) : null}
      </Stack>
    </Box>
  );
}
