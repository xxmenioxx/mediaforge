import {
  Alert,
  Box,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  Grid,
  Stack,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';

type JobDetailsDialogProps = {
  job: QueueJob | null;
  onClose: () => void;
};

export function JobDetailsDialog({ job, onClose }: JobDetailsDialogProps) {
  const [tab, setTab] = useState(0);
  const artifacts = useQuery({
    queryKey: ['jobArtifacts', job?.id],
    queryFn: () => api.jobArtifacts(job?.id ?? 0),
    enabled: Boolean(job),
  });

  if (!job) {
    return null;
  }

  const asIs = artifacts.data?.asIs;
  const result = artifacts.data?.result;
  const sourceProbe = objectValue(asIs, 'sourceProbe');
  const outputProbe = objectValue(result, 'outputProbe');
  const resultPayload = objectValue(result, 'result');
  const profile = objectValue(asIs, 'profile');

  return (
    <Dialog open={Boolean(job)} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <Typography variant="h3">Job #{job.id}</Typography>
          <Chip label={job.status} color={statusColor(job.status)} size="small" />
          <Chip label={`${job.progress}%`} color="primary" size="small" variant="outlined" />
          {job.workerName ? <Chip label={job.workerName} size="small" /> : null}
        </Stack>
      </DialogTitle>
      <DialogContent dividers>
        <Stack spacing={2}>
          <Stack spacing={0.5}>
            <Typography fontWeight={700}>{fileNameFromPath(job.mediaPath)}</Typography>
            <Typography color="text.secondary" sx={{ wordBreak: 'break-all' }}>
              {job.mediaPath}
            </Typography>
          </Stack>
          {artifacts.data?.warnings?.length ? (
            <Alert severity="warning">{artifacts.data.warnings.join(' ')}</Alert>
          ) : null}
          {artifacts.isLoading ? <Alert severity="info">Loading job artifacts...</Alert> : null}
          <Tabs value={tab} onChange={(_, value) => setTab(value)} variant="scrollable">
            <Tab label="Original snapshot" />
            <Tab label={result ? 'Final result' : 'Planned result'} />
          </Tabs>
          {tab === 0 ? (
            <Stack spacing={2}>
              {!asIs ? (
                <Alert severity="info">
                  The AS-IS snapshot is generated when the worker starts conversion. This job has not generated it yet.
                </Alert>
              ) : null}
              <SummaryGrid
                items={[
                  ['Source container', stringValue(objectValue(sourceProbe, 'format'), 'format_name') || 'Unknown'],
                  ['Duration', formatSeconds(numberValue(objectValue(sourceProbe, 'format'), 'duration')) || 'Unknown'],
                  ['Original video', streamSummary(firstStream(sourceProbe, 'video'))],
                  ['Original audio', audioStreamsSummary(sourceProbe)],
                  ['Subtitles', subtitleStreamsSummary(sourceProbe)],
                  ['Selected profile', stringValue(profile, 'name') || `Profile #${job.profileId}`],
                  ['Audio profile', stringValue(asIs, 'audioProfileKey') || job.audioProfileKey || 'None'],
                  ['Processing mode', stringValue(asIs, 'processingMode') || 'Standard'],
                ]}
              />
              <InfoBlock title="Conversion command" value={stringValue(asIs, 'command') || commandFromNotes(job.notes)} />
              <ArtifactBlock title="AS-IS JSON" value={asIs} />
            </Stack>
          ) : (
            <Stack spacing={2}>
              {!result ? (
                <Alert severity="info">
                  Final analysis will be available when the job finishes. For now this shows the expected conversion plan.
                </Alert>
              ) : null}
              <SummaryGrid
                items={[
                  ['Pipeline status', stringValue(resultPayload, 'status') || job.status],
                  ['Output path', job.publishedPath || job.outputPath || 'Pending'],
                  ['Published path', job.publishedPath || 'Not published yet'],
                  ['Validation', validationSummary(job, resultPayload)],
                  ['Elapsed', elapsedSummary(job, resultPayload)],
                  ['Final video', streamSummary(firstStream(outputProbe, 'video')) || expectedVideoSummary(profile)],
                  ['Final audio', audioStreamsSummary(outputProbe) || expectedAudioSummary(job, profile)],
                  ['Automation', automationSummary(resultPayload)],
                ]}
              />
              <ChangeSummary job={job} sourceProbe={sourceProbe} outputProbe={outputProbe} profile={profile} result={resultPayload} />
              <InfoBlock title="Notes" value={job.notes} />
              <ArtifactBlock title={result ? 'Result JSON' : 'Planned job JSON'} value={result ?? plannedJob(job, asIs)} />
            </Stack>
          )}
        </Stack>
      </DialogContent>
    </Dialog>
  );
}

function SummaryGrid({ items }: { items: Array<[string, string]> }) {
  return (
    <Grid container spacing={1.25}>
      {items.map(([label, value]) => (
        <Grid key={label} size={{ xs: 12, sm: 6, md: 3 }}>
          <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25, height: '100%' }}>
            <Typography color="text.secondary" variant="body2">
              {label}
            </Typography>
            <Typography sx={{ mt: 0.5, wordBreak: 'break-word' }}>{value || 'Unknown'}</Typography>
          </Box>
        </Grid>
      ))}
    </Grid>
  );
}

