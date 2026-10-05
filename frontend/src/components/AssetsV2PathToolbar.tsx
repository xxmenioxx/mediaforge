import { Box, Divider, Typography } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { CompactActionGroup } from './CompactActionGroup';
import { mvforgeTokens } from '../theme/theme';

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
  const tokens = useTheme().mvforge ?? mvforgeTokens;

  return (
    <Box
      component="section"
      aria-label={`${mode === 'raw' ? 'RAW' : 'Library'} path actions for ${path || 'current location'}`}
      data-testid="assets-path-toolbar"
      data-ui-density="compact"
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
        borderColor: tokens.colors.border.default,
        borderRadius: '6px',
        bgcolor: tokens.colors.workspace,
        '& .MuiAutocomplete-root': { flex: '0 0 auto' },
      }}
    >
      <Box sx={{ minWidth: 120, flex: '1 1 220px', overflow: 'hidden' }}>{breadcrumbs}</Box>
      <CompactActionGroup role="group" aria-label="Path actions" gap={0.4} sx={{ minWidth: 0 }}>
        {primaryActions}
        {pathOperations ? (
          <Box data-testid="assets-path-operations" sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
            {pathOperations}
          </Box>
        ) : null}
        {destructiveActions ? (
          <>
            <Divider orientation="vertical" flexItem sx={{ mx: 0.25, height: 19, alignSelf: 'center', borderColor: '#1f4467' }} />
            <CompactActionGroup role="group" aria-label="Destructive path actions" gap={0.4} data-testid="assets-path-destructive-actions">
              {destructiveActions(() => undefined)}
            </CompactActionGroup>
          </>
        ) : null}
        {selectedCount > 0 ? (
          <Typography variant="caption" aria-label={`${selectedCount} selected`} sx={{ pl: 0.45, pr: 0.15, color: tokens.colors.text.muted, fontSize: tokens.typography.supporting, lineHeight: 1.2, whiteSpace: 'nowrap' }}>
            {selectedCount} selected
          </Typography>
        ) : null}
      </CompactActionGroup>
    </Box>
  );
}
