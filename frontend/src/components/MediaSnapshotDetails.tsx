import { Box, Card, CardContent, Chip, Grid, Stack, Table, TableBody, TableCell, TableHead, TableRow, Typography } from '@mui/material';
import type { MediaStreamInfo, ScanResult } from '../api/types';

type MediaSnapshotDetailsProps = {
  scan: ScanResult;
};

export function MediaSnapshotDetails({ scan }: MediaSnapshotDetailsProps) {
  return (
    <Stack spacing={2}>
      <Card variant="outlined" sx={{ bgcolor: 'transparent' }}>
        <CardContent>
          <Grid container spacing={2}>
            <SnapshotMetric label="Codec" value={scan.videoCodec || 'unknown'} />
            <SnapshotMetric label="Resolution" value={scan.width && scan.height ? `${scan.width}x${scan.height}` : 'unknown'} />
            <SnapshotMetric label="Duration" value={formatDuration(scan.duration)} />
            <SnapshotMetric label="Bitrate" value={formatBitrate(scan.bitrate)} />
            <SnapshotMetric label="Size" value={formatBytes(scan.sizeBytes)} />
            <SnapshotMetric label="Range" value={scan.hdr ? 'HDR' : 'SDR'} />
            <SnapshotMetric label="Chapters" value={`${scan.chapters}`} />
            <SnapshotMetric label="Container" value={scan.container || 'unknown'} wide />
          </Grid>
        </CardContent>
      </Card>

      <StreamSection title={`Video Tracks (${scan.videoStreams?.length ?? 0})`} streams={scan.videoStreams ?? []} emptyLabel="No video tracks found." />
      <StreamSection title={`Audio Tracks (${scan.audioTracks})`} streams={scan.audioStreams ?? []} emptyLabel="No audio tracks found." />
      <StreamSection title={`Subtitle Tracks (${scan.subtitleTracks})`} streams={scan.subtitleStreams ?? []} emptyLabel="No subtitle tracks found." />
    </Stack>
  );
}

function StreamSection({ title, streams, emptyLabel }: { title: string; streams: MediaStreamInfo[]; emptyLabel: string }) {
  return (
    <Box>
      <Typography variant="h3" sx={{ mb: 1 }}>
        {title}
      </Typography>
      {streams.length ? (
        <Box sx={{ overflowX: 'auto' }}>
          <Table size="small" sx={{ minWidth: 860 }}>
            <TableHead>
              <TableRow>
                <TableCell>Track</TableCell>
                <TableCell>Language</TableCell>
                <TableCell>Codec</TableCell>
                <TableCell>Details</TableCell>
                <TableCell>Flags</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {streams.map((stream) => (
                <TableRow key={`${stream.type}-${stream.index}`}>
                  <TableCell>
                    <Stack spacing={0.3}>
                      <Typography fontWeight={700}>#{stream.index}</Typography>
                      {stream.title ? (
                        <Typography color="text.secondary" variant="body2">
                          {stream.title}
                        </Typography>
                      ) : null}
                    </Stack>
                  </TableCell>
                  <TableCell>{languageLabel(stream.language)}</TableCell>
                  <TableCell>
                    <Stack spacing={0.3}>
                      <Typography>{stream.codec || 'unknown'}</Typography>
                      <Typography color="text.secondary" variant="body2">
                        {stream.codecLong || stream.profile || 'No codec details'}
                      </Typography>
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                      {streamDetails(stream).map((detail) => (
                        <Chip key={detail} label={detail} size="small" />
                      ))}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                      {streamFlags(stream).map((flag) => (
                        <Chip key={flag} label={flag} size="small" color={flag === 'forced' ? 'warning' : 'default'} />
                      ))}
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      ) : (
        <Typography color="text.secondary">{emptyLabel}</Typography>
      )}
    </Box>
  );
}

function SnapshotMetric({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <Grid size={{ xs: 6, sm: 4, md: wide ? 3 : 1.5 }}>
      <Typography color="text.secondary" variant="body2">
        {label}
      </Typography>
      <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
        {value}
      </Typography>
    </Grid>
  );
}

function streamDetails(stream: MediaStreamInfo) {
  const details = [
    stream.width && stream.height ? `${stream.width}x${stream.height}` : '',
    stream.hdr ? 'HDR' : '',
    stream.pixFmt ?? '',
    stream.colorPrimaries ? `primaries ${stream.colorPrimaries}` : '',
    stream.colorTransfer ? `transfer ${stream.colorTransfer}` : '',
    stream.avgFrameRate && stream.avgFrameRate !== '0/0' ? `${stream.avgFrameRate} fps` : '',
    stream.channels ? `${stream.channels} ch` : '',
    stream.channelLayout ?? '',
    stream.sampleRate ? `${stream.sampleRate} Hz` : '',
    stream.bitDepth ? `${stream.bitDepth}-bit` : '',
    stream.bitrate ? formatBitrate(stream.bitrate) : '',
    stream.duration ? formatDuration(stream.duration) : '',
  ];

  return details.filter(Boolean);
}

function streamFlags(stream: MediaStreamInfo) {
  return [
    stream.default ? 'default' : '',
    stream.forced ? 'forced' : '',
    stream.comment ? 'commentary' : '',
    stream.hearingImpaired ? 'hearing impaired' : '',
  ].filter(Boolean);
}

function languageLabel(language: string) {
  if (!language || language === 'und') {
    return 'Undetermined';
  }

  return language.toUpperCase();
}

function formatBytes(bytes: number) {
  if (!bytes) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`;
}

function formatDuration(seconds: number) {
  if (!seconds) {
    return 'unknown';
  }
  const total = Math.round(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const remainingSeconds = total % 60;
  return [hours, minutes, remainingSeconds].map((part) => String(part).padStart(2, '0')).join(':');
}

function formatBitrate(bitsPerSecond: number) {
  if (!bitsPerSecond) {
    return 'unknown';
  }
  return `${(bitsPerSecond / 1_000_000).toFixed(2)} Mbps`;
}
