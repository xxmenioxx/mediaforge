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
