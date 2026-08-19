export function formatEstimatedByteRange(
  minimum: number,
  maximum: number,
  formatBytes: (value: number) => string,
) {
  if (minimum <= 0 && maximum <= 0) {
    return '';
  }

  if (minimum === maximum) {
    return `≈${formatBytes(maximum)}`;
  }

  return `${formatBytes(minimum)}–${formatBytes(maximum)}`;
}