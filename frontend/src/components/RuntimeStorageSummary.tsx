import { Box, Card, CardContent, LinearProgress, Stack, Tooltip, Typography } from '@mui/material';
import StorageOutlinedIcon from '@mui/icons-material/StorageOutlined';
import type { RuntimeState } from '../api/types';

type RuntimeFilesystem = {
  volumeId?: string;
  measuredPath?: string;
  path?: string;
  paths?: string[];
  probePaths?: string[];
  type?: string;
  totalBytes?: number;
  availableBytes?: number;
};

type RuntimeDestination = {
  path?: string;
  measuredPath?: string;
  accessible?: boolean;
  error?: string;
};

type RuntimeDisk = RuntimeFilesystem & {
  filesystems?: RuntimeFilesystem[];
  destinations?: RuntimeDestination[];
};

type StorageVolumeRow = {
  key: string;
  roles: Set<string>;
  paths: Set<string>;
  volumeId?: string;
  totalBytes?: number;
  availableBytes?: number;
  type?: string;
};

type UnavailableDestination = {
  role: string;
  path: string;
  error?: string;
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

function displayRole(role: string) {
  return role.replaceAll('_', ' ').replace(/\b\w/g, (character) => character.toUpperCase());
}

function storageRows(snapshot?: RuntimeState) {
  const disks = Object.entries(snapshot?.disks ?? {}).map(([role, raw]) => [role, diskValue(raw)] as const);
  const volumes = new Map<string, StorageVolumeRow>();
  const unavailable: UnavailableDestination[] = [];

  for (const [role, disk] of disks) {
    const filesystems = Array.isArray(disk.filesystems) && disk.filesystems.length
      ? disk.filesystems
      : disk.totalBytes !== undefined || disk.availableBytes !== undefined || disk.measuredPath || disk.path
        ? [{
            volumeId: disk.volumeId,
            measuredPath: disk.measuredPath || disk.path,
            paths: Array.isArray(disk.probePaths) ? disk.probePaths : disk.path ? [disk.path] : [],
            type: disk.type,
            totalBytes: disk.totalBytes,
            availableBytes: disk.availableBytes,
          }]
        : [];

    for (const filesystem of filesystems) {
      const paths = Array.isArray(filesystem.paths) ? filesystem.paths.filter((path): path is string => typeof path === 'string' && path.length > 0) : [];
      const measuredPath = typeof filesystem.measuredPath === 'string' ? filesystem.measuredPath : '';
      if (measuredPath && !paths.includes(measuredPath)) paths.unshift(measuredPath);
      const volumeId = typeof filesystem.volumeId === 'string' ? filesystem.volumeId : '';
      const key = volumeId ? `volume:${volumeId}` : `path:${measuredPath || paths[0] || role}`;
      const row = volumes.get(key) ?? {
        key,
        roles: new Set<string>(),
        paths: new Set<string>(),
        volumeId: volumeId || undefined,
        totalBytes: byteValue(filesystem.totalBytes),
        availableBytes: byteValue(filesystem.availableBytes),
        type: filesystem.type,
      };
      row.roles.add(role);
      paths.forEach((path) => row.paths.add(path));
      volumes.set(key, row);
    }

    for (const destination of Array.isArray(disk.destinations) ? disk.destinations : []) {
      if (destination.accessible === false) {
        unavailable.push({ role, path: destination.path || 'Configured destination', error: destination.error });
      }
    }

    if (!filesystems.length && !disk.destinations?.length) {
      unavailable.push({ role, path: disk.path || 'Configured destination' });
    }
  }

  return { volumes: [...volumes.values()], unavailable };
}

export function RuntimeStorageSummary({ snapshot, loading, error, compact = false }: {
  snapshot?: RuntimeState;
  loading: boolean;
  error: boolean;
  compact?: boolean;
}) {
  const disks = snapshot ? Object.entries(snapshot.disks ?? {}) : [];
  const { volumes, unavailable } = storageRows(snapshot);
  const content = (
    <Stack spacing={compact ? 0.75 : 0.85}>
      {loading ? <Typography variant="caption" color="text.secondary">Loading storage status…</Typography> : null}
      {error ? <Typography variant="caption" color="warning.main">Storage status is unavailable.</Typography> : null}
      {!loading && !error && !snapshot ? <Typography variant="caption" color="text.secondary">Storage status is unavailable.</Typography> : null}
      {!loading && !error && snapshot && !disks.length ? <Typography variant="caption" color="text.secondary">No controlled destinations detected.</Typography> : null}
      {!loading && !error ? volumes.map((volume) => {
        const totalBytes = byteValue(volume.totalBytes);
        const availableBytes = byteValue(volume.availableBytes);
        const usedBytes = totalBytes !== undefined && availableBytes !== undefined && availableBytes <= totalBytes
          ? totalBytes - availableBytes
          : undefined;
        const usedPercent = usedBytes !== undefined && totalBytes !== undefined && totalBytes > 0
          ? Math.round((usedBytes / totalBytes) * 100)
          : undefined;
        const roleLabel = [...volume.roles].map(displayRole).sort().join(' · ');
        const pathLabel = [...volume.paths].join(', ') || 'Path unavailable';
        return (
          <Box key={volume.key} data-testid="storage-filesystem" sx={{ minWidth: 0 }}>
            <Stack direction="row" alignItems="baseline" justifyContent="space-between" spacing={0.5}>
              <Typography variant="caption" fontWeight={700} sx={{ minWidth: 0, color: '#b5c6da', fontSize: '0.64rem' }}>
                {roleLabel || 'Storage'}
              </Typography>
              {usedPercent !== undefined ? (
                <Typography variant="caption" fontWeight={700} sx={{ color: '#d4dfec', fontSize: '0.61rem', flexShrink: 0 }}>{usedPercent}% used</Typography>
              ) : null}
            </Stack>
            <Tooltip title={`Measured filesystem path: ${pathLabel}${volume.volumeId ? ` · volume ${volume.volumeId}` : ''}`} describeChild>
              <Typography variant="caption" sx={{ display: 'block', color: '#8298b1', fontSize: '0.57rem', overflowWrap: 'anywhere', lineHeight: 1.25 }}>
                {pathLabel}{volume.volumeId ? ` · volume ${volume.volumeId}` : ''}
              </Typography>
            </Tooltip>
            {usedBytes !== undefined && totalBytes !== undefined ? (
              <>
                <LinearProgress
                  aria-label={`${roleLabel || 'Storage'} filesystem capacity used`}
                  variant="determinate"
                  value={usedPercent ?? 0}
                  sx={{ height: 3, borderRadius: 99, my: 0.3, bgcolor: 'rgba(63, 88, 119, 0.46)', '& .MuiLinearProgress-bar': { background: 'linear-gradient(90deg, #5865d8, #38b9cb)' } }}
                />
                <Typography variant="caption" sx={{ display: 'block', color: '#93a7bd', fontSize: '0.59rem', lineHeight: 1.25, overflowWrap: 'anywhere' }}>
                  {formatBytes(usedBytes)} of {formatBytes(totalBytes)} used · {formatBytes(availableBytes)} free
                </Typography>
              </>
            ) : (
              <Typography variant="caption" sx={{ display: 'block', color: '#8298b1', fontSize: '0.59rem' }}>Capacity unavailable</Typography>
            )}
          </Box>
        );
      }) : null}
      {!loading && !error ? unavailable.map((destination, index) => (
        <Tooltip key={`${destination.role}:${destination.path}:${index}`} title={destination.error || `Destination unavailable: ${destination.path}`} describeChild>
          <Box data-testid="storage-destination-unavailable" sx={{ minWidth: 0 }}>
            <Typography variant="caption" sx={{ display: 'block', color: '#a9b8cc', fontSize: '0.62rem', overflowWrap: 'anywhere' }}>
              {displayRole(destination.role)} · {destination.path}
            </Typography>
            <Typography variant="caption" sx={{ color: '#8298b1', fontSize: '0.59rem' }}>Unavailable</Typography>
          </Box>
        </Tooltip>
      )) : null}
    </Stack>
  );

  if (!compact) return content;

  return (
    <Card
      component="section"
      aria-label="Storage summary"
      data-testid="assets-storage-footer"
      variant="outlined"
      sx={{ flexShrink: 0, borderColor: 'rgba(42, 69, 97, 0.78)', bgcolor: '#081a2a', borderRadius: 1, mx: 0.1 }}
    >
      <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
        <Stack spacing={0.6}>
          <Stack direction="row" alignItems="center" justifyContent="space-between">
            <Stack direction="row" alignItems="center" spacing={0.65}>
              <StorageOutlinedIcon sx={{ fontSize: 14, color: '#8ca5bf' }} />
              <Typography variant="caption" fontWeight={700} sx={{ color: '#b0c1d4', letterSpacing: '0.02em', fontSize: '0.66rem' }}>Storage</Typography>
            </Stack>
            <Typography variant="caption" sx={{ color: '#7188a1', fontSize: '0.56rem' }}>Filesystem capacity</Typography>
          </Stack>
          <Box sx={{ maxHeight: 144, overflowY: volumes.length + unavailable.length > 3 ? 'auto' : 'visible', pr: volumes.length + unavailable.length > 3 ? 0.35 : 0 }}>
            {content}
          </Box>
        </Stack>
      </CardContent>
    </Card>
  );
}
