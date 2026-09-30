import { useEffect, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { api, waitForCompatiblePreviewReady } from '../api/client';
import { SourceBitrateQualityEvidence } from './SourceBitrateQualityEvidence';
import type {
  QueueBatchPreflightAsset,
  QueueBatchPreflightInput,
  QueueBatchPreflightResponse,
  QueueJobInput,
} from '../api/types';

function recordText(record: Record<string, unknown>, key: string) {
  const value = record[key];
  if (value === undefined || value === null || value === '') return '';
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

function planSummary(asset: QueueBatchPreflightAsset) {
  const plan = asset.plan;
  if (!plan) return null;
  const profile = plan.profile as unknown as Record<string, unknown>;
  const workerConfig = (profile.workerConfig && typeof profile.workerConfig === 'object' ? profile.workerConfig : {}) as Record<string, unknown>;
  const destination = plan.destination;
  const provenance = Object.entries(plan.profileResolution).flatMap(([domain, decision]) => {
    if (!decision || typeof decision !== 'object') return [];
    const source = recordText(decision as Record<string, unknown>, 'source');
    return source ? [`${domain}: ${source}`] : [];
  }).join(' · ');
  const items = [
    ['Profile', recordText(profile, 'name') || `Profile ${asset.input.profileId}`],
    ['Profile provenance', provenance || 'Frozen effective resolution'],
    ['Profile version', `v${plan.profileVersion}${plan.profileCapturedAt ? ` · captured ${plan.profileCapturedAt}` : ''}`],
    ['Encoder / codec', `${plan.selectedEncoder} / ${recordText(profile, 'videoCodec') || recordText(workerConfig, 'videoCodec') || 'configured codec'}`],
    ['Quality', [recordText(profile, 'qualityMode'), recordText(profile, 'qualityValue') || recordText(workerConfig, 'qualityValue')].filter(Boolean).join(' ') || 'from frozen profile'],
    ['Frame structure', [recordText(profile, 'frameRate'), recordText(profile, 'gopSize') || recordText(workerConfig, 'gopSize')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['Geometry', `${recordText(profile, 'width') || recordText(workerConfig, 'width') || 'source'} × ${recordText(profile, 'height') || recordText(workerConfig, 'height') || 'source'}`],
    ['Crop / scale', [recordText(workerConfig, 'crop'), recordText(workerConfig, 'videoFilters')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['SAR / DAR', `${recordText(profile, 'sampleAspectRatio') || 'source'} / ${recordText(profile, 'displayAspectRatio') || 'resolved'}`],
    ['Color / HDR', [recordText(profile, 'colorPolicy'), recordText(workerConfig, 'colorPolicy')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['Restoration', recordText(workerConfig, 'restoration') || 'preserved/resolved in snapshot'],
    ['Audio actions', recordText(plan.audioProfileSnapshot, 'key') || (Object.keys(plan.audioProfileSnapshot).length ? 'Frozen audio profile' : 'Preserved by audio snapshot')],
    ['Tracks / subtitles', Object.keys(plan.trackProfileSnapshot).length ? 'Frozen track selection and subtitle policy' : 'Preserved by track snapshot'],
    ['Destination', recordText(destination, 'destinationRoot') || 'Unavailable'],
    ['Publish mode', plan.publishMode],
  ];
  return <Grid container spacing={0.75}>
    {items.map(([label, value]) => <Grid key={label} size={{ xs: 12, sm: 6 }}>
      <Typography variant="caption" color="text.secondary">{label}</Typography>
      <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{value}</Typography>
    </Grid>)}
    {plan.qualityEvidence ? <Grid size={{ xs: 12 }}><SourceBitrateQualityEvidence source={plan.qualityEvidence.sourceBitrateEvidence} assessment={plan.qualityEvidence.bpppfAssessment} /></Grid> : null}
    <Grid size={{ xs: 12 }}>
      <Typography variant="caption" color="text.secondary">Frozen plan evidence</Typography>
      <Box component="details" sx={{ mt: 0.5 }}>
        <Box component="summary" sx={{ cursor: 'pointer', typography: 'caption' }}>Profile provenance, audio, tracks, constraints and destination snapshots</Box>
        <Box component="pre" sx={{ overflow: 'auto', maxHeight: 240, whiteSpace: 'pre-wrap', fontSize: 11 }}>
          {JSON.stringify({ profile: plan.profileSnapshot, profileResolution: plan.profileResolution, qualityEvidence: plan.qualityEvidence, audio: plan.audioProfileSnapshot, tracks: plan.trackProfileSnapshot, constraints: plan.constraints, destination: plan.destination }, null, 2)}
        </Box>
      </Box>
    </Grid>
  </Grid>;
}

export function BatchPreflightDialog({
  candidate: initialCandidate,
  onClose,
  onAccepted,
}: {
  candidate: QueueBatchPreflightResponse | null;
  onClose: () => void;
  onAccepted: (candidate: QueueBatchPreflightResponse) => void;
}) {
  const [candidate, setCandidate] = useState(initialCandidate);
  const [selected, setSelected] = useState<string[]>(() => initialCandidate?.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id) ?? []);
  const [editing, setEditing] = useState(initialCandidate?.resolved === false);
  const [batchName, setBatchName] = useState(initialCandidate?.batchName ?? '');
  const [jobs, setJobs] = useState<QueueJobInput[]>(() => initialCandidate?.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })) ?? []);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [editBaseline, setEditBaseline] = useState<{ batchName: string; jobs: QueueJobInput[] } | null>(null);

  useEffect(() => {
    if (!initialCandidate) return;
    setCandidate(initialCandidate);
    setSelected(initialCandidate.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
    setBatchName(initialCandidate.batchName);
    setJobs(initialCandidate.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })));
    setEditing(!initialCandidate.resolved);
    setEditBaseline(initialCandidate.resolved ? null : {
      batchName: initialCandidate.batchName,
      jobs: initialCandidate.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })),
    });
    setError('');
  }, [initialCandidate]);

  if (!candidate) return null;

  const updateJob = (index: number, patch: Partial<QueueJobInput>) => {
    setJobs((current) => current.map((job, jobIndex) => jobIndex === index ? { ...job, ...patch } : job));
  };

  const beginEdit = async () => {
    setBusy(true); setError('');
    try {
      const invalidated = await api.editQueueBatchPreflight(candidate.id, candidate.revision);
      setCandidate(invalidated);
      setBatchName(invalidated.batchName);
      setJobs(invalidated.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })));
      setEditBaseline({ batchName: invalidated.batchName, jobs: invalidated.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })) });
      setSelected([]);
      setEditing(true);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const resolveEdits = async () => {
    setBusy(true); setError('');
    try {
      const input: QueueBatchPreflightInput = { revision: candidate.revision, batchId: candidate.batchId, batchName, jobs };
      const resolved = await api.updateQueueBatchPreflight(candidate.id, input);
      setCandidate(resolved);
      setSelected(resolved.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
      setEditing(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const accept = async () => {
    setBusy(true); setError('');
    try {
      const accepted = await api.acceptQueueBatchPreflight(candidate.id, candidate.revision, selected);
      setCandidate(accepted);
      onAccepted(accepted);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const runPreview = async (asset: QueueBatchPreflightAsset) => {
    if (!asset.plan) return;
    const previewWindow = window.open('about:blank', '_blank', 'noopener,noreferrer');
    setBusy(true); setError('');
    try {
      const preview = await api.createCompatiblePreviewRequest({
        path: asset.mediaPath,
        profile: asset.plan.profile,
        preflightCandidateId: candidate.id,
        preflightRevision: candidate.revision,
        preflightAssetId: asset.id,
        ephemeral: true,
      });
      const ready = await waitForCompatiblePreviewReady(preview.requestId);
      if (previewWindow) previewWindow.location.href = api.compatibleAssetPreviewUrl(preview.requestId);
      else window.open(api.compatibleAssetPreviewUrl(preview.requestId), '_blank', 'noopener,noreferrer');
      if (ready.state !== 'ready') throw new Error('Compatible Preview did not produce a ready artifact.');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const runTestEncode = async (asset: QueueBatchPreflightAsset) => {
    if (!asset.plan) return;
    setBusy(true); setError('');
    try {
      await api.createTestEncode({
        sourcePath: asset.mediaPath,
        libraryId: asset.input.libraryId,
        configurationSource: 'batch_preflight',
        preflightCandidateId: candidate.id,
        preflightRevision: candidate.revision,
        preflightAssetId: asset.id,
        startMode: 'beginning',
        durationSeconds: 60,
      });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const openLab = (asset: QueueBatchPreflightAsset) => {
    const params = new URLSearchParams({ preflightCandidateId: candidate.id, preflightRevision: String(candidate.revision), assetId: asset.id });
    window.open(`/profile-lab?${params.toString()}`, '_blank', 'noopener,noreferrer');
  };

  const discardEdits = async () => {
    if (!editBaseline) { setEditing(false); return; }
    setBusy(true); setError('');
    try {
      const restored = await api.updateQueueBatchPreflight(candidate.id, {
        revision: candidate.revision,
        batchId: candidate.batchId,
        batchName: editBaseline.batchName,
        jobs: editBaseline.jobs,
      });
      setCandidate(restored);
      setSelected(restored.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
      setEditing(false);
      setEditBaseline(null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const unresolvedCount = candidate.assets.filter((asset) => asset.status !== 'queued').length;
  const blockerCount = candidate.assets.filter((asset) => asset.status !== 'queued' && asset.blockers.length > 0).length;

  return <Dialog open onClose={busy ? undefined : onClose} fullWidth maxWidth="lg" aria-labelledby="batch-preflight-title">
    <DialogTitle id="batch-preflight-title">Batch Preflight · {candidate.batchName || 'Selected assets'}</DialogTitle>
    <DialogContent dividers>
      <Stack spacing={2}>
        <Alert severity={candidate.resolved ? 'info' : 'warning'}>
          Reviewed plan is the plan frozen into Queue. {candidate.assets.filter((asset) => asset.status === 'queued').length} queued; {unresolvedCount} remain in this candidate.
        </Alert>
        {error ? <Alert severity="error">{error}</Alert> : null}
        {editing ? <>
          <TextField label="Batch name" value={batchName} onChange={(event) => setBatchName(event.target.value)} fullWidth size="small" />
          {jobs.map((job, index) => <Box key={`${job.mediaPath}-${index}`} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
            <Typography variant="subtitle2" sx={{ mb: 1, overflowWrap: 'anywhere' }}>{job.mediaPath}</Typography>
            <Grid container spacing={1}>
              <Grid size={{ xs: 6, md: 2 }}><TextField label="Profile ID" type="number" size="small" value={job.profileId} onChange={(event) => updateJob(index, { profileId: Number(event.target.value) })} fullWidth /></Grid>
              <Grid size={{ xs: 6, md: 2 }}><TextField label="Library ID" type="number" size="small" value={job.libraryId} onChange={(event) => updateJob(index, { libraryId: Number(event.target.value) })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 3 }}><TextField label="Audio profile key" size="small" value={job.audioProfileKey ?? ''} onChange={(event) => updateJob(index, { audioProfileKey: event.target.value })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 3 }}><TextField label="Track profile key" size="small" value={job.trackProfileKey ?? ''} onChange={(event) => updateJob(index, { trackProfileKey: event.target.value })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 2 }}><TextField select label="Processing" size="small" value={job.processingMode ?? 'full_encode'} onChange={(event) => updateJob(index, { processingMode: event.target.value as QueueJobInput['processingMode'] })} fullWidth><MenuItem value="full_encode">Full encode</MenuItem><MenuItem value="audio_only">Audio only</MenuItem></TextField></Grid>
            </Grid>
          </Box>)}
        </> : candidate.assets.map((asset) => <Box key={asset.id} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
          <Stack spacing={1.25}>
            <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} justifyContent="space-between" gap={1}>
              <Stack direction="row" alignItems="center" flexWrap="wrap" useFlexGap gap={1}>
                {asset.status === 'ready' ? <FormControlLabel control={<Checkbox checked={selected.includes(asset.id)} onChange={(event) => setSelected((current) => event.target.checked ? [...current, asset.id] : current.filter((id) => id !== asset.id))} />} label="Queue this asset" /> : null}
                <Typography variant="subtitle2" sx={{ overflowWrap: 'anywhere' }}>{asset.mediaPath}</Typography>
              </Stack>
              <Chip size="small" label={asset.status} color={asset.status === 'ready' || asset.status === 'queued' ? 'success' : asset.status === 'review' ? 'warning' : 'error'} />
            </Stack>
            {asset.blockers.map((blocker) => <Alert key={blocker} severity="error" variant="outlined">{blocker}</Alert>)}
            {asset.warnings.map((warning) => <Alert key={warning} severity="warning" variant="outlined">{warning}</Alert>)}
            {planSummary(asset)}
            <Divider />
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Button size="small" disabled={busy || !asset.plan} onClick={() => void runPreview(asset)}>Preview exact plan</Button>
              <Button size="small" disabled={busy || !asset.plan} onClick={() => void runTestEncode(asset)}>Test Encode exact plan</Button>
              <Button size="small" disabled={!asset.plan} onClick={() => openLab(asset)}>Open LAB with candidate context</Button>
            </Stack>
          </Stack>
        </Box>)}
        {blockerCount ? <Alert severity="warning">{blockerCount} unresolved asset(s) need review or configuration. They remain in this candidate after approved assets are queued.</Alert> : null}
      </Stack>
    </DialogContent>
    <DialogActions>
      {editing ? <>
        <Button disabled={busy} onClick={() => void discardEdits()}>Discard edits and re-resolve</Button>
        <Button variant="contained" disabled={busy || jobs.length === 0} onClick={() => void resolveEdits()}>Resolve Preflight again</Button>
      </> : <>
        <Button disabled={busy || !candidate.resolved || unresolvedCount === 0} onClick={() => void beginEdit()}>Edit</Button>
        <Button disabled={busy} onClick={onClose}>Close</Button>
        <Button variant="contained" disabled={busy || !candidate.resolved || selected.length === 0} onClick={() => void accept()}>Accept &amp; Queue {selected.length ? `(${selected.length})` : ''}</Button>
      </>}
    </DialogActions>
  </Dialog>;
}
