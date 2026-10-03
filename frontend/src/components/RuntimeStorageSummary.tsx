import { Box, Card, CardContent, LinearProgress, Stack, Tooltip, Typography } from '@mui/material';
import StorageOutlinedIcon from '@mui/icons-material/StorageOutlined';
import type { RuntimeState } from '../api/types';

type RuntimeDisk = {
  type?: string;
  totalBytes?: number;
  availableBytes?: number;
};

function diskValue(value: unknown): RuntimeDisk {
  return value && typeof value === 'object' ? value as RuntimeDisk : {};
}

function byteValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : undefined;
}

function formatBytes(bytes: number | undefined): string {
  if (bytes === undefined) return 'Unavailable';
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** unit;
  return `${value.toFixed(unit >= 3 ? 1 : 0)} ${units[unit]}`;
}

export function RuntimeStorageSummary({ snapshot, loading, error, compact = false }: {
  snapshot?: RuntimeState;
  loading: boolean;
  error: boolean;
  compact?: boolean;
}) {
  const disks = snapshot ? Object.entries(snapshot.disks ?? {}) : [];
  const content = (
    <Stack spacing={compact ? 0.65 : 0.75}>
      {loading ? <Typography variant="caption" color="text.secondary">Loading storage status…</Typography> : null}
      {error ? <Typography variant="caption" color="warning.main">Storage status is unavailable.</Typography> : null}
      {!loading && !error && !snapshot ? <Typography variant="caption" color="text.secondary">Storage status is unavailable.</Typography> : null}
      {!loading && !error && snapshot && !disks.length ? <Typography variant="caption" color="text.secondary">No controlled disks detected.</Typography> : null}
      {!loading && !error ? disks.map(([role, raw]) => {
        const disk = diskValue(raw);
        const totalBytes = byteValue(disk.totalBytes);
        const availableBytes = byteValue(disk.availableBytes);
        const usedBytes = totalBytes !== undefined && totalBytes > 0 && availableBytes !== undefined && availableBytes <= totalBytes
          ? totalBytes - availableBytes
          : undefined;
        const usedPercent = usedBytes !== undefined && totalBytes !== undefined
          ? Math.round((usedBytes / totalBytes) * 100)
          : undefined;
        return (
          <Box key={role} sx={{ minWidth: 0 }}>
            <Stack direction="row" alignItems="baseline" justifyContent="space-between" spacing={0.75}>
              <Tooltip title={role}>
                <Typography variant="caption" color="text.secondary" noWrap sx={{ minWidth: 0, textTransform: 'capitalize' }}>
                  {role.replaceAll('_', ' ')}{disk.type ? ` · ${disk.type}` : ''}
                </Typography>
              </Tooltip>
              <Typography variant="caption" fontWeight={700} noWrap>
                {usedPercent === undefined ? `${formatBytes(availableBytes)} free` : `${usedPercent}% used`}
              </Typography>
            </Stack>
            {usedBytes !== undefined && usedPercent !== undefined ? (
              <>
                <LinearProgress
                  aria-label={`${role} storage used`}
                  variant="determinate"
                  value={usedPercent}
                  sx={{ height: 4, borderRadius: 99, my: 0.35, bgcolor: 'rgba(63, 88, 119, 0.46)', '& .MuiLinearProgress-bar': { background: 'linear-gradient(90deg, #6556ff, #3db9ff)' } }}
                />
                <Typography variant="caption" color="text.secondary" sx={{ fontSize: compact ? '0.58rem' : undefined }}>
                  {formatBytes(usedBytes)} of {formatBytes(totalBytes)} used · {formatBytes(availableBytes)} free
                </Typography>
              </>
            ) : availableBytes === undefined ? (
              <Typography variant="caption" color="text.secondary" sx={{ fontSize: compact ? '0.58rem' : undefined }}>Capacity unavailable</Typography>
            ) : null}
          </Box>
        );
      }) : null}
    </Stack>
  );

  if (!compact) return content;

  return (
    <Card
      component="section"
      aria-label="Storage summary"
      data-testid="assets-storage-footer"
      variant="outlined"
      sx={{ flexShrink: 0, borderColor: 'rgba(56, 81, 111, 0.68)', bgcolor: 'rgba(13, 29, 47, 0.88)', borderRadius: 1, mx: 0.1 }}
    >
      <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
        <Stack spacing={0.65}>
          <Stack direction="row" alignItems="center" justifyContent="space-between">
            <Stack direction="row" alignItems="center" spacing={0.65}>
              <StorageOutlinedIcon sx={{ fontSize: 15, color: '#9cb0c8' }} />
              <Typography variant="caption" fontWeight={700} sx={{ color: '#9cb0c8', letterSpacing: '0.03em' }}>Storage</Typography>
            </Stack>
          </Stack>
          <Box sx={{ maxHeight: 132, overflowY: disks.length > 2 ? 'auto' : 'visible', pr: disks.length > 2 ? 0.35 : 0 }}>
            {content}
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}
