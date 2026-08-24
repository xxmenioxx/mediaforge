import { Alert, Box, Chip, Stack, Typography } from '@mui/material';
import { avTimingViewModel } from '../utils/avTiming';

export function AVTimingSummary({ report }: { report: unknown }) {
  const timing = avTimingViewModel(report);
  if (!timing) return null;
  const severity = timing.status === 'mismatch' ? 'error' : timing.status === 'warning' || timing.status === 'unverified' ? 'warning' : 'success';
  return (
    <Stack spacing={1.25}>
      <Stack direction="row" spacing={1} alignItems="center"><Typography variant="h3">A/V timestamp alignment</Typography><Chip size="small" label={statusLabel(timing.status, timing.withinTolerance, timing.toleranceFrames)} color={severity} /></Stack>
      <Alert severity={severity}>Compares relative audio/video timestamps; it does not claim perceptual lip-sync validation.</Alert>
      {timing.tracks.map((track, index) => (
        <Box key={`${track.sourceAudioIndex}-${track.outputAudioIndex}-${index}`} sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(4, minmax(0, 1fr))' }, gap: 1, border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
          <TimingValue label="Audio track" value={`${track.sourceAudioIndex} → ${track.outputAudioIndex}`} />
          <TimingValue label="Source offset" value={formatMilliseconds(track.sourceOffsetMs)} />
          <TimingValue label="Output offset" value={formatMilliseconds(track.outputOffsetMs)} />
          <TimingValue label="Introduced" value={`${formatMilliseconds(track.introducedOffsetMs)} (${track.introducedOffsetFrames.toFixed(2)} frames)`} />
        </Box>
      ))}
      <Typography variant="caption" color="text.secondary">Tolerance: ±{timing.toleranceFrames.toFixed(0)} frame ({timing.toleranceMs.toFixed(1)} ms at {timing.frameRateUsed || 'unknown FPS'}) · Drift: {timing.driftStatus.replaceAll('_', ' ')}</Typography>
    </Stack>
  );
}

function TimingValue({ label, value }: { label: string; value: string }) {
  return <Box><Typography variant="caption" color="text.secondary">{label}</Typography><Typography variant="body2">{value}</Typography></Box>;
}

function formatMilliseconds(value: number) { return `${value >= 0 ? '+' : ''}${value.toFixed(1)} ms`; }

function statusLabel(status: string, withinTolerance: boolean, toleranceFrames: number) {
  if (status === 'validated' && withinTolerance) return `validated · within ${toleranceFrames.toFixed(0)}-frame tolerance`;
  return status.replaceAll('_', ' ');
}