function ChangeSummary({
  job,
  sourceProbe,
  outputProbe,
  profile,
  result,
}: {
  job: QueueJob;
  sourceProbe?: Record<string, unknown>;
  outputProbe?: Record<string, unknown>;
  profile?: Record<string, unknown>;
  result?: Record<string, unknown>;
}) {
  const changes = buildChanges(job, sourceProbe, outputProbe, profile, result);
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Typography fontWeight={700} sx={{ mb: 1 }}>
        What changed
      </Typography>
      <Stack spacing={0.75} divider={<Divider flexItem />}>
        {changes.map((change) => (
          <Typography key={change} color="text.secondary">
            {change}
          </Typography>
        ))}
      </Stack>
    </Box>
  );
}

function InfoBlock({ title, value }: { title: string; value?: string }) {
  if (!value) {
    return null;
  }
  return (
    <Box>
      <Typography fontWeight={700} sx={{ mb: 0.5 }}>
        {title}
      </Typography>
      <Box
        component="pre"
        sx={{
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
          bgcolor: 'rgba(255,255,255,0.03)',
          m: 0,
          p: 1.5,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          maxHeight: 220,
          overflow: 'auto',
        }}
      >
        {value}
      </Box>
    </Box>
  );
}

function ArtifactBlock({ title, value }: { title: string; value?: Record<string, unknown> }) {
  if (!value) {
    return null;
  }
  return <InfoBlock title={title} value={JSON.stringify(value, null, 2)} />;
}

function buildChanges(
  job: QueueJob,
  sourceProbe?: Record<string, unknown>,
  outputProbe?: Record<string, unknown>,
  profile?: Record<string, unknown>,
  result?: Record<string, unknown>,
) {
  const changes: string[] = [];
  const originalVideo = firstStream(sourceProbe, 'video');
  const finalVideo = firstStream(outputProbe, 'video');
  const originalVideoCodec = stringValue(originalVideo, 'codec_name');
  const finalVideoCodec = stringValue(finalVideo, 'codec_name') || stringValue(profile, 'videoCodec');
  if (originalVideoCodec && finalVideoCodec) {
    changes.push(originalVideoCodec === finalVideoCodec ? `Video codec stayed as ${finalVideoCodec}.` : `Video changed from ${originalVideoCodec} to ${finalVideoCodec}.`);
  } else if (finalVideoCodec) {
    changes.push(`Video will be encoded with ${finalVideoCodec}.`);
  }

  const originalAudio = firstStream(sourceProbe, 'audio');
  const finalAudio = firstStream(outputProbe, 'audio');
  const originalAudioCodec = stringValue(originalAudio, 'codec_name');
  const finalAudioCodec = stringValue(finalAudio, 'codec_name') || stringValue(profile, 'audioCodec');
  if (originalAudioCodec && finalAudioCodec) {
    changes.push(originalAudioCodec === finalAudioCodec ? `Audio codec stayed as ${finalAudioCodec}.` : `Audio changed from ${originalAudioCodec} to ${finalAudioCodec}.`);
  } else if (job.audioProfileKey) {
    changes.push(`Audio enhancement profile applied: ${job.audioProfileKey}.`);
  }

  if (job.publishedPath || stringValue(objectValue(result, 'automation'), 'publishedPath')) {
    changes.push('The converted file was published to the destination library.');
  }
  if (job.notes.includes('Original archived:')) {
    changes.push('The original file was moved to the originals archive.');
  }
  if (job.validationScore) {
    changes.push(`Validation score: ${job.validationScore}.`);
  }
  if (changes.length === 0) {
    changes.push('Final comparison is waiting for the completed result artifact.');
  }
  return changes;
}

function plannedJob(job: QueueJob, asIs?: Record<string, unknown>) {
  return {
    job,
    command: stringValue(asIs, 'command') || commandFromNotes(job.notes),
    profile: objectValue(asIs, 'profile'),
    audioProfileKey: stringValue(asIs, 'audioProfileKey') || job.audioProfileKey,
    expectedOutputPath: job.outputPath,
  };
}

function firstStream(probe: Record<string, unknown> | undefined, type: string) {
  const streams = Array.isArray(probe?.streams) ? probe.streams : [];
  return streams.find((stream): stream is Record<string, unknown> => {
    return typeof stream === 'object' && stream !== null && stringValue(stream as Record<string, unknown>, 'codec_type') === type;
  });
}

