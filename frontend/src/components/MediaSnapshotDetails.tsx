import { Box, Card, CardContent, Checkbox, Chip, Grid, Stack, Switch, TextField, Typography } from '@mui/material';
import type { MediaStreamInfo, ScanResult, StreamMetadataOverride } from '../api/types';

type MediaSnapshotDetailsProps = {
  scan: ScanResult;
  streamControls?: StreamControls;
  metadataControls?: StreamMetadataControls;
  section?: 'all' | 'general' | 'tracks';
};

export type StreamControls = {
  video: StreamControlState;
  audio: StreamControlState;
  subtitle: StreamControlState;
};

type StreamControlState = {
  selected: number[];
  disabled?: boolean;
  onToggle: (index: number, keep: boolean) => void;
};

export type StreamMetadataControls = {
  video?: StreamMetadataControlState;
  audio: StreamMetadataControlState;
  subtitle: StreamMetadataControlState;
};

export function MediaTechnicalSnapshotSummary({ scan }: { scan: ScanResult }) {
  const video = scan.videoStreams?.[0];
  const frames = scan.frameStructureAnalysis;
  const frameRecommendation = scan.frameStructureRecommendation;
  const crop = scan.cropAnalysis;
  const interlace = scan.interlaceAnalysis;

  return (
    <Stack spacing={1.5}>
      <Grid container spacing={1.5}>
      <Grid size={{ xs: 12, md: 6 }}>
        <TechnicalFactGroup title="Video format" facts={[
          ['Codec', scan.videoCodec || video?.codec || 'Unknown'],
          ['Resolution', scan.width && scan.height ? `${scan.width}×${scan.height}` : 'Unknown'],
          ['Pixel format', video?.pixFmt || 'Unknown'],
          ['Bit depth', video?.bitDepth ? `${video.bitDepth}-bit` : video?.bitsPerRawSample || 'Unknown'],
          ['Frame rate', video?.avgFrameRate || video?.realFrameRate || 'Unknown'],
          ['Dynamic range', scan.hdr ? 'HDR' : 'SDR'],
          ['Video bitrate', formatBitrate(video?.bitrate || scan.bitrate)],
        ]} />
      </Grid>
      <Grid size={{ xs: 12, md: 6 }}>
        <TechnicalFactGroup title="Frame structure" facts={frames ? [
          ['Sample', `${frames.framesAnalyzed} frames`],
          ['Frame mix', `I ${frames.iFrames} · P ${frames.pFrames} · B ${frames.bFrames}`],
          ['B-frame share', `${(frames.bFrameRatio * 100).toFixed(1)}%`],
          ['Longest B run', String(frames.maxConsecutiveBFrames)],
          ['Keyframes', String(frames.keyFrames)],
          ['Average GOP', frames.averageGopLength > 0 ? frames.averageGopLength.toFixed(1) : 'Not enough keyframes'],
          ['Complete GOPs', String(frames.completeGops ?? 0)],
          ['Sampling', frames.windowCount ? `${frames.sampledSeconds?.toFixed(0) ?? 0}s across ${frames.windowCount} regions` : `${frames.framesAnalyzed} frames`],
          ['Coverage', frames.coverageRatio !== undefined ? `${(frames.coverageRatio * 100).toFixed(1)}% · ${frames.confidence || 'unknown'} confidence` : 'Legacy sample'],
          ['Variability', frames.variability || 'Unknown'],
          ['Recommended GOP', frameRecommendation?.byMode?.balanced?.targetGopFrames ? `${frameRecommendation.byMode.balanced.targetGopFrames} frames · balanced` : 'Unavailable'],
          ['Recommended B depth', frameRecommendation?.recommendedMaxBFrames !== undefined ? String(frameRecommendation.recommendedMaxBFrames) : 'Unavailable'],
        ] : [['Status', 'Run Process Asset to refresh frame analysis']]} />
      </Grid>
      <Grid size={{ xs: 12, md: 6 }}>
        <TechnicalFactGroup title="Motion & geometry" facts={[
          ['Scan type', interlaceLabel(scan)],
          ['Field order', interlace?.detectedFieldOrder || interlace?.fieldOrder || 'Unknown'],
          ['Field-order match', interlace?.fieldOrderMismatch ? 'Mismatch · review' : 'No mismatch detected'],
          ['Crop status', crop?.status || 'Unknown'],
          ['Recommended crop', crop?.recommendedCrop || 'None'],
          ['Display aspect ratio', video?.displayAspectRatio || 'Unknown'],
          ['Sample aspect ratio', video?.sampleAspectRatio || 'Unknown'],
        ]} />
      </Grid>
      <Grid size={{ xs: 12, md: 6 }}>
        <TechnicalFactGroup title="Container & tracks" facts={[
          ['Container', friendlyContainer(scan.container)],
          ['Duration', formatDuration(scan.duration)],
          ['File size', formatBytes(scan.sizeBytes)],
          ['Audio tracks', String(scan.audioTracks)],
          ['Subtitle tracks', String(scan.subtitleTracks)],
          ['Chapters', String(scan.chapters)],
        ]} />
      </Grid>
      </Grid>
      {frames?.windows?.length ? (
        <Card variant="outlined">
          <CardContent>
            <Stack spacing={1.25}>
              <Typography fontWeight={700}>Frame structure regions</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                {frames.windows.map((window, index) => (
                  <Chip key={`${window.position}-${window.startSeconds}`} size="small" variant="outlined" label={`Region ${index + 1} · ${Math.round(window.position * 100)}% · I ${window.analysis.iFrames} / P ${window.analysis.pFrames} / B ${window.analysis.bFrames} · GOP ${window.analysis.averageGopLength > 0 ? window.analysis.averageGopLength.toFixed(1) : 'n/a'} · B-run ${window.analysis.maxConsecutiveBFrames}`} />
                ))}
              </Stack>
            </Stack>
          </CardContent>
        </Card>
      ) : null}
    </Stack>
  );
}

