import AddIcon from '@mui/icons-material/Add';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditIcon from '@mui/icons-material/Edit';
import { Alert, Box, Button, Checkbox, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, IconButton, LinearProgress, MenuItem, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, TextField, Tooltip, Typography } from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { AssetMaintenanceOperation, TrackMaintenanceStream } from '../api/types';

type Props = { path: string; active?: boolean; hasOpenJob?: boolean };

type EditDraft = { stream: TrackMaintenanceStream; title: string; language: string; default: boolean; forced: boolean };
type AACDraft = { stream: TrackMaintenanceStream; bitrateKbps: number; channels: 'source' | 'stereo'; title: string; language: string; default: boolean };

export function TrackMaintenancePanel({ path, active = true, hasOpenJob = false }: Props) {
  const client = useQueryClient();
  const [operation, setOperation] = useState<AssetMaintenanceOperation | null>(null);
  const [edit, setEdit] = useState<EditDraft | null>(null);
  const [aac, setAAC] = useState<AACDraft | null>(null);
  const [remove, setRemove] = useState<TrackMaintenanceStream | null>(null);
  const [removeConfirmed, setRemoveConfirmed] = useState(false);
  const inventory = useQuery({
    queryKey: ['trackMaintenanceInventory', path],
    queryFn: () => api.trackMaintenanceInventory(path),
    enabled: active && Boolean(path) && !operation,
  });
  const editMutation = useMutation({ mutationFn: () => api.editTrack({ path, streamIndex: edit!.stream.index, expectedFingerprint: inventory.data!.fingerprint, title: edit!.title, language: edit!.language, default: edit!.default, forced: edit!.forced }), onSuccess: (value) => { setEdit(null); setOperation(value); } });
  const aacMutation = useMutation({ mutationFn: () => api.addAACTrack({ path, sourceStreamIndex: aac!.stream.index, expectedFingerprint: inventory.data!.fingerprint, bitrateKbps: aac!.bitrateKbps, channels: aac!.channels, title: aac!.title, language: aac!.language, default: aac!.default }), onSuccess: (value) => { setAAC(null); setOperation(value); } });
  const removeMutation = useMutation({ mutationFn: () => api.startTrackRemoval({ path, streamIndexes: [remove!.index], expectedFingerprint: inventory.data!.fingerprint, confirmed: removeConfirmed }), onSuccess: (value) => { setRemove(null); setOperation(value); } });
  const busy = editMutation.isPending || aacMutation.isPending || removeMutation.isPending || operation?.status === 'queued' || operation?.status === 'running';
  const disabledReason = hasOpenJob ? 'This asset has an active Queue job. Track maintenance is temporarily locked.' : inventory.data?.maintenanceDisabledReason;
  const canMutate = Boolean(inventory.data?.maintenanceAllowed) && !hasOpenJob && !busy;

  useEffect(() => {
    if (!operation || (operation.status !== 'queued' && operation.status !== 'running')) return;
    const timer = window.setInterval(async () => {
      const current = await api.maintenanceOperation(operation.id);
      setOperation(current);
      if (current.status === 'completed') {
        await Promise.all([
          client.invalidateQueries({ queryKey: ['assets'] }),
          client.invalidateQueries({ queryKey: ['trackMaintenanceInventory', path] }),
        ]);
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [client, operation, path]);

  function closeOperation() {
    setOperation(null);
    editMutation.reset(); aacMutation.reset(); removeMutation.reset();
  }

  return <Stack spacing={2} sx={{ pt: 1.5 }}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
      <Box><Typography variant="h3">Tracks</Typography><Typography variant="body2" color="text.secondary">Inspect every stream and maintain active Library/Converted MKV assets without re-encoding unchanged tracks.</Typography></Box>
    </Stack>
    {inventory.isLoading ? <LinearProgress /> : null}
    {inventory.error ? <Alert severity="warning">{inventory.error.message}</Alert> : null}
    {editMutation.error ? <Alert severity="error">{editMutation.error.message}</Alert> : null}
    {aacMutation.error ? <Alert severity="error">{aacMutation.error.message}</Alert> : null}
    {removeMutation.error ? <Alert severity="error">{removeMutation.error.message}</Alert> : null}
    {disabledReason ? <Alert severity="info">{disabledReason}</Alert> : null}
    {operation ? <Stack spacing={1}><LinearProgress variant="determinate" value={operation.progress} /><Typography variant="body2">{operation.operationType} · {operation.phase} · {operation.progress}%</Typography>{operation.status === 'completed' ? <Alert severity="success">Track maintenance completed, validated, and the technical snapshot was refreshed.</Alert> : null}{operation.errorMessage ? <Alert severity="error">{operation.errorMessage}</Alert> : null}{operation.warning ? <Alert severity="warning">{operation.warning}</Alert> : null}{operation.status === 'completed' || operation.status === 'failed' ? <Button onClick={closeOperation} sx={{ alignSelf: 'flex-start' }}>Close result</Button> : null}</Stack> : null}
    {inventory.data ? <>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Chip size="small" label={`${inventory.data.streams.length} streams`} /><Chip size="small" label={`${inventory.data.chapters} chapters`} /></Stack>
      <TableContainer sx={{ border: 1, borderColor: 'divider', borderRadius: 1 }}><Table size="small"><TableHead><TableRow><TableCell>Stream</TableCell><TableCell>Codec</TableCell><TableCell>Language / title</TableCell><TableCell>Characteristics</TableCell><TableCell align="right">Actions</TableCell></TableRow></TableHead><TableBody>{inventory.data.streams.map((stream) => <TableRow key={stream.index}><TableCell>#{stream.index} · {stream.type}</TableCell><TableCell>{stream.codec}{stream.profile ? ` · ${stream.profile}` : ''}</TableCell><TableCell>{stream.language || 'und'}{stream.title ? ` · ${stream.title}` : ''}{stream.fileName ? ` · ${stream.fileName}` : ''}</TableCell><TableCell>{[stream.layout || (stream.channels ? `${stream.channels} ch` : ''), stream.width && stream.height ? `${stream.width}×${stream.height}` : '', stream.default ? 'default' : '', stream.forced ? 'forced' : ''].filter(Boolean).join(' · ') || '—'}</TableCell><TableCell align="right"><Stack direction="row" justifyContent="flex-end" spacing={0.5}>
        {stream.type === 'video' || stream.type === 'audio' || stream.type === 'subtitle' ? <Tooltip title="Edit language, title, and dispositions"><span><IconButton aria-label="Edit language, title, and dispositions" size="small" disabled={!canMutate} onClick={() => setEdit({ stream, title: stream.title ?? '', language: stream.language ?? '', default: stream.default, forced: stream.forced })}><EditIcon fontSize="small" /></IconButton></span></Tooltip> : null}
        {stream.type === 'audio' ? <Tooltip title="Create an additional AAC audio track"><span><IconButton aria-label="Create an additional AAC audio track" size="small" color="primary" disabled={!canMutate} onClick={() => setAAC({ stream, bitrateKbps: 192, channels: 'stereo', title: 'AAC Stereo (MVForge)', language: stream.language ?? '', default: false })}><AddIcon fontSize="small" /></IconButton></span></Tooltip> : null}
        {stream.type !== 'data' ? <Tooltip title="Delete this track"><span><IconButton aria-label="Delete this track" size="small" color="error" disabled={!canMutate} onClick={() => { setRemove(stream); setRemoveConfirmed(false); }}><DeleteOutlineIcon fontSize="small" /></IconButton></span></Tooltip> : null}
      </Stack></TableCell></TableRow>)}</TableBody></Table></TableContainer>
    </> : null}

    <Dialog open={Boolean(edit)} onClose={busy ? undefined : () => setEdit(null)} fullWidth maxWidth="sm"><DialogTitle>Edit track #{edit?.stream.index}</DialogTitle><DialogContent dividers><Stack spacing={2}><TextField label="Track title" value={edit?.title ?? ''} onChange={(event) => setEdit((current) => current ? { ...current, title: event.target.value } : current)} /><TextField label="Language" value={edit?.language ?? ''} onChange={(event) => setEdit((current) => current ? { ...current, language: event.target.value.toLowerCase() } : current)} helperText="ISO language code, for example eng, spa, or jpn." /><FormControlLabel control={<Checkbox checked={edit?.default ?? false} onChange={(_, checked) => setEdit((current) => current ? { ...current, default: checked } : current)} />} label="Default track" />{edit?.stream.type === 'subtitle' ? <FormControlLabel control={<Checkbox checked={edit.forced} onChange={(_, checked) => setEdit((current) => current ? { ...current, forced: checked } : current)} />} label="Forced track" /> : null}</Stack></DialogContent><DialogActions><Button disabled={busy} onClick={() => setEdit(null)}>Cancel</Button><Button variant="contained" disabled={busy || !edit} onClick={() => editMutation.mutate()}>Save metadata</Button></DialogActions></Dialog>

    <Dialog open={Boolean(aac)} onClose={busy ? undefined : () => setAAC(null)} fullWidth maxWidth="sm"><DialogTitle>Create AAC copy from track #{aac?.stream.index}</DialogTitle><DialogContent dividers><Stack spacing={2}><Alert severity="info">The source track and every other stream are preserved. Only the additional AAC track is encoded.</Alert><TextField select label="AAC bitrate" value={aac?.bitrateKbps ?? 192} onChange={(event) => setAAC((current) => current ? { ...current, bitrateKbps: Number(event.target.value) } : current)}>{[128,160,192,256,320].map((value) => <MenuItem key={value} value={value}>{value} kbps</MenuItem>)}</TextField><TextField select label="Channels" value={aac?.channels ?? 'stereo'} onChange={(event) => setAAC((current) => current ? { ...current, channels: event.target.value as 'source' | 'stereo' } : current)}><MenuItem value="stereo">Stereo compatibility copy</MenuItem><MenuItem value="source">Preserve source channel count</MenuItem></TextField><TextField label="Track title" value={aac?.title ?? ''} onChange={(event) => setAAC((current) => current ? { ...current, title: event.target.value } : current)} /><TextField label="Language" value={aac?.language ?? ''} onChange={(event) => setAAC((current) => current ? { ...current, language: event.target.value.toLowerCase() } : current)} /><FormControlLabel control={<Checkbox checked={aac?.default ?? false} onChange={(_, checked) => setAAC((current) => current ? { ...current, default: checked } : current)} />} label="Make the new AAC track default" /></Stack></DialogContent><DialogActions><Button disabled={busy} onClick={() => setAAC(null)}>Cancel</Button><Button variant="contained" disabled={busy || !aac} onClick={() => aacMutation.mutate()}>Create AAC track</Button></DialogActions></Dialog>

    <Dialog open={Boolean(remove)} onClose={busy ? undefined : () => setRemove(null)} fullWidth maxWidth="sm"><DialogTitle>Delete track #{remove?.index}</DialogTitle><DialogContent dividers><Stack spacing={2}><Alert severity="error">This rewrites the MKV without the selected track. The temporary recovery backup is removed after successful validation.</Alert><Typography>{remove ? `${remove.type} · ${remove.codec} · ${remove.language || 'und'}${remove.title ? ` · ${remove.title}` : ''}` : ''}</Typography><FormControlLabel control={<Checkbox checked={removeConfirmed} onChange={(_, checked) => setRemoveConfirmed(checked)} />} label="I understand that this track will be removed." /></Stack></DialogContent><DialogActions><Button disabled={busy} onClick={() => setRemove(null)}>Cancel</Button><Button color="error" variant="contained" disabled={busy || !remove || !removeConfirmed} onClick={() => removeMutation.mutate()}>Delete track</Button></DialogActions></Dialog>
  </Stack>;
}
