import { Box } from '@mui/material';
import type { ReactNode } from 'react';

export const LAB_COMPARISON_VIEWPORT_POLICY = '16:9-contain';

export const labComparisonViewportSx = {
  position: 'relative',
  width: '100%',
  aspectRatio: '16 / 9',
  overflow: 'hidden',
  bgcolor: 'black',
  borderRadius: 1,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

export const labComparisonImageSx = {
  display: 'block',
  width: '100%',
  height: '100%',
  objectFit: 'contain',
  objectPosition: 'center',
};

export const labComparisonLayerSx = {
  ...labComparisonImageSx,
  position: 'absolute',
  inset: 0,
};

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
