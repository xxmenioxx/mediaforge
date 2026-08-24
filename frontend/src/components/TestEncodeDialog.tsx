import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  LinearProgress,
  MenuItem,
  Stack,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { Library, TestEncode, TestEncodeInput } from '../api/types';
import { AVTimingSummary } from './AVTimingSummary';

type Props = {
  open: boolean;
  onClose: () => void;
  sourcePath: string;
  libraries: Library[];
  defaultLibraryId?: number;
  request: Omit<TestEncodeInput, 'sourcePath' | 'libraryId' | 'startMode' | 'startSeconds' | 'durationSeconds'>;
};

export function TestEncodeDialog({ open, onClose, sourcePath, libraries, defaultLibraryId = 0, request }: Props) {
  const queryClient = useQueryClient();
  const [libraryId, setLibraryId] = useState(defaultLibraryId || libraries[0]?.id || 0);
  const [startMode, setStartMode] = useState<TestEncodeInput['startMode']>('representative');
  const [startSeconds, setStartSeconds] = useState(0);
  const [durationSeconds, setDurationSeconds] = useState(20);
  const [details, setDetails] = useState<TestEncode | null>(null);
  const [detailsTab, setDetailsTab] = useState(0);
  const selectedLibraryId = libraryId || defaultLibraryId || libraries[0]?.id || 0;
  const tests = useQuery({
    queryKey: ['testEncodes', sourcePath],
    queryFn: () => api.testEncodes(sourcePath),
    enabled: open && Boolean(sourcePath),
    refetchInterval: (query) => query.state.data?.some((item) => item.status === 'waiting' || item.status === 'generating') ? 1500 : false,
  });
  const invalidate = async () => {
    await queryClient.invalidateQueries({ queryKey: ['testEncodes', sourcePath] });
  };
  const create = useMutation({
    mutationFn: () => api.createTestEncode({
      ...request,
      configurationToken: request.configurationSource === 'lab_draft' ? stableConfigurationToken(request) : undefined,
      sourcePath,
      libraryId: selectedLibraryId,
      startMode,
      startSeconds: startMode === 'custom' ? startSeconds : undefined,
      durationSeconds,
    }),
    onSuccess: invalidate,
  });
  const cancel = useMutation({ mutationFn: api.cancelTestEncode, onSuccess: invalidate });
  const keep = useMutation({ mutationFn: ({ id, value }: { id: number; value: boolean }) => api.keepTestEncode(id, value), onSuccess: invalidate });
  const remove = useMutation({ mutationFn: api.deleteTestEncode, onSuccess: invalidate });
  const busy = create.isPending || cancel.isPending || keep.isPending || remove.isPending;
  const openDetails = (item: TestEncode) => {
    setDetailsTab(0);
    setDetails(item);
  };
  const closeDetails = () => {
    setDetails(null);
    setDetailsTab(0);
  };

  return (
    <>
      <Dialog open={open} onClose={busy ? undefined : onClose} maxWidth="md" fullWidth>
        <DialogTitle>Generate Test Encode</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Alert severity="info">
              Generates a real short encode with the final MVForge pipeline. It does not enter Queue, publish, or archive the original.
            </Alert>
            {request.configurationSource === 'lab_draft' ? <Alert severity="warning">Uses the current Lab configuration, including unsaved changes.</Alert> : null}
            <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>{sourcePath}</Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
              <TextField select size="small" label="Destination library" value={selectedLibraryId || ''} onChange={(event) => setLibraryId(Number(event.target.value))} fullWidth>
                {libraries.map((library) => <MenuItem key={library.id} value={library.id}>{library.name}</MenuItem>)}
              </TextField>
              <TextField select size="small" label="Sample position" value={startMode} onChange={(event) => setStartMode(event.target.value as TestEncodeInput['startMode'])} fullWidth>
                <MenuItem value="representative">Representative</MenuItem>
                <MenuItem value="beginning">Beginning</MenuItem>
                <MenuItem value="middle">Middle</MenuItem>
                <MenuItem value="custom">Custom</MenuItem>
              </TextField>
              {startMode === 'custom' ? <TextField size="small" type="number" label="Start seconds" value={startSeconds} inputProps={{ min: 0, step: 1 }} onChange={(event) => setStartSeconds(Math.max(0, Number(event.target.value) || 0))} fullWidth /> : null}
              <TextField size="small" type="number" label="Duration" value={durationSeconds} inputProps={{ min: 5, max: 120 }} onChange={(event) => setDurationSeconds(Math.min(120, Math.max(5, Number(event.target.value) || 20)))} fullWidth />
            </Stack>
            <Button variant="contained" disabled={!selectedLibraryId || create.isPending} onClick={() => create.mutate()}>
              {create.isPending ? 'Preparing…' : 'Generate New Test Encode'}
            </Button>
            {create.isError ? <Alert severity="error">{create.error.message}</Alert> : null}
            <Divider />
            <Typography variant="h3">Test Encodes</Typography>
            {tests.isLoading ? <LinearProgress /> : null}
            {tests.isError ? <Alert severity="error">{tests.error.message}</Alert> : null}
            {tests.data?.length === 0 ? <Typography color="text.secondary">No tests have been generated for this asset.</Typography> : null}
            {tests.data?.map((item) => (
              <Stack key={item.id} spacing={1} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
                <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                  <Stack spacing={0.5}>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                      <Chip size="small" label={`T${item.id}`} />
                      <Chip size="small" color={item.status === 'ready' ? 'success' : item.status === 'failed' ? 'error' : item.status === 'generating' ? 'primary' : 'default'} label={item.status} />
                      <Chip size="small" label={`${item.durationSeconds}s`} />
                      {item.effectiveEncoder ? <Chip size="small" label={item.effectiveEncoder} /> : null}
                      {item.keep ? <Chip size="small" color="info" label="Kept" /> : null}
                      {!item.keep && item.expiresAt ? <Chip size="small" label={`Expires ${new Date(item.expiresAt).toLocaleString()}`} /> : null}
                      {testEncodeIsStale(item, request) ? <Chip size="small" color="warning" label="Stale" /> : null}
                    </Stack>
                    <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>{item.outputPath || item.phase}</Typography>
                  </Stack>
                  <Stack direction="row" spacing={0.75} alignItems="center">
                    {(item.status === 'waiting' || item.status === 'generating') ? <Button size="small" color="warning" onClick={() => cancel.mutate(item.id)}>Cancel</Button> : null}
                    {item.status === 'ready' ? <FormControlLabel control={<Switch size="small" checked={item.keep} onChange={(_, value) => keep.mutate({ id: item.id, value })} />} label="Keep" /> : null}
                    <Button size="small" onClick={() => openDetails(item)}>Details</Button>
                    {!['waiting', 'generating'].includes(item.status) ? <Button size="small" color="error" onClick={() => { if (window.confirm(`Delete Test Encode T${item.id}?`)) remove.mutate(item.id); }}>Delete</Button> : null}
                  </Stack>
                </Stack>
                {(item.status === 'waiting' || item.status === 'generating') ? <LinearProgress variant={item.progress > 0 ? 'determinate' : 'indeterminate'} value={item.progress} /> : null}
                {item.errorMessage ? <Alert severity="error">{item.errorMessage}</Alert> : null}
                {item.staleReason ? <Alert severity="warning">{item.staleReason}</Alert> : null}
                {testEncodeIsStale(item, request) && !item.staleReason ? <Alert severity="warning">Configuration changed since this test was generated.</Alert> : null}
              </Stack>
            ))}
          </Stack>
        </DialogContent>
        <DialogActions><Button onClick={onClose} disabled={busy}>Close</Button></DialogActions>
      </Dialog>
      <Dialog open={Boolean(details)} onClose={closeDetails} maxWidth="md" fullWidth>
        <DialogTitle>Test Encode T{details?.id} Details</DialogTitle>
        <DialogContent dividers sx={{ px: { xs: 2, sm: 3 } }}>
          <Tabs
            value={detailsTab}
            onChange={(_, value: number) => setDetailsTab(value)}
            variant="scrollable"
            scrollButtons="auto"
            allowScrollButtonsMobile
            aria-label="Test Encode details sections"
            sx={{ mb: 2, borderBottom: 1, borderColor: 'divider' }}
          >
            <Tab label="Summary" />
            <Tab label="Configuration" />
            <Tab label="FFmpeg" />
            <Tab label="Validation" />
          </Tabs>

          {detailsTab === 0 ? (
            <Stack spacing={1.5} role="tabpanel" aria-label="Summary">
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                <Chip size="small" label={details?.status || 'pending'} color={details?.status === 'ready' ? 'success' : details?.status === 'failed' ? 'error' : 'default'} />
                <Chip size="small" variant="outlined" label={`${details?.progress ?? 0}%`} />
                {details?.phase ? <Chip size="small" variant="outlined" label={details.phase} /> : null}
                {details?.stale ? <Chip size="small" color="warning" label="Stale" /> : null}
                {details?.keep ? <Chip size="small" color="info" label="Kept" /> : null}
              </Stack>
              {details?.errorMessage ? <Alert severity="error">{details.errorMessage}</Alert> : null}
              {details?.staleReason ? <Alert severity="warning">{details.staleReason}</Alert> : null}
              <DetailRow label="Configuration" value={`${details?.configurationSource || 'pending'} · ${details?.configurationHash || 'pending'}`} />
              <DetailRow label="Sample window" value={`${details?.startSeconds ?? 0}s + ${details?.durationSeconds ?? 0}s`} />
              <DetailRow label="Worker / encoder" value={`${details?.workerName || 'pending'} · ${details?.effectiveEncoder || 'pending'}`} />
              <DetailRow label="Source" value={details?.sourcePath || 'pending'} path />
              <DetailRow label="Output" value={details?.outputPath || 'pending'} path />
              <DetailRow label="Output size" value={formatBytes(details?.outputSizeBytes ?? 0)} />
              <DetailRow label="Created" value={formatDateTime(details?.createdAt)} />
              <DetailRow label="Started" value={formatDateTime(details?.startedAt)} />
              <DetailRow label="Completed" value={formatDateTime(details?.completedAt)} />
              {!details?.keep ? <DetailRow label="Expires" value={formatDateTime(details?.expiresAt)} /> : null}
            </Stack>
          ) : detailsTab === 1 ? (
            <Stack spacing={2} role="tabpanel" aria-label="Configuration">
              <Alert severity="info">Requested settings are kept separate from the effective configuration resolved by the pipeline.</Alert>
              <TextField label="Requested configuration snapshot" value={JSON.stringify(details?.requestedConfiguration ?? {}, null, 2)} multiline minRows={8} inputProps={{ readOnly: true }} />
              <TextField label="Effective configuration snapshot" value={JSON.stringify(details?.effectiveConfiguration ?? {}, null, 2)} multiline minRows={8} inputProps={{ readOnly: true }} />
            </Stack>
          ) : detailsTab === 2 ? (
            <Stack spacing={1.5} role="tabpanel" aria-label="FFmpeg">
              <Typography variant="body2" color="text.secondary">The effective command generated for this sample.</Typography>
              <TextField label="Effective FFmpeg command" value={details?.ffmpegCommand || 'Not available yet'} multiline minRows={10} inputProps={{ readOnly: true }} />
            </Stack>
          ) : (
            <Stack spacing={2} role="tabpanel" aria-label="Validation">
              <AVTimingSummary report={details?.validationReport} />
              <TextField label="Validation report" value={JSON.stringify(details?.validationReport ?? {}, null, 2)} multiline minRows={8} inputProps={{ readOnly: true }} />
              <TextField label="Subtitle artifacts" value={JSON.stringify(details?.subtitleArtifacts ?? [], null, 2)} multiline minRows={5} inputProps={{ readOnly: true }} />
            </Stack>
          )}
        </DialogContent>
        <DialogActions><Button onClick={closeDetails}>Close</Button></DialogActions>
      </Dialog>
    </>
  );
}

function DetailRow({ label, value, path = false }: { label: string; value: string; path?: boolean }) {
  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '150px minmax(0, 1fr)' }, gap: { xs: 0.25, sm: 1.5 } }}>
      <Typography variant="body2" color="text.secondary">{label}</Typography>
      <Typography variant="body2" sx={path ? { wordBreak: 'break-all' } : undefined}>{value}</Typography>
    </Box>
  );
}

function formatBytes(bytes: number): string {
  if (!bytes) return 'Not available yet';
  const units = ['B', 'KB', 'MB', 'GB'];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / (1024 ** index)).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function formatDateTime(value?: string): string {
  return value ? new Date(value).toLocaleString() : 'Not available yet';
}

function stableConfigurationToken(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableConfigurationToken).join(',')}]`;
  if (value && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${stableConfigurationToken(item)}`)
      .join(',')}}`;
  }
  return JSON.stringify(value) ?? 'null';
}

function testEncodeIsStale(item: TestEncode, request: Props['request']): boolean {
  return item.stale || (item.configurationSource === 'lab_draft'
    && typeof item.requestedConfiguration.configurationToken === 'string'
    && item.requestedConfiguration.configurationToken !== stableConfigurationToken(request));
}