function streamsOf(probe: Record<string, unknown> | undefined, type: string) {
  const streams = Array.isArray(probe?.streams) ? probe.streams : [];
  return streams.filter((stream): stream is Record<string, unknown> => {
    return typeof stream === 'object' && stream !== null && stringValue(stream as Record<string, unknown>, 'codec_type') === type;
  });
}

function streamSummary(stream?: Record<string, unknown>) {
  if (!stream) {
    return '';
  }
  const codec = stringValue(stream, 'codec_name') || 'unknown codec';
  const width = numberValue(stream, 'width');
  const height = numberValue(stream, 'height');
  const channels = numberValue(stream, 'channels');
  const language = languageOf(stream);
  if (width && height) {
    return `${codec} · ${width}x${height}${language ? ` · ${language}` : ''}`;
  }
  if (channels) {
    return `${codec} · ${channels} channel${channels === 1 ? '' : 's'}${language ? ` · ${language}` : ''}`;
  }
  return `${codec}${language ? ` · ${language}` : ''}`;
}

function audioStreamsSummary(probe?: Record<string, unknown>) {
  const streams = streamsOf(probe, 'audio');
  if (!streams.length) {
    return '';
  }
  return streams.map(streamSummary).join(' / ');
}

function subtitleStreamsSummary(probe?: Record<string, unknown>) {
  const streams = streamsOf(probe, 'subtitle');
  if (!streams.length) {
    return 'None detected';
  }
  return streams.map((stream) => languageOf(stream) || stringValue(stream, 'codec_name') || 'subtitle').join(', ');
}

function expectedVideoSummary(profile?: Record<string, unknown>) {
  const codec = stringValue(profile, 'videoCodec');
  const container = stringValue(profile, 'container');
  if (!codec && !container) {
    return 'Waiting for output probe';
  }
  return [codec, container].filter(Boolean).join(' · ');
}

function expectedAudioSummary(job: QueueJob, profile?: Record<string, unknown>) {
  if (job.audioProfileKey) {
    return job.audioProfileKey;
  }
  return stringValue(profile, 'audioCodec') || 'copy / preserve';
}

function validationSummary(job: QueueJob, result?: Record<string, unknown>) {
  const status = job.validationStatus || stringValue(result, 'validationStatus') || 'Pending';
  const score = job.validationScore || numberValue(result, 'validationScore');
  return score ? `${status} · ${score}` : status;
}

function elapsedSummary(job: QueueJob, result?: Record<string, unknown>) {
  const timing = objectValue(result, 'timing');
  const elapsed = numberValue(timing, 'elapsedSeconds');
  if (elapsed) {
    return formatDuration(Math.round(elapsed));
  }
  if (!job.startedAt) {
    return 'Not started';
  }
  const end = job.finishedAt ? new Date(job.finishedAt).getTime() : Date.now();
  const start = new Date(job.startedAt).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) {
    return 'Unknown';
  }
  return formatDuration(Math.round((end - start) / 1000));
}

function automationSummary(result?: Record<string, unknown>) {
  const automation = objectValue(result, 'automation');
  if (!automation) {
    return 'Pending';
  }
  const stoppedAt = stringValue(automation, 'stoppedAt');
  const message = stringValue(automation, 'message');
  return [stoppedAt || 'complete', message].filter(Boolean).join(' · ');
}

function languageOf(stream: Record<string, unknown>) {
  const tags = objectValue(stream, 'tags');
  return stringValue(tags, 'language');
}

function commandFromNotes(notes: string) {
  const marker = 'Conversion command: ';
  const line = notes.split('\n').find((entry) => entry.includes(marker));
  return line ? line.slice(line.indexOf(marker) + marker.length) : '';
}

function objectValue(value: unknown, key?: string): Record<string, unknown> | undefined {
  const target = key && typeof value === 'object' && value !== null ? (value as Record<string, unknown>)[key] : value;
  return typeof target === 'object' && target !== null && !Array.isArray(target) ? (target as Record<string, unknown>) : undefined;
}

function stringValue(value: unknown, key: string) {
  if (!value || typeof value !== 'object') {
    return '';
  }
  const candidate = (value as Record<string, unknown>)[key];
  return typeof candidate === 'string' ? candidate : '';
}

function numberValue(value: unknown, key: string) {
  if (!value || typeof value !== 'object') {
    return 0;
  }
  const candidate = (value as Record<string, unknown>)[key];
  if (typeof candidate === 'number') {
    return candidate;
  }
  if (typeof candidate === 'string') {
    const parsed = Number(candidate);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function fileNameFromPath(path: string) {
  return path.split('/').filter(Boolean).pop() || path;
}

function formatSeconds(seconds: number) {
  if (!seconds) {
    return '';
  }
  return formatDuration(Math.round(seconds));
}

function formatDuration(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

function statusColor(status: QueueJob['status']) {
  switch (status) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'running':
      return 'primary';
    case 'canceled':
      return 'default';
    default:
      return 'warning';
  }
}
