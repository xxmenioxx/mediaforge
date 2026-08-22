import { Alert, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, LinearProgress, Stack, Typography } from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../api/client';
import type { AssetMaintenanceOperation, TrackMaintenanceStream } from '../api/types';

type Props = { open: boolean; path: string; onClose: () => void };

type PanelProps = {
  path: string;
  active?: boolean;
  disabledReason?: string;
  onClose?: () => void;
  onBusyChange?: (busy: boolean) => void;
};

export function RemoveTracksDialog({ open, path, onClose }: Props) {
  const [busy, setBusy] = useState(false);
  return <Dialog open={open} onClose={busy ? undefined : onClose} maxWidth="md" fullWidth disableEscapeKeyDown={busy}>
    <DialogTitle>Remove tracks</DialogTitle>
    <DialogContent dividers>
      <RemoveTracksPanel path={path} active={open} onClose={onClose} onBusyChange={setBusy} />
    </DialogContent>
  </Dialog>;
}

export function RemoveTracksPanel({ path, active = true, disabledReason, onClose, onBusyChange }: PanelProps) {
  const client = useQueryClient();
  const [selected, setSelected] = useState<number[]>([]);
  const [confirmed, setConfirmed] = useState(false);
  const [operation, setOperation] = useState<AssetMaintenanceOperation | null>(null);
  const inventory = useQuery({
    queryKey: ['trackMaintenanceInventory', path],
    queryFn: () => api.trackMaintenanceInventory(path),
    enabled: active && Boolean(path) && !disabledReason && !operation,
  });
  const removal = useMutation({
    mutationFn: () => api.startTrackRemoval({ path, streamIndexes: selected, expectedFingerprint: inventory.data!.fingerprint, confirmed }),
    onSuccess: setOperation,
  });
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
  const selectedStreams = useMemo(() => inventory.data?.streams.filter((stream) => selected.includes(stream.index)) ?? [], [inventory.data, selected]);
  const playableVideo = inventory.data?.streams.filter((stream) => stream.type === 'video' && !stream.attachedPic && !stream.stillImage) ?? [];
  const removesAllVideo = playableVideo.length > 0 && playableVideo.every((stream) => selected.includes(stream.index));
  const busy = removal.isPending || operation?.status === 'queued' || operation?.status === 'running';
  useEffect(() => {
    onBusyChange?.(Boolean(busy));
    return () => onBusyChange?.(false);
  }, [busy, onBusyChange]);

  function close() {
    if (busy) return;
    setSelected([]); setConfirmed(false); setOperation(null); removal.reset(); onClose?.();
  }

  return <Stack spacing={2}>
        {disabledReason ? <Alert severity="info">{disabledReason}</Alert> : null}
        <Alert severity="warning">Removed tracks cannot be recovered after this operation completes. MVForge will remux the MKV without re-encoding video or audio.</Alert>
        {inventory.isLoading ? <LinearProgress /> : null}
        {inventory.error ? <Alert severity="error">{inventory.error.message}</Alert> : null}
        {inventory.data ? <Stack spacing={1}>{inventory.data.streams.filter((stream) => stream.type !== 'data').map((stream) =>
          <FormControlLabel key={stream.index} control={<Checkbox checked={selected.includes(stream.index)} disabled={busy} onChange={(_, checked) => setSelected((current) => checked ? [...current, stream.index] : current.filter((value) => value !== stream.index))} />} label={streamLabel(stream)} />
        )}</Stack> : null}
        {selectedStreams.some((stream) => stream.type === 'attachment') ? <Alert severity="warning">Removing font attachments can change ASS/SSA subtitle rendering.</Alert> : null}
        {removesAllVideo ? <Alert severity="error">At least one playable video stream must remain.</Alert> : null}
        {operation ? <Stack spacing={1}><LinearProgress variant="determinate" value={operation.progress} /><Typography variant="body2">{operation.phase} · {operation.progress}%</Typography>{operation.status === 'completed' ? <Alert severity="success">Tracks removed, asset validated, and snapshot refreshed.</Alert> : null}{operation.errorMessage ? <Alert severity="error">{operation.errorMessage}</Alert> : null}{operation.warning ? <Alert severity="warning">{operation.warning}</Alert> : null}</Stack> : null}
        {!operation ? <FormControlLabel control={<Checkbox checked={confirmed} disabled={busy} onChange={(_, checked) => setConfirmed(checked)} />} label={`I understand that ${selected.length || 'the selected'} track(s) cannot be recovered.`} /> : null}
        <DialogActions sx={{ px: 0, pb: 0 }}>
          {onClose ? <Button onClick={close} disabled={busy}>{operation?.status === 'completed' || operation?.status === 'failed' ? 'Close' : 'Cancel'}</Button> : null}
          {!operation ? <Button variant="contained" color="error" disabled={!inventory.data || selected.length === 0 || !confirmed || removesAllVideo || removal.isPending} onClick={() => removal.mutate()}>Remove {selected.length} track{selected.length === 1 ? '' : 's'}</Button> : null}
        </DialogActions>
  </Stack>;
}

function streamLabel(stream: TrackMaintenanceStream) {
  const details = [stream.language, stream.title, stream.fileName, stream.profile, stream.width && stream.height ? `${stream.width}×${stream.height}` : '', stream.layout || (stream.channels ? `${stream.channels} ch` : ''), stream.default ? 'default' : '', stream.forced ? 'forced' : ''].filter(Boolean);
  return `#${stream.index} · ${stream.type} · ${stream.codec}${details.length ? ` · ${details.join(' · ')}` : ''}`;
}
