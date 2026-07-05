import { Box, Typography } from '@mui/material';
import type { ReactNode } from 'react';

type PageHeaderProps = {
  title: string;
  eyebrow?: string;
  children?: ReactNode;
};

export function PageHeader({ title, eyebrow, children }: PageHeaderProps) {
  return (
    <Box sx={{ px: { xs: 2, md: 4 }, pt: 4, pb: 2 }}>
      {eyebrow ? (
        <Typography color="primary.main" fontWeight={700} variant="body2">
          {eyebrow}
        </Typography>
      ) : null}
      <Typography variant="h1" sx={{ mt: 0.5 }}>
        {title}
      </Typography>
      {children}
    </Box>
  );
}