function TechnicalFactGroup({ title, facts }: { title: string; facts: string[][] }) {
  return (
    <Card variant="outlined" sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={1.25}>
          <Typography fontWeight={700}>{title}</Typography>
          <Grid container spacing={1}>
            {facts.map(([label, value]) => (
              <Grid key={label} size={{ xs: 12, sm: 6 }}>
                <Typography variant="caption" color="text.secondary">{label}</Typography>
                <Typography variant="body2" fontWeight={600}>{value}</Typography>
              </Grid>
            ))}
          </Grid>
        </Stack>
      </CardContent>
    </Card>
  );
}

type StreamMetadataControlState = {
  values: Record<string, StreamMetadataOverride>;
  disabled?: boolean;
  onChange: (index: number, patch: StreamMetadataOverride) => void;
};

export function MediaSnapshotDetails({ scan, streamControls, metadataControls, section = 'all' }: MediaSnapshotDetailsProps) {
  return (
    <Stack spacing={1.25}>
      {section !== 'tracks' ? <MediaTechnicalSnapshotSummary scan={scan} /> : null}
      {section !== 'tracks' && streamControls ? (
        <Stack direction="row"><SnapshotChip label="Selection savings" value={selectionSavingsLabel(scan, streamControls)} /></Stack>
      ) : null}

      {section !== 'general' ? <><StreamSection
        title={`Video (${scan.videoStreams?.length ?? 0})`}
        streams={scan.videoStreams ?? []}
        emptyLabel="No video tracks found."
        control={streamControls?.video}
        metadataControl={metadataControls?.video}
      />
      <StreamSection
        title={`Audio (${scan.audioTracks})`}
        streams={scan.audioStreams ?? []}
        emptyLabel="No audio tracks found."
        control={streamControls?.audio}
        metadataControl={metadataControls?.audio}
      />
      <StreamSection
        title={`Subtitles (${scan.subtitleTracks})`}
        streams={scan.subtitleStreams ?? []}
        emptyLabel="No subtitle tracks found."
        control={streamControls?.subtitle}
        metadataControl={metadataControls?.subtitle}
      />
      </> : null}
    </Stack>
  );
}

