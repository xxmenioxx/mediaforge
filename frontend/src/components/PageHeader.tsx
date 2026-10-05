import { Box, Stack, Typography } from '@mui/material';
import type { Theme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { getMVForgeTokens } from '../theme/theme';

type PageHeaderProps = {
  title: string;
  eyebrow?: string;
  children?: ReactNode;
  actions?: ReactNode;
  metadata?: ReactNode;
};

const pageHorizontalGutter = { xs: 2, md: 4 } as const;

export function PageHeader({ title, eyebrow, children, actions, metadata }: PageHeaderProps) {
  return (
    <Box component="header" sx={{ px: pageHorizontalGutter, pt: { xs: 3, md: 4 }, pb: 2 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'flex-start' }} justifyContent="space-between" spacing={1.5}>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          {eyebrow ? (
            <Typography
              color="secondary.main"
              fontWeight={700}
              sx={{ fontSize: (theme: Theme) => getMVForgeTokens(theme).typography.supporting, letterSpacing: '0.025em' }}
            >
              {eyebrow}
            </Typography>
          ) : null}
          <Typography
            component="h1"
            variant="h1"
            sx={{
              mt: eyebrow ? 0.5 : 0,
              color: (theme: Theme) => getMVForgeTokens(theme).colors.text.primary,
              fontSize: (theme: Theme) => getMVForgeTokens(theme).typography.pageHeading,
              lineHeight: 1.2,
            }}
          >
            {title}
          </Typography>
          {children ? (
            <Box
              sx={{
                color: 'text.secondary',
                fontSize: (theme: Theme) => getMVForgeTokens(theme).typography.supporting,
                '& .MuiTypography-root': { fontSize: (theme: Theme) => getMVForgeTokens(theme).typography.supporting },
              }}
            >
              {children}
            </Box>
          ) : null}
          {metadata ? (
            <Stack
              direction="row"
              spacing={1}
              useFlexGap
              flexWrap="wrap"
              sx={{
                mt: 1,
                alignItems: 'center',
                color: (theme: Theme) => getMVForgeTokens(theme).colors.text.muted,
                fontSize: (theme: Theme) => getMVForgeTokens(theme).typography.supporting,
              }}
            >
              {metadata}
            </Stack>
          ) : null}
        </Box>
        {actions ? (
          <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ alignItems: 'center', flexShrink: 0, pt: { xs: 0, sm: 0.5 } }}>
            {actions}
          </Stack>
        ) : null}
      </Stack>
    </Box>
  );
}

export function PageContent({ children }: { children: ReactNode }) {
  return (
    <Box data-testid="page-content-workspace" sx={{ width: '100%', minWidth: 0, bgcolor: (theme: Theme) => getMVForgeTokens(theme).colors.workspace }}>
      <Box data-testid="page-content-inner" sx={{ width: '100%', minWidth: 0, px: pageHorizontalGutter, pb: 4 }}>
        {children}
      </Box>
    </Box>
  );
}
