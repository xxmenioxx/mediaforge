import { Card, CardActionArea, CardContent, Stack, Typography } from '@mui/material';
import type { ReactNode } from 'react';
import { Link as RouterLink } from 'react-router-dom';

type MetricCardProps = {
  label: string;
  value: string | number;
  icon: ReactNode;
  to?: string;
};

export function MetricCard({ label, value, icon, to }: MetricCardProps) {
  const content = (
    <CardContent sx={{ py: 1.5, px: 2, '&:last-child': { pb: 1.5 } }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={2}>
        <Stack spacing={0.25} sx={{ minWidth: 0 }}>
          <Typography color="text.secondary" variant="body2">
            {label}
          </Typography>
          <Typography variant="h2">{value}</Typography>
        </Stack>
        <Stack
          alignItems="center"
          justifyContent="center"
          sx={{
            width: 44,
            height: 44,
            borderRadius: 1,
            bgcolor: 'rgba(79,179,255,0.12)',
            color: 'primary.main',
            flexShrink: 0,
          }}
        >
          {icon}
        </Stack>
      </Stack>
    </CardContent>
  );

  return (
    <Card sx={{ height: '100%' }}>
      {to ? (
        <CardActionArea component={RouterLink} to={to} sx={{ height: '100%' }}>
          {content}
        </CardActionArea>
      ) : (
        content
      )}
    </Card>
  );
}
