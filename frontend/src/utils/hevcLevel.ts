export function formatHEVCLevel(level?: number) {
  if (!level || level <= 0) return 'Unknown';
  const major = Math.floor(level / 30);
  const remainder = level % 30;
  return `${major}.${Math.round(remainder / 3)}`;
}
