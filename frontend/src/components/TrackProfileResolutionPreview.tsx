import { Alert, Box, Chip, LinearProgress, Stack, Table, TableBody, TableCell, TableRow, Typography } from '@mui/material';
import type { ScanResult, TrackProfileResolutionPreview as ResolutionPreview } from '../api/types';

type Props = {
  scan: ScanResult;
  preview?: ResolutionPreview;
  loading: boolean;
  error: Error | null;
};

export function TrackProfileResolutionPreview({ scan, preview, loading, error }: Props) {
  if (loading && !preview) return <LinearProgress />;
  if (error) return <Alert severity="error">{error.message}</Alert>;
  if (!preview) return <Alert severity="info">Select an asset in the Lab header to preview these rules.</Alert>;
  const groups = [
    ['Video', preview.video, scan.videoStreams],
    ['Audio', preview.audio, scan.audioStreams],
    ['Subtitles', preview.subtitle, scan.subtitleStreams],
  ] as const;
  const subtitleActions = new Map(preview.resolvedTrackPlan.subtitleStreams.map((stream) => [stream.streamIndex, stream.action]));
  const attachmentInventoryAvailable = scan.attachmentInventoryAvailable === true;
  const attachments = Array.isArray(preview.resolvedTrackPlan.attachmentStreams) ? preview.resolvedTrackPlan.attachmentStreams : [];
  const attachmentDisposition = preview.resolvedTrackPlan.attachmentsKept ? 'Keep' : 'Remove';
  return (
    <Stack spacing={1.5}>
      {preview.warnings.map((warning) => <Alert severity="warning" key={warning}>{warning}</Alert>)}
      {groups.map(([label, decisions, streams]) => (
        <Box key={label} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
          <Typography fontWeight={700} sx={{ px: 1.5, py: 1, bgcolor: 'action.hover' }}>{label}</Typography>
          {decisions.length ? <Table size="small" aria-label={`${label} Path rule preview`}>
            <TableBody>
              {decisions.map((decision) => {
                const stream = streams.find((candidate) => candidate.index === decision.index);
                return <TableRow key={decision.index}>
                  <TableCell sx={{ width: 128 }}><Chip size="small" color={decision.kept ? 'success' : 'default'} label={label === 'Subtitles' ? subtitleActionLabel(subtitleActions.get(decision.index) ?? 'remove') : decision.kept ? 'Keep' : 'Remove'} /></TableCell>
                  <TableCell sx={{ wordBreak: 'break-word' }}>#{decision.index} · {stream?.codec?.toUpperCase() || 'UNKNOWN'}{stream?.language ? ` · ${stream.language.toUpperCase()}` : ''}{stream?.title ? ` · ${stream.title}` : ''}</TableCell>
                  <TableCell sx={{ color: 'text.secondary' }}>{trackPreviewReasonLabel(decision.reason)}</TableCell>
                </TableRow>;
              })}
            </TableBody>
          </Table> : <Typography variant="body2" color="text.secondary" sx={{ p: 1.5 }}>No streams detected.</Typography>}
        </Box>
      ))}
      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
        <Typography fontWeight={700} sx={{ px: 1.5, py: 1, bgcolor: 'action.hover' }}>Attachments</Typography>
        {attachments.length ? <Table size="small" aria-label="Attachment inventory">
          <TableBody>
            {attachments.map((attachment) => {
              const filename = attachment.filename?.trim() || `Attachment #${attachment.streamIndex}`;
              return <TableRow key={attachment.streamIndex}>
                <TableCell sx={{ width: 128 }}><Chip size="small" color={preview.resolvedTrackPlan.attachmentsKept ? 'success' : 'default'} label={`Final MKV: ${attachmentDisposition}`} /></TableCell>
                <TableCell sx={{ wordBreak: 'break-word' }}>
                  <Typography variant="body2">#{attachment.streamIndex} · {filename}</Typography>
                  <Typography variant="caption" color="text.secondary">{attachmentTypeLabel(attachment)}</Typography>
                  {attachment.mimeType ? <Typography variant="caption" color="text.secondary" display="block">MIME: {attachment.mimeType}</Typography> : null}
                </TableCell>
              </TableRow>;
            })}
          </TableBody>
        </Table> : <Typography variant="body2" color="text.secondary" sx={{ p: 1.5 }}>
          {attachmentInventoryAvailable ? 'No source attachments.' : 'Attachment metadata unavailable.'}
        </Typography>}
      </Box>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} flexWrap="wrap" useFlexGap>
        <Chip label={`Attachments: ${preview.resolvedTrackPlan.attachmentsKept ? 'Keep' : 'Remove'} (${preview.resolvedTrackPlan.attachmentPolicy})`} title={preview.resolvedTrackPlan.attachmentReason} />
        <Chip label={`Chapters: ${preview.resolvedTrackPlan.chaptersKept ? 'Keep' : 'Remove'}`} />
        <Chip label={`Sidecars: ${preview.resolvedTrackPlan.sidecarOutputs.length}`} />
      </Stack>
      {preview.resolvedTrackPlan.sidecarOutputs.length ? <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
        <Typography fontWeight={700} sx={{ mb: 1 }}>Resolved subtitle sidecars</Typography>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          {preview.resolvedTrackPlan.sidecarOutputs.map((output) => <Chip key={`${output.streamIndex}-${output.format}`} label={`#${output.streamIndex} · ${(output.format || 'unknown').toUpperCase()} · ${output.mode === 'converted' ? 'Compatibility conversion' : 'Original format'}`} color={output.mode === 'converted' ? 'info' : 'default'} />)}
        </Stack>
      </Box> : null}
    </Stack>
  );
}

function attachmentTypeLabel(attachment: ResolutionPreview['resolvedTrackPlan']['attachmentStreams'][number]) {
  const kind = attachment.attachmentKind || 'ATTACHMENT';
  if (attachment.fontFormat) return `${kind} · ${attachment.fontFormat}`;
  const mimeType = attachment.mimeType?.toLowerCase();
  const filename = attachment.filename?.toLowerCase() ?? '';
  const codec = attachment.codec?.toLowerCase();
  if (kind === 'IMAGE') {
    if (mimeType === 'image/jpeg' || /\.jpe?g$/.test(filename) || codec === 'jpeg' || codec === 'mjpeg') return 'IMAGE · JPEG';
    if (mimeType === 'image/png' || filename.endsWith('.png') || codec === 'png') return 'IMAGE · PNG';
  }
  return `${kind} · ${codec?.toUpperCase() || 'UNKNOWN'}`;
}

function subtitleActionLabel(action: 'keep' | 'remove' | 'extract' | 'keep_and_extract') {
  return { keep: 'Keep', remove: 'Remove', extract: 'Extract', keep_and_extract: 'Keep + Extract' }[action];
}

function trackPreviewReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    first_video: 'First video', all_video: 'Keep all video', additional_video: 'Additional video',
    language_match: 'Language match', language_not_selected: 'Language not selected', commentary: 'Commentary rule',
    default_track: 'Default track', not_default: 'Not default', all_audio: 'Keep all audio', audio_disabled: 'Audio disabled',
    forced: 'Forced subtitle', not_forced: 'Not forced', all_subtitles: 'Keep all subtitles', subtitle_disabled: 'Subtitles disabled',
  };
  return labels[reason] ?? reason.replaceAll('_', ' ');
}
