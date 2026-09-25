import { Box } from '@mui/material';
import type { ReactNode } from 'react';
import { LAB_COMPARISON_VIEWPORT_POLICY, labComparisonViewportSx } from '../utils/labComparisonViewport';

export function LabComparisonViewport({ children }: { children: ReactNode }) {
  return (
    <Box
      data-testid="lab-comparison-viewport"
      data-lab-comparison-viewport={LAB_COMPARISON_VIEWPORT_POLICY}
      sx={labComparisonViewportSx}
    >
      {children}
    </Box>
  );
}
