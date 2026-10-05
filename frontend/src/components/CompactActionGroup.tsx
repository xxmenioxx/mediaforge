import Stack, { type StackProps } from '@mui/material/Stack';
import type { ReactNode } from 'react';

export function CompactActionGroup({ children, ...props }: StackProps & { children?: ReactNode }) {
  return (
    <Stack
      direction="row"
      alignItems="center"
      flexWrap="wrap"
      useFlexGap
      gap={0.5}
      data-ui-density="compact"
      {...props}
    >
      {children}
    </Stack>
  );
}
