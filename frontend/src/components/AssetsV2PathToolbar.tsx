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
        py: 0.45,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1,
        border: 1,
        borderColor: '#15304a',
        borderRadius: '6px',
        bgcolor: '#071727',
        '& .MuiButton-root': {
          minHeight: 28,
          height: 28,
          px: 0.9,
          borderRadius: '5px',
          borderColor: '#15304a',
          bgcolor: '#0c2035',
          color: '#9cb0c8',
          fontSize: '0.64rem',
          fontWeight: 600,
          lineHeight: 1,
          textTransform: 'none',
          whiteSpace: 'nowrap',
          boxShadow: 'none',
          '&:hover': { borderColor: '#1f4467', bgcolor: '#112a44', color: '#e7eef8', boxShadow: 'none' },
          '&.Mui-disabled': { borderColor: 'rgba(21, 48, 74, 0.55)', bgcolor: 'rgba(12, 32, 53, 0.46)', color: '#526981' },
          '& .MuiButton-startIcon': { mr: 0.4 },
          '& .MuiButton-startIcon svg': { fontSize: 14 },
        },
        '& .MuiAutocomplete-root': { flex: '0 0 auto' },
        '& .MuiAutocomplete-root .MuiInputBase-root': { minHeight: 32, height: 32, fontSize: '0.66rem', borderRadius: '5px' },
        '& .MuiAutocomplete-root .MuiAutocomplete-input': { py: '4px !important', px: '8px !important', fontSize: '0.66rem' },
        '& .MuiAutocomplete-root .MuiInputLabel-root': { fontSize: '0.66rem', transform: 'translate(12px, 8px) scale(1)' },
        '& .MuiAutocomplete-root .MuiInputLabel-root.MuiInputLabel-shrink': { transform: 'translate(12px, -7px) scale(0.75)' },
        '& .MuiAutocomplete-root .MuiAutocomplete-endAdornment': { right: 2 },
        '& .MuiAutocomplete-root .MuiAutocomplete-popupIndicator': { p: 0.25 },
        '& .MuiAutocomplete-root .MuiAutocomplete-popupIndicator svg': { fontSize: 16 },
        '& .MuiButton-contained': { border: '1px solid #1f4467', bgcolor: '#0c2035', color: '#c0c9e8' },
        '& .MuiButton-contained:hover': { bgcolor: '#112a44' },
        '& [data-testid="assets-path-destructive-actions"] .MuiButton-colorError.MuiButton-contained': {
          borderColor: '#982841',
          bgcolor: '#4b1826',
          color: '#ff92a3',
          '&:hover': { borderColor: '#b63750', bgcolor: 'rgba(255, 78, 106, 0.2)', color: '#ffb1bd' },
        },
        '& [data-testid="assets-path-destructive-actions"] .MuiButton-colorError:not(.MuiButton-contained)': {
          borderColor: 'transparent',
          bgcolor: 'transparent',
          color: '#ff778d',
          '&:hover': { borderColor: 'rgba(152, 40, 65, 0.5)', bgcolor: 'rgba(75, 24, 38, 0.44)', color: '#ff92a3' },
        },
      }}
    >
      <Box sx={{ minWidth: 120, flex: '1 1 220px', overflow: 'hidden' }}>{breadcrumbs}</Box>
      <Stack direction="row" alignItems="center" justifyContent="flex-end" spacing={0.4} flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
        {primaryActions}
        {pathOperations ? (
          <Box data-testid="assets-path-operations" sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
            {pathOperations}
          </Box>
        ) : null}
        {destructiveActions ? (
          <>
            <Divider orientation="vertical" flexItem sx={{ mx: 0.25, height: 19, alignSelf: 'center', borderColor: '#1f4467' }} />
            <Stack direction="row" alignItems="center" spacing={0.4} flexWrap="wrap" useFlexGap aria-label="Destructive path actions" data-testid="assets-path-destructive-actions">
              {destructiveActions(() => undefined)}
            </Stack>
          </>
        ) : null}
        {selectedCount > 0 ? (
          <Typography variant="caption" aria-label={`${selectedCount} selected`} sx={{ pl: 0.45, pr: 0.15, color: '#59718b', fontSize: '0.58rem', lineHeight: 1.2, whiteSpace: 'nowrap' }}>
            {selectedCount} selected
          </Typography>
        ) : null}
      </Stack>
    </Box>
  );
}