function interlaceLabel(scan: ScanResult) {
  const status = scan.interlaceAnalysis?.status ?? 'unknown';
  const fieldOrder = scan.interlaceAnalysis?.fieldOrder;
  switch (status) {
    case 'interlaced': return `Interlaced${fieldOrder && fieldOrder !== 'unknown' ? ` · ${fieldOrder.toUpperCase()}` : ''}`;
    case 'mixed': return 'Mixed · review';
    case 'telecine_suspected': return `Telecine suspected${fieldOrder && fieldOrder !== 'unknown' ? ` · ${fieldOrder.toUpperCase()}` : ''}`;
    case 'progressive': return 'Progressive';
    default: return 'Unknown';
  }
}

function StreamSection({
  title,
  streams,
  emptyLabel,
  control,
  metadataControl,
}: {
  title: string;
  streams: MediaStreamInfo[];
  emptyLabel: string;
  control?: StreamControlState;
  metadataControl?: StreamMetadataControlState;
}) {
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 1.25, py: 0.9, bgcolor: 'rgba(255,255,255,0.025)' }}>
        <Typography fontWeight={700}>{title}</Typography>
        {control ? <Typography color="text.secondary" variant="body2">{control.selected.length}/{streams.length} kept · {removedSavingsLabel(streams, control.selected)}</Typography> : null}
      </Stack>
      {streams.length ? (
        <Stack>
          {streams.map((stream) => (
            <StreamRow key={`${stream.type}-${stream.index}`} stream={stream} control={control} metadataControl={metadataControl} />
          ))}
        </Stack>
      ) : (
        <Typography color="text.secondary" sx={{ px: 1.25, py: 1 }}>
          {emptyLabel}
        </Typography>
      )}
    </Box>
  );
}

function StreamRow({
  stream,
  control,
  metadataControl,
}: {
  stream: MediaStreamInfo;
  control?: StreamControlState;
  metadataControl?: StreamMetadataControlState;
}) {
  const metadata = metadataControl?.values[String(stream.index)] ?? {};
  const canEditMetadata = Boolean(metadataControl);

  return (
    <Box sx={{ borderTop: 1, borderColor: 'divider', px: 1, py: 0.85 }}>
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'stretch', md: 'center' }}>
        <Stack direction="row" spacing={1} alignItems="center" sx={{ minWidth: { md: 300 }, flex: 1 }}>
          {control ? (
            <Checkbox
              checked={control.selected.includes(stream.index)}
              onChange={(event) => control.onToggle(stream.index, event.target.checked)}
              disabled={control.disabled}
              size="small"
              inputProps={{ 'aria-label': `Keep ${stream.type} track ${stream.index}` }}
            />
          ) : null}
          <Stack spacing={0.35} sx={{ minWidth: 0 }}>
            <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap>
              <Chip label={`#${stream.index}`} size="small" />
              <Typography fontWeight={700}>{languageLabel(stream.language)}</Typography>
              <Typography color="text.secondary">{stream.codec || 'unknown'}</Typography>
            </Stack>
            <Typography color="text.secondary" variant="body2" noWrap>
              {stream.title || stream.codecLong || stream.profile || 'No title'}
            </Typography>
          </Stack>
        </Stack>

        <Stack direction="row" spacing={0.6} flexWrap="wrap" useFlexGap sx={{ flex: 1.3 }}>
          {streamDetails(stream).slice(0, 5).map((detail) => (
            <Chip key={detail} label={detail} size="small" />
          ))}
          {streamFlags(stream).map((flag) => (
            <Chip key={flag} label={flag} size="small" color={flag === 'forced' ? 'warning' : 'default'} />
          ))}
        </Stack>

        {canEditMetadata ? (
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.75} sx={{ minWidth: { md: 420 } }}>
            <TextField
              label="Lang"
              value={metadata.language ?? ''}
              onChange={(event) => metadataControl?.onChange(stream.index, { language: event.target.value })}
              placeholder={stream.language && stream.language !== 'und' ? stream.language : 'eng'}
              disabled={metadataControl?.disabled}
              size="small"
              sx={{ width: { sm: 92 } }}
            />
            <TextField
              label="Title"
              value={metadata.title ?? ''}
              onChange={(event) => metadataControl?.onChange(stream.index, { title: event.target.value })}
              placeholder={stream.title || 'Track title'}
              disabled={metadataControl?.disabled}
              size="small"
              sx={{ flex: 1 }}
            />
            <MetadataSwitch
              label="Default"
              value={metadata.default}
              fallback={stream.default}
              disabled={metadataControl?.disabled}
              onChange={(value) => metadataControl?.onChange(stream.index, { default: value })}
            />
            <MetadataSwitch
              label="Forced"
              value={metadata.forced}
              fallback={stream.forced}
              disabled={metadataControl?.disabled}
              onChange={(value) => metadataControl?.onChange(stream.index, { forced: value })}
            />
          </Stack>
        ) : null}
      </Stack>
    </Box>
  );
}

