import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ExternalSubtitle, MediaStreamInfo, SubtitleSyncInput, SubtitleSyncProposal } from '../api/types';

function formatTimestamp(milliseconds: number) {
  const hours = Math.floor(milliseconds / 3_600_000);
  const minutes = Math.floor((milliseconds % 3_600_000) / 60_000);
  const seconds = Math.floor((milliseconds % 60_000) / 1000);
  const fraction = milliseconds % 1000;
  return `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}.${String(fraction).padStart(3, '0')}`;
}

function audioLabel(stream: MediaStreamInfo) {
  const parts = [`#${stream.index}`, stream.language || 'und', stream.codec.toUpperCase()];
  if (stream.title) parts.push(stream.title);
  return parts.join(' · ');
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : 'Subtitle synchronization request failed.';
}

export function SubtitleSyncDialog({
  open,
  path,
  subtitle,
  audioStreams,
  onClose,
  onApply,
}: {
  open: boolean;
  path: string;
  subtitle: ExternalSubtitle | null;
  audioStreams: MediaStreamInfo[];
  onClose: () => void;
  onApply: (proposal: SubtitleSyncProposal) => Promise<void>;
}) {
  const [audioIndex, setAudioIndex] = useState('');
  const [offset, setOffset] = useState('');
  const [proposal, setProposal] = useState<SubtitleSyncProposal | null>(null);
  const capability = useQuery({
    queryKey: ['subtitleSyncCapability'],
    queryFn: api.subtitleSyncCapability,
    enabled: open,
    staleTime: 60_000,
  });
  const preview = useMutation({
    mutationFn: api.previewSubtitleSync,
    onSuccess: setProposal,
  });
  const apply = useMutation({
    mutationFn: onApply,
    onSuccess: () => {
      setProposal(null);
      onClose();
    },
  });
  const resetPreview = preview.reset;
  const resetApply = apply.reset;

  useEffect(() => {
    if (!open) return;
    setAudioIndex('');
    setOffset('');
    setProposal(null);
    resetPreview();
    resetApply();
  }, [open, subtitle?.path, resetApply, resetPreview]);

  const offsetMilliseconds = offset.trim() === '' ? Number.NaN : Number(offset);
  const selectedAudioIndex = audioIndex === '' ? Number.NaN : Number(audioIndex);
  const offsetValid = Number.isInteger(offsetMilliseconds)
    && Math.abs(offsetMilliseconds) <= 43_200_000
    && (subtitle?.format === 'srt' || offsetMilliseconds % 10 === 0);
  const canAnalyze = Boolean(
    subtitle
    && capability.data?.enabled
    && capability.data.fixedOffset
    && Number.isInteger(selectedAudioIndex)
    && offsetValid,
  );
  const input: SubtitleSyncInput | null = subtitle && canAnalyze ? {
    path,
    subtitlePath: subtitle.path,
    referenceAudioStreamIndex: selectedAudioIndex,
    offsetMilliseconds,
  } : null;

  function analyze() {
    if (!input) return;
    setProposal(null);
    preview.mutate(input);
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>Synchronize subtitle</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5}>
          {subtitle ? (
            <>
              <Typography variant="body2"><strong>Subtitle:</strong> {subtitle.language || 'Unknown language'} · {subtitle.format.toUpperCase()} · {subtitle.fileName}</Typography>
              <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>Source: {subtitle.path}</Typography>
              <FormControl fullWidth size="small">
                <InputLabel id="subtitle-sync-audio-label">Reference audio</InputLabel>
                <Select
                  labelId="subtitle-sync-audio-label"
                  label="Reference audio"
                  value={audioIndex}
                  onChange={(event) => {
                    setAudioIndex(event.target.value);
                    setProposal(null);
                  }}
                >
                  {audioStreams.map((stream) => <MenuItem key={stream.index} value={String(stream.index)}>{audioLabel(stream)}</MenuItem>)}
                </Select>
              </FormControl>
              {audioStreams.length === 0 ? <Alert severity="warning">Audio stream evidence is unavailable. Refresh Analysis before synchronizing.</Alert> : null}
              <Typography variant="subtitle2">Mode: Fixed offset</Typography>
              <TextField
                type="number"
                size="small"
                label="Offset (milliseconds)"
                value={offset}
                inputProps={{ step: subtitle.format === 'srt' ? 1 : 10, min: -43_200_000, max: 43_200_000 }}
                onChange={(event) => {
                  setOffset(event.target.value);
                  setProposal(null);
                }}
                helperText={subtitle.format === 'srt' ? 'Positive values move cues later; negative values move them earlier.' : 'ASS/SSA use centisecond timestamps, so offsets must use 10 ms increments.'}
              />
              {capability.data && !capability.data.automaticAlignment ? (
                <Alert severity="info">Automatic alignment is unavailable on this server. Fixed offset is available without an external engine.</Alert>
              ) : null}
              {capability.isLoading ? <LinearProgress /> : null}
              {capability.isError ? <Alert severity="warning">{errorMessage(capability.error)}</Alert> : null}
              <Button variant="outlined" onClick={analyze} disabled={!canAnalyze || preview.isPending}>
                {preview.isPending ? 'Analyzing…' : 'Preview offset'}
              </Button>
              {preview.isError ? <Alert severity="warning">{errorMessage(preview.error)}</Alert> : null}
              {proposal ? (
                <Stack spacing={1}>
                  <Alert severity="warning">
                    Manual fixed offset · {proposal.offsetMilliseconds >= 0 ? '+' : ''}{proposal.offsetMilliseconds} ms · Review recommended. No confidence score is generated for a manual correction.
                  </Alert>
                  <Typography variant="body2"><strong>Reference audio:</strong> {audioStreams.find((stream) => stream.index === proposal.referenceAudio.streamIndex) ? audioLabel(audioStreams.find((stream) => stream.index === proposal.referenceAudio.streamIndex)!) : `#${proposal.referenceAudio.streamIndex} · ${proposal.referenceAudio.codec}`}</Typography>
                  <Table size="small" aria-label="Subtitle timing preview">
                    <TableHead><TableRow><TableCell>Cue</TableCell><TableCell>Current</TableCell><TableCell>Proposed</TableCell></TableRow></TableHead>
                    <TableBody>
                      {proposal.preview.map((cue) => (
                        <TableRow key={cue.cueIndex}>
                          <TableCell>{cue.cueIndex}</TableCell>
                          <TableCell>{formatTimestamp(cue.currentStartMs)} – {formatTimestamp(cue.currentEndMs)}</TableCell>
                          <TableCell>{formatTimestamp(cue.proposedStartMs)} – {formatTimestamp(cue.proposedEndMs)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {proposal.diagnostics.map((diagnostic) => <Typography key={diagnostic} variant="caption" color="text.secondary">{diagnostic}</Typography>)}
                  {apply.isError ? <Alert severity="warning">{errorMessage(apply.error)}</Alert> : null}
                </Stack>
              ) : null}
            </>
          ) : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        {proposal ? <Button variant="contained" disabled={apply.isPending} onClick={() => apply.mutate(proposal)}>{apply.isPending ? 'Starting…' : 'Apply Sync'}</Button> : null}
      </DialogActions>
    </Dialog>
  );
}
