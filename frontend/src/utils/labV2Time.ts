export function parseStartSeconds(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const numeric = Number(trimmed);
  if (Number.isFinite(numeric) && numeric >= 0) return numeric;
  const parts = trimmed.split(':').map(Number);
  if (parts.length < 2 || parts.length > 3 || parts.some((part) => !Number.isFinite(part) || part < 0)) return null;
  const [hours = 0, minutes = 0, seconds = 0] = parts.length === 3 ? parts : [0, parts[0], parts[1]];
  if (minutes >= 60 || seconds >= 60) return null;
  return hours * 3600 + minutes * 60 + seconds;
}

export function formatTimecode(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return [hours, minutes, remainder].map((part) => String(part).padStart(2, '0')).join(':');
}