function MetadataSwitch({
  label,
  value,
  fallback,
  disabled,
  onChange,
}: {
  label: string;
  value?: boolean;
  fallback?: boolean;
  disabled?: boolean;
  onChange: (value: boolean | undefined) => void;
}) {
  const checked = value ?? fallback ?? false;
  return (
    <Stack direction="row" alignItems="center" spacing={0.25} sx={{ minWidth: 92 }}>
      <Switch checked={checked} onChange={(event) => onChange(event.target.checked === fallback ? undefined : event.target.checked)} disabled={disabled} size="small" />
      <Typography variant="body2">{label}</Typography>
    </Stack>
  );
}

function SnapshotChip({ label, value }: { label: string; value: string }) {
  return (
    <Chip
      label={
        <Stack direction="row" spacing={0.5}>
          <Typography component="span" color="text.secondary" variant="body2">
            {label}
          </Typography>
          <Typography component="span" fontWeight={700} variant="body2">
            {value}
          </Typography>
        </Stack>
      }
      sx={{ height: 30, maxWidth: '100%' }}
    />
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
    stream.sizeBytes ? `${stream.sizeEstimated ? '≈ ' : ''}${formatBytes(stream.sizeBytes)}` : '',
    stream.duration ? formatDuration(stream.duration) : '',
  ];

  return details.filter(Boolean);
}

function streamRemovalSize(stream: MediaStreamInfo, duration: number) {
  if (stream.sizeBytes && stream.sizeBytes > 0) return { bytes: stream.sizeBytes, estimated: Boolean(stream.sizeEstimated) };
  if (stream.bitrate && duration > 0) return { bytes: stream.bitrate * duration / 8, estimated: true };
  return { bytes: 0, estimated: true };
}

function removedSavingsLabel(streams: MediaStreamInfo[], selected: number[]) {
  const removed = streams.filter((stream) => !selected.includes(stream.index));
  if (!removed.length) return 'no tracks removed';
  const known = removed.filter((stream) => Boolean(stream.sizeBytes));
  const bytes = known.reduce((total, stream) => total + (stream.sizeBytes ?? 0), 0);
  if (!bytes) return 'saving unknown';
  return `save ${known.some((stream) => stream.sizeEstimated) || known.length !== removed.length ? '≈ ' : ''}${formatBytes(bytes)}`;
}

function selectionSavingsLabel(scan: ScanResult, controls: StreamControls) {
  const groups = [
    { streams: scan.videoStreams ?? [], selected: controls.video.selected },
    { streams: scan.audioStreams ?? [], selected: controls.audio.selected },
    { streams: scan.subtitleStreams ?? [], selected: controls.subtitle.selected },
  ];
  const removed = groups.flatMap(({ streams, selected }) => streams.filter((stream) => !selected.includes(stream.index)));
  if (!removed.length) return '0 B';
  const values = removed.map((stream) => streamRemovalSize(stream, scan.duration));
  const bytes = values.reduce((total, value) => total + value.bytes, 0);
  if (!bytes) return 'unknown';
  return `${values.some((value) => value.estimated) ? '≈ ' : ''}${formatBytes(bytes)} (${Math.min(100, bytes / Math.max(scan.sizeBytes, 1) * 100).toFixed(1)}%)`;
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

function friendlyContainer(container: string) {
  if (!container) {
    return 'unknown';
  }
  return container
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => (part === 'matroska' ? 'mkv' : part))
    .join(', ');
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
